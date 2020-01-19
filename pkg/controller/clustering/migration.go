package clustering

import (
	"fmt"
	"math"
	"sort"

	logf "sigs.k8s.io/controller-runtime/pkg/log"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
	"github.com/ucloud/redis-cluster-operator/pkg/redisutil"
)

var log = logf.Log.WithName("clustering")

type migrationInfo struct {
	From *redisutil.Node
	To   *redisutil.Node
}

type mapSlotByMigInfo map[migrationInfo][]redisutil.Slot

// DispatchMasters used to select nodes with master roles
func DispatchMasters(cluster *redisutil.Cluster, nodes redisutil.Nodes, nbMaster int32) (redisutil.Nodes, redisutil.Nodes, redisutil.Nodes, error) {
	log.Info("start dispatching slots to masters", "nb nodes:", len(nodes))
	var allMasterNodes redisutil.Nodes
	// First loop get Master with already Slots assign on it
	currentMasterNodes := nodes.FilterByFunc(redisutil.IsMasterWithSlot)
	allMasterNodes = append(allMasterNodes, currentMasterNodes...)

	// add also available Master without slot
	currentMasterWithNoSlot := nodes.FilterByFunc(redisutil.IsMasterWithNoSlot)
	allMasterNodes = append(allMasterNodes, currentMasterWithNoSlot...)
	log.V(2).Info("master with no slot", "lens:", len(currentMasterWithNoSlot))

	newMasterNodesSmartSelection, besteffort, err := PlaceMasters(cluster, currentMasterNodes, currentMasterWithNoSlot, nbMaster)

	log.V(2).Info(fmt.Sprintf("Total masters: %d - target %d - selected: %d", len(allMasterNodes), nbMaster, len(newMasterNodesSmartSelection)))
	if err != nil {
		return redisutil.Nodes{}, redisutil.Nodes{}, redisutil.Nodes{}, fmt.Errorf("not enough master available current:%d target:%d, err:%v", len(allMasterNodes), nbMaster, err)
	}

	newMasterNodesSmartSelection = newMasterNodesSmartSelection.SortByFunc(func(a, b *redisutil.Node) bool { return a.ID < b.ID })

	cluster.Status = redisv1alpha1.ClusterStatusCalculatingRebalancing
	if besteffort {
		cluster.NodesPlacement = redisv1alpha1.NodesPlacementInfoBestEffort
	} else {
		cluster.NodesPlacement = redisv1alpha1.NodesPlacementInfoOptimal
	}

	return newMasterNodesSmartSelection, currentMasterNodes, allMasterNodes, nil
}

// DispatchSlotToNewMasters used to dispatch Slot to the new master nodes
func (c *Ctx) DispatchSlotToNewMasters(admin redisutil.IAdmin, newMasterNodes, currentMasterNodes, allMasterNodes redisutil.Nodes) error {
	// Calculate the Migration slot information (which slots goes from where to where)
	migrationSlotInfo, info := c.feedMigInfo(newMasterNodes, currentMasterNodes, allMasterNodes, int(admin.GetHashMaxSlot()+1))
	c.cluster.ActionsInfo = info
	c.cluster.Status = redisv1alpha1.ClusterStatusRebalancing
	for nodesInfo, slots := range migrationSlotInfo {
		// There is a need for real error handling here, we must ensure we don't keep a slot in abnormal state
		if nodesInfo.From == nil {
			c.log.V(4).Info("1) add slots that having probably been lost during scale down", "destination:", nodesInfo.To.ID, "total:", len(slots), " : ", redisutil.SlotSlice(slots))
			err := admin.AddSlots(nodesInfo.To.IPPort(), slots)
			if err != nil {
				c.log.Error(err, "error during ADDSLOTS")
				return err
			}
		} else {
			c.log.V(6).Info("1) Send SETSLOT IMPORTING command", "target:", nodesInfo.To.ID, "source-node:", nodesInfo.From.ID, " total:", len(slots), " : ", redisutil.SlotSlice(slots))
			err := admin.SetSlots(nodesInfo.To.IPPort(), "IMPORTING", slots, nodesInfo.From.ID)
			if err != nil {
				c.log.Error(err, "error during IMPORTING")
				return err
			}
			c.log.V(6).Info("2) Send SETSLOT MIGRATION command", "target:", nodesInfo.From.ID, "destination-node:", nodesInfo.To.ID, " total:", len(slots), " : ", redisutil.SlotSlice(slots))
			err = admin.SetSlots(nodesInfo.From.IPPort(), "MIGRATING", slots, nodesInfo.To.ID)
			if err != nil {
				c.log.Error(err, "error during MIGRATING")
				return err
			}

			c.log.V(6).Info("3) Migrate Key")
			nbMigrated, migerr := admin.MigrateKeys(nodesInfo.From.IPPort(), nodesInfo.To, slots, 10, 30000, true)
			if migerr != nil {
				c.log.Error(migerr, "error during MIGRATION")
			} else {
				c.log.V(7).Info("migrated", "key", nbMigrated)
			}

			// we absolutly need to do setslot on the node owning the slot first, otherwise in case of manager crash, only the owner may think it is now owning the slot
			// creating a cluster view discrepency
			err = admin.SetSlots(nodesInfo.To.IPPort(), "NODE", slots, nodesInfo.To.ID)
			if err != nil {
				c.log.V(4).Info(fmt.Sprintf("warning during SETSLOT NODE on %s: %v", nodesInfo.To.IPPort(), err))
			}
			err = admin.SetSlots(nodesInfo.From.IPPort(), "NODE", slots, nodesInfo.To.ID)
			if err != nil {
				c.log.V(4).Info(fmt.Sprintf("warning during SETSLOT NODE on %s: %v", nodesInfo.From.IPPort(), err))
			}

			// Update bom
			nodesInfo.From.Slots = redisutil.RemoveSlots(nodesInfo.From.Slots, slots)

			// now tell all other nodes
			for _, master := range allMasterNodes {
				if master.IPPort() == nodesInfo.To.IPPort() || master.IPPort() == nodesInfo.From.IPPort() {
					// we already did those two
					continue
				}
				if master.TotalSlots() == 0 {
					// some nodes may not be master anymore
					// as we removed all slots in previous iteration of this code
					// we ignore those nodes
					continue
				}
				c.log.V(6).Info("4) Send SETSLOT NODE command", "target:", master.ID, "new owner:", nodesInfo.To.ID, " total:", len(slots), " : ", redisutil.SlotSlice(slots))
				err = admin.SetSlots(master.IPPort(), "NODE", slots, nodesInfo.To.ID)
				if err != nil {
					c.log.V(4).Info(fmt.Sprintf("warning during SETSLOT NODE on %s: %v", master.IPPort(), err))
				}
			}
		}
		// Update bom
		nodesInfo.To.Slots = redisutil.AddSlots(nodesInfo.To.Slots, slots)
	}
	return nil
}

func (c *Ctx) feedMigInfo(newMasterNodes, oldMasterNodes, allMasterNodes redisutil.Nodes, nbSlots int) (mapOut mapSlotByMigInfo, info redisutil.ClusterActionsInfo) {
	mapOut = make(mapSlotByMigInfo)
	mapSlotToUpdate := c.buildSlotsByNode(newMasterNodes, oldMasterNodes, allMasterNodes, nbSlots)

	for id, slots := range mapSlotToUpdate {
		for _, s := range slots {
			found := false
			for _, oldNode := range oldMasterNodes {
				if oldNode.ID == id {
					if redisutil.Contains(oldNode.Slots, s) {
						found = true
						break
					}
					continue
				}
				if redisutil.Contains(oldNode.Slots, s) {
					newNode, err := newMasterNodes.GetNodeByID(id)
					if err != nil {
						c.log.Error(err, "unable to find node", "with id:", id)
						continue
					}
					mapOut[migrationInfo{From: oldNode, To: newNode}] = append(mapOut[migrationInfo{From: oldNode, To: newNode}], s)
					found = true
					// increment slots counter
					info.NbslotsToMigrate++
					break
				}
			}
			if !found {
				// new slots added (not from an existing master). Correspond to lost slots during important scale down
				newNode, err := newMasterNodes.GetNodeByID(id)
				if err != nil {
					c.log.Error(err, "unable to find node", "with id:", id)
					continue
				}
				mapOut[migrationInfo{From: nil, To: newNode}] = append(mapOut[migrationInfo{From: nil, To: newNode}], s)

				// increment slots counter
				info.NbslotsToMigrate++
			}
		}
	}
	return mapOut, info
}

// buildSlotsByNode get all slots that have to be migrated with retrieveSlotToMigrateFrom and retrieveSlotToMigrateFromRemovedNodes
// and assign those slots to node that need them
func (c *Ctx) buildSlotsByNode(newMasterNodes, oldMasterNodes, allMasterNodes redisutil.Nodes, nbSlots int) map[string][]redisutil.Slot {
	var nbNode = len(newMasterNodes)
	if nbNode == 0 {
		return make(map[string][]redisutil.Slot)
	}
	nbSlotByNode := int(math.Ceil(float64(nbSlots) / float64(nbNode)))
	slotToMigrateByNode := c.retrieveSlotToMigrateFrom(oldMasterNodes, nbSlotByNode)
	slotToMigrateByNodeFromDeleted := c.retrieveSlotToMigrateFromRemovedNodes(newMasterNodes, oldMasterNodes)
	for id, slots := range slotToMigrateByNodeFromDeleted {
		slotToMigrateByNode[id] = slots
	}

	slotToMigrateByNode[""] = c.retrieveLostSlots(oldMasterNodes, nbSlots)
	if len(slotToMigrateByNode[""]) != 0 {
		c.log.Error(nil, fmt.Sprintf("several slots have been lost: %v", redisutil.SlotSlice(slotToMigrateByNode[""])))
	}
	slotToAddByNode := c.buildSlotByNodeFromAvailableSlots(newMasterNodes, nbSlotByNode, slotToMigrateByNode)

	total := 0
	for _, node := range allMasterNodes {
		currentSlots := 0
		removedSlots := 0
		addedSlots := 0
		expectedSlots := 0
		if slots, ok := slotToMigrateByNode[node.ID]; ok {
			removedSlots = len(slots)
		}
		if slots, ok := slotToAddByNode[node.ID]; ok {
			addedSlots = len(slots)
		}
		currentSlots += len(node.Slots)
		total += currentSlots - removedSlots + addedSlots
		searchByAddrFunc := func(n *redisutil.Node) bool {
			return n.IPPort() == node.IPPort()
		}
		if _, err := newMasterNodes.GetNodesByFunc(searchByAddrFunc); err == nil {
			expectedSlots = nbSlotByNode
		}
		c.log.Info(fmt.Sprintf("node %s will have %d + %d - %d = %d slots; expected: %d[+/-%d]", node.ID, currentSlots, addedSlots, removedSlots, currentSlots+addedSlots-removedSlots, expectedSlots, len(newMasterNodes)))
	}
	c.log.Info(fmt.Sprintf("Total slots: %d - expected: %d", total, nbSlots))

	return slotToAddByNode
}

// retrieveSlotToMigrateFrom list the number of slots that need to be migrated to reach nbSlotByNode per nodes
func (c *Ctx) retrieveSlotToMigrateFrom(oldMasterNodes redisutil.Nodes, nbSlotByNode int) map[string][]redisutil.Slot {
	slotToMigrateByNode := make(map[string][]redisutil.Slot)
	for _, node := range oldMasterNodes {
		c.log.V(6).Info("--- oldMasterNode:", "ID:", node.ID)
		nbSlot := node.TotalSlots()
		if nbSlot >= nbSlotByNode {
			if len(node.Slots[nbSlotByNode:]) > 0 {
				slotToMigrateByNode[node.ID] = append(slotToMigrateByNode[node.ID], node.Slots[nbSlotByNode:]...)
			}
			c.log.V(6).Info(fmt.Sprintf("--- migrating from %s, %d slots", node.ID, len(slotToMigrateByNode[node.ID])))
		}
	}
	return slotToMigrateByNode
}

// retrieveSlotToMigrateFromRemovedNodes given the list of node that will be masters with slots, and the list of nodes that were masters with slots
// return the list of slots from previous nodes that will be moved, because this node will no longer hold slots
func (c *Ctx) retrieveSlotToMigrateFromRemovedNodes(newMasterNodes, oldMasterNodes redisutil.Nodes) map[string][]redisutil.Slot {
	slotToMigrateByNode := make(map[string][]redisutil.Slot)
	var removedNodes redisutil.Nodes
	for _, old := range oldMasterNodes {
		c.log.V(6).Info("--- oldMasterNode:", old.ID)
		isPresent := false
		for _, new := range newMasterNodes {
			if old.ID == new.ID {
				isPresent = true
				break
			}
		}
		if !isPresent {
			removedNodes = append(removedNodes, old)
		}
	}

	for _, node := range removedNodes {
		slotToMigrateByNode[node.ID] = node.Slots
		c.log.V(6).Info(fmt.Sprintf("--- migrating from %s, %d slots", node.ID, len(slotToMigrateByNode[node.ID])))
	}
	return slotToMigrateByNode
}

// retrieveLostSlots retrieve the list of slots that are not attributed to a node
func (c *Ctx) retrieveLostSlots(oldMasterNodes redisutil.Nodes, nbSlots int) []redisutil.Slot {
	currentFullRange := []redisutil.Slot{}
	for _, node := range oldMasterNodes {
		// TODO a lot of perf improvement can be done here with better algorithm to add slot ranges
		currentFullRange = append(currentFullRange, node.Slots...)
	}
	sort.Sort(redisutil.SlotSlice(currentFullRange))
	lostSlots := []redisutil.Slot{}
	// building []slot of slots that are missing from currentFullRange to reach [0, nbSlots]
	last := redisutil.Slot(0)
	if len(currentFullRange) == 0 || currentFullRange[0] != 0 {
		lostSlots = append(lostSlots, 0)
	}
	for _, slot := range currentFullRange {
		if slot > last+1 {
			for i := last + 1; i < slot; i++ {
				lostSlots = append(lostSlots, i)
			}
		}
		last = slot
	}
	for i := last + 1; i < redisutil.Slot(nbSlots); i++ {
		lostSlots = append(lostSlots, i)
	}

	return lostSlots
}

func (c *Ctx) buildSlotByNodeFromAvailableSlots(newMasterNodes redisutil.Nodes, nbSlotByNode int, slotToMigrateByNode map[string][]redisutil.Slot) map[string][]redisutil.Slot {
	slotToAddByNode := make(map[string][]redisutil.Slot)
	var nbNode = len(newMasterNodes)
	if nbNode == 0 {
		return slotToAddByNode
	}
	var slotOfNode = make(map[int][]redisutil.Slot)
	for i, node := range newMasterNodes {
		slotOfNode[i] = node.Slots
	}
	var idNode = 0
	for _, slotsFrom := range slotToMigrateByNode {
		for _, slot := range slotsFrom {
			var missingSlots = nbSlotByNode - len(slotOfNode[idNode])
			if missingSlots > 0 {
				slotOfNode[idNode] = append(slotOfNode[idNode], slot)
				slotToAddByNode[newMasterNodes[idNode].ID] = append(slotToAddByNode[newMasterNodes[idNode].ID], slot)
			} else {
				idNode++
				if idNode > (nbNode - 1) {
					// all nodes have been filled
					idNode--
					c.log.V(7).Info(fmt.Sprintf("some available slots have not been assigned, over-filling node %s", newMasterNodes[idNode].ID))
				}
				slotOfNode[idNode] = append(slotOfNode[idNode], slot)
				slotToAddByNode[newMasterNodes[idNode].ID] = append(slotToAddByNode[newMasterNodes[idNode].ID], slot)
			}
		}
	}

	return slotToAddByNode
}
