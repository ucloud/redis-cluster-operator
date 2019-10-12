package clustering

import (
	"fmt"

	"github.com/ucloud/redis-cluster-operator/pkg/redisutil"
)

const unknownVMName = "unknown" // <-- I hope nobody will ever name a VM "unknown" because this will impact the algorythm inside that package. Maybe you should generate a mangled name or amore complex name here to reduce probability.

// PlaceMasters used to select Redis Node knowing on which VM they are running in order to spread as possible
// the masters on different VMs.
// Improvement: Use Kube Node labeling instead of the "NodeName", (availability zone and so)
func PlaceMasters(cluster *redisutil.Cluster, currentMaster redisutil.Nodes, allPossibleMasters redisutil.Nodes, nbMaster int32) (redisutil.Nodes, bool, error) {
	selection := redisutil.Nodes{}
	selection = append(selection, currentMaster...)

	// in case of scale down the current number of master is supperior to
	// the number of needed master so we limit the size of the selection.
	if len(selection) > int(nbMaster) {
		selection = selection[0:nbMaster]
	}

	masterByVM := sortRedisNodeByVM(cluster, allPossibleMasters)
	vmWithAlreadyMaster := sortRedisNodeByVM(cluster, currentMaster)

	bestEffort := false
	for len(selection) < int(nbMaster) {
		isProgress := false
		for vmName, nodes := range masterByVM {
			if !bestEffort {
				// discard vm with already Master(s) when we are not in best effort
				if _, ok := vmWithAlreadyMaster[vmName]; ok {
					continue
				}
			}
			if len(nodes) == 0 {
				continue
			}
			log.Info(fmt.Sprintf("- add node:%s to the master selection", nodes[0].ID))
			selection = append(selection, nodes[0])
			masterByVM[vmName] = nodes[1:]
			isProgress = true
			if len(selection) >= int(nbMaster) {
				return selection, bestEffort, nil
			}
		}
		if bestEffort && !isProgress {
			log.Error(nil, "nothing appends since last loop, it means no more master available")
			break
		}
		bestEffort = true
		log.Info("the pod are not spread enough on VMs to have only one master by VM.")
	}
	log.Info(fmt.Sprintf("- bestEffort %v", bestEffort))
	for _, node := range selection {
		log.Info(fmt.Sprintf("- Master %s, ip:%s", node.ID, node.IP))
	}
	if len(selection) >= int(nbMaster) {
		return selection, bestEffort, nil
	}
	return selection, bestEffort, fmt.Errorf("unable to found enough node for have the request number of master")
}

func sortRedisNodeByVM(cluster *redisutil.Cluster, nodes redisutil.Nodes) map[string]redisutil.Nodes {
	nodesByVM := make(map[string]redisutil.Nodes)

	for _, rnode := range nodes {
		cnode, err := cluster.GetNodeByID(rnode.ID)
		if err != nil {
			log.Error(err, fmt.Sprintf("[sortRedisNodeByVM] unable fo found the Cluster.Node with redis ID:%s", rnode.ID))
			continue // if not then next line with cnode.Pod will cause a panic since cnode is nil
		}
		vmName := unknownVMName
		if cnode.NodeName != "" {
			vmName = cnode.NodeName
		}
		if _, ok := nodesByVM[vmName]; !ok {
			nodesByVM[vmName] = redisutil.Nodes{}
		}
		nodesByVM[vmName] = append(nodesByVM[vmName], rnode)
	}

	return nodesByVM
}

// PlaceSlaves used to select Redis Node knowing on which VM they are running in order to spread as possible
func PlaceSlaves(cluster *redisutil.Cluster, masters, oldSlaves, newSlaves redisutil.Nodes, replicationFactor int32) (map[string]redisutil.Nodes, bool) {
	slavesByMaster := make(map[string]redisutil.Nodes)

	// be sure that no oldSlaves is presentin in newSlaves
	for _, newSlave := range newSlaves {
		for _, oldSlaves := range oldSlaves {
			if newSlave.ID == oldSlaves.ID {
				removeIDFunc := func(node *redisutil.Node) bool {
					return node.ID == newSlave.ID
				}
				newSlaves.FilterByFunc(removeIDFunc)
				log.V(4).Info("remove oldSlave for newSlave", "id:", newSlave.ID)
			}
		}
	}

	newSlavesByVM := sortRedisNodeByVM(cluster, newSlaves)

	for _, node := range masters {
		slavesByMaster[node.ID] = redisutil.Nodes{}
	}

	for _, slave := range oldSlaves {
		for _, master := range masters {
			if slave.MasterReferent == master.ID {
				if len(slavesByMaster[slave.MasterReferent]) >= int(replicationFactor) {
					if node, err := cluster.GetNodeByID(slave.ID); err != nil {
						vmName := unknownVMName
						if node.NodeName != "" {
							vmName = node.NodeName
						}
						newSlavesByVM[vmName] = append(newSlavesByVM[vmName], slave)
					}
				} else {
					//The master of this slave is among the new master nodes
					slavesByMaster[slave.MasterReferent] = append(slavesByMaster[slave.MasterReferent], slave)
					break
				}
			}
		}
	}

	slavesByVMNotUsed := make(map[string]redisutil.Nodes)
	isSlaveNodeUsed := false

	// we iterate on free slaves by Vms
	for vmName, slaves := range newSlavesByVM {
		// then for this VM "vmName" we try to attach those slaves on a Master
		for idPossibleSlave, possibleSlave := range slaves {
			// Now we iterate on the Master and check if the current VM is already used for a Slave attach
			// to the current master "idMaster"
			slaveUsed := false
			for idMaster, currentSlaves := range slavesByMaster {
				if len(currentSlaves) >= int(replicationFactor) {
					// already enough slaves attached to this master
					continue
				}

				if checkIfSameVM(cluster, idMaster, vmName) {
					continue
				}

				// lets check if the VM already host a slave for this master
				vmAlreadyUsedForSlave := false
				for _, currentSlave := range currentSlaves {
					vmSlaveNode, err := cluster.GetNodeByID(currentSlave.ID)
					if err != nil {
						log.Error(err, fmt.Sprintf("unable to find in the cluster the slave with id: %s", currentSlave.ID))
						continue
					}
					vmSlaveName := unknownVMName
					vmSlaveName = vmSlaveNode.NodeName
					if vmName == vmSlaveName {
						vmAlreadyUsedForSlave = true
						break
					}
				}
				if !vmAlreadyUsedForSlave {
					// This vm is not already used for hosting a slave for this master so we can attach this slave to it.
					slavesByMaster[idMaster] = append(slavesByMaster[idMaster], slaves[idPossibleSlave])
					slaveUsed = true
					break
				}
			}
			if !slaveUsed {
				isSlaveNodeUsed = true
				// store unused slave for later dispatch
				slavesByVMNotUsed[vmName] = append(slavesByVMNotUsed[vmName], possibleSlave)
			}
		}
	}

	bestEffort := false
	if isSlaveNodeUsed {
		bestEffort = true
		log.V(4).Info("unable to spread properly all the Slave on different VMs, we start best effort")
		for _, freeSlaves := range slavesByVMNotUsed {
			for _, freeSlave := range freeSlaves {
				for masterID, slaves := range slavesByMaster {
					if len(slaves) >= int(replicationFactor) {
						continue
					}
					slavesByMaster[masterID] = append(slavesByMaster[masterID], freeSlave)
					break
				}
			}
		}
	}

	return slavesByMaster, bestEffort
}

func checkIfSameVM(cluster *redisutil.Cluster, redisID, vmName string) bool {
	nodeVMName := unknownVMName
	if vmNode, err := cluster.GetNodeByID(redisID); err == nil {
		nodeVMName = vmNode.NodeName
	}

	if vmName == nodeVMName {
		return true
	}

	return false
}
