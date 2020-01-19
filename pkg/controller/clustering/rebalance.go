package clustering

import (
	"fmt"
	"math"

	"github.com/ucloud/redis-cluster-operator/pkg/redisutil"
	"github.com/ucloud/redis-cluster-operator/pkg/utils"
)

// RebalancedCluster rebalanced a redis cluster.
func (c *Ctx) RebalancedCluster(admin redisutil.IAdmin, newMasterNodes redisutil.Nodes) error {
	nbNode := len(newMasterNodes)
	for _, node := range newMasterNodes {
		expected := int(float64(admin.GetHashMaxSlot()+1) / float64(nbNode))
		node.SetBalance(len(node.Slots) - expected)
	}

	totalBalance := 0
	for _, node := range newMasterNodes {
		totalBalance += node.Balance()
	}

	for totalBalance > 0 {
		for _, node := range newMasterNodes {
			if node.Balance() < 0 && totalBalance > 0 {
				b := node.Balance() - 1
				node.SetBalance(b)
				totalBalance -= 1
			}
		}
	}

	// Sort nodes by their slots balance.
	sn := newMasterNodes.SortByFunc(func(a, b *redisutil.Node) bool { return a.Balance() < b.Balance() })
	if log.V(4).Enabled() {
		for _, node := range sn {
			log.Info("debug rebalanced master", "node", node.IPPort(), "balance", node.Balance())
		}
	}

	log.Info(">>> rebalancing", "nodeNum", nbNode)

	dstIdx := 0
	srcIdx := len(sn) - 1

	for dstIdx < srcIdx {
		dst := sn[dstIdx]
		src := sn[srcIdx]

		var numSlots float64
		if math.Abs(float64(dst.Balance())) < math.Abs(float64(src.Balance())) {
			numSlots = math.Abs(float64(dst.Balance()))
		} else {
			numSlots = math.Abs(float64(src.Balance()))
		}

		if numSlots > 0 {
			log.Info(fmt.Sprintf("Moving %f slots from %s to %s", numSlots, src.IPPort(), dst.IPPort()))
			srcs := redisutil.Nodes{src}
			reshardTable := computeReshardTable(srcs, int(numSlots))
			if len(reshardTable) != int(numSlots) {
				log.Error(nil, "*** Assertion failed: Reshard table != number of slots", "table", len(reshardTable), "slots", numSlots)
			}
			for _, e := range reshardTable {
				if err := c.moveSlot(e, dst, admin); err != nil {
					return err
				}
			}
		}

		// Update nodes balance.
		log.V(4).Info("balance", "dst", dst.Balance(), "src", src.Balance(), "slots", numSlots)
		dst.SetBalance(dst.Balance() + int(numSlots))
		src.SetBalance(src.Balance() - int(numSlots))
		if dst.Balance() == 0 {
			dstIdx += 1
		}
		if src.Balance() == 0 {
			srcIdx -= 1
		}
	}

	return nil
}

type MovedNode struct {
	Source *redisutil.Node
	Slot   redisutil.Slot
}

// computeReshardTable Given a list of source nodes return a "resharding plan"
// with what slots to move in order to move "numslots" slots to another instance.
func computeReshardTable(src redisutil.Nodes, numSlots int) []*MovedNode {
	var moved []*MovedNode

	sources := src.SortByFunc(func(a, b *redisutil.Node) bool { return a.TotalSlots() < b.TotalSlots() })
	sourceTotSlots := 0
	for _, node := range sources {
		sourceTotSlots += node.TotalSlots()
	}
	for idx, node := range sources {
		n := float64(numSlots) / float64(sourceTotSlots) * float64(node.TotalSlots())

		if idx == 0 {
			n = math.Ceil(n)
		} else {
			n = math.Floor(n)
		}

		keys := node.Slots

		for i := 0; i < int(n); i++ {
			if len(moved) < numSlots {
				mnode := &MovedNode{
					Source: node,
					Slot:   keys[i],
				}
				moved = append(moved, mnode)
			}
		}
	}
	return moved
}

func (c *Ctx) moveSlot(source *MovedNode, target *redisutil.Node, admin redisutil.IAdmin) error {
	if err := admin.SetSlot(target.IPPort(), "IMPORTING", source.Slot, target.ID); err != nil {
		return err
	}
	if err := admin.SetSlot(source.Source.IPPort(), "MIGRATING", source.Slot, source.Source.ID); err != nil {
		return err
	}
	if _, err := admin.MigrateKeysInSlot(source.Source.IPPort(), target, source.Slot, 10, 30000, true); err != nil {
		return err
	}
	if err := admin.SetSlot(target.IPPort(), "NODE", source.Slot, target.ID); err != nil {
		c.log.Error(err, "SET NODE", "node", target.IPPort())
	}
	if err := admin.SetSlot(source.Source.IPPort(), "NODE", source.Slot, target.ID); err != nil {
		c.log.Error(err, "SET NODE", "node", source.Source.IPPort())
	}
	source.Source.Slots = redisutil.RemoveSlot(source.Source.Slots, source.Slot)
	return nil
}

func (c *Ctx) AllocSlots(admin redisutil.IAdmin, newMasterNodes redisutil.Nodes) error {
	mastersNum := len(newMasterNodes)
	clusterHashSlots := int(admin.GetHashMaxSlot() + 1)
	slotsPerNode := float64(clusterHashSlots) / float64(mastersNum)
	first := 0
	cursor := 0.0
	for index, node := range newMasterNodes {
		last := utils.Round(cursor + slotsPerNode - 1)
		if last > clusterHashSlots || index == mastersNum-1 {
			last = clusterHashSlots - 1
		}

		if last < first {
			last = first
		}

		node.Slots = redisutil.BuildSlotSlice(redisutil.Slot(first), redisutil.Slot(last))
		first = last + 1
		cursor += slotsPerNode
		if err := admin.AddSlots(node.IPPort(), node.Slots); err != nil {
			return err
		}
	}
	return nil
}
