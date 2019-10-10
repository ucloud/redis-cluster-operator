package distributedrediscluster

import (
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
	"github.com/ucloud/redis-cluster-operator/pkg/redisutil"
)

func SetClusterFailed(status *redisv1alpha1.DistributedRedisClusterStatus, reason string) {
	status.Status = redisv1alpha1.ClusterStatusKO
	status.Reason = reason
}

func SetClusterOK(status *redisv1alpha1.DistributedRedisClusterStatus, reason string) {
	status.Status = redisv1alpha1.ClusterStatusOK
	status.Reason = reason
}

func SetClusterCreating(status *redisv1alpha1.DistributedRedisClusterStatus, reason string) {
	status.Status = redisv1alpha1.ClusterStatusCreating
	status.Reason = reason
}

func SetClusterScaling(status *redisv1alpha1.DistributedRedisClusterStatus, reason string) {
	status.Status = redisv1alpha1.ClusterStatusScaling
	status.Reason = reason
}

func buildClusterStatus(err error, clusterInfos *redisutil.ClusterInfos, pods []corev1.Pod) *redisv1alpha1.DistributedRedisClusterStatus {
	status := &redisv1alpha1.DistributedRedisClusterStatus{}
	if err == nil {
		status.Status = redisv1alpha1.ClusterStatusOK
		status.Reason = "OK"
	} else {
		status.Status = redisv1alpha1.ClusterStatusKO
		status.Reason = err.Error()
	}

	nbMaster := int32(0)
	nbSlaveByMaster := map[string]int{}

	for _, pod := range pods {
		newNode := redisv1alpha1.RedisClusterNode{
			PodName:  pod.Name,
			NodeName: pod.Spec.NodeName,
			IP:       pod.Status.PodIP,
			Slots:    []string{},
		}
		redisNodes, err := clusterInfos.GetNodes().GetNodesByFunc(func(node *redisutil.Node) bool {
			return node.IP == pod.Status.PodIP
		})
		if err != nil {
			log.Error(err, fmt.Sprintf("unable to retrieve the associated redis node with the pod: %s, ip:%s", pod.Name, pod.Status.PodIP))
			continue
		}
		if len(redisNodes) == 1 {
			redisNode := redisNodes[0]
			if redisutil.IsMasterWithSlot(redisNode) {
				if _, ok := nbSlaveByMaster[redisNode.ID]; !ok {
					nbSlaveByMaster[redisNode.ID] = 0
				}
				nbMaster++
			}

			newNode.ID = redisNode.ID
			newNode.Role = redisNode.GetRole()
			newNode.Port = redisNode.Port
			newNode.Slots = []string{}
			if redisutil.IsSlave(redisNode) && redisNode.MasterReferent != "" {
				nbSlaveByMaster[redisNode.MasterReferent] = nbSlaveByMaster[redisNode.MasterReferent] + 1
				newNode.MasterRef = redisNode.MasterReferent
			}
			if len(redisNode.Slots) > 0 {
				slots := redisutil.SlotRangesFromSlots(redisNode.Slots)
				for _, slot := range slots {
					newNode.Slots = append(newNode.Slots, slot.String())
				}
			}
		}
		status.Nodes = append(status.Nodes, newNode)
	}
	status.NumberOfMaster = nbMaster

	return status
}

func (r *ReconcileDistributedRedisCluster) updateClusterIfNeed(cluster *redisv1alpha1.DistributedRedisCluster, newStatus *redisv1alpha1.DistributedRedisClusterStatus) {
	if compareStatus(&cluster.Status, newStatus) {
		log.WithValues("namespace", cluster.Namespace, "name", cluster.Name).
			V(3).Info("status changed")
		cluster.Status = *newStatus
		r.clusterStatusController.UpdateCluster(cluster)
	}
}

func compareStatus(old, new *redisv1alpha1.DistributedRedisClusterStatus) bool {
	if old.Status != new.Status || old.Reason != new.Reason {
		return true
	}

	if old.NumberOfMaster != new.NumberOfMaster {
		return true
	}

	if len(old.Nodes) != len(new.Nodes) {
		return true
	}

	if compareStringValue("ClusterStatus", string(old.Status), string(new.Status)) {
		return true
	}

	if compareStringValue("ClusterStatusReason", old.Reason, new.Reason) {
		return true
	}

	if compareInts("NumberOfMaster", old.NumberOfMaster, new.NumberOfMaster) {
		return true
	}

	if compareInts("len(Nodes)", int32(len(old.Nodes)), int32(len(new.Nodes))) {
		return true
	}

	for _, nodeA := range old.Nodes {
		found := false
		for _, nodeB := range new.Nodes {
			if nodeA.ID == nodeB.ID {
				found = true
				if compareNodes(&nodeA, &nodeB) {
					return true
				}
			}
		}
		if !found {
			return true
		}
	}

	return false
}

func compareNodes(nodeA, nodeB *redisv1alpha1.RedisClusterNode) bool {
	if compareStringValue("Node.IP", nodeA.IP, nodeB.IP) {
		return true
	}
	if compareStringValue("Node.MasterRef", nodeA.MasterRef, nodeB.MasterRef) {
		return true
	}
	if compareStringValue("Node.PodName", nodeA.PodName, nodeB.PodName) {
		return true
	}
	if compareStringValue("Node.Port", nodeA.Port, nodeB.Port) {
		return true
	}
	if compareStringValue("Node.Role", string(nodeA.Role), string(nodeB.Role)) {
		return true
	}

	sizeSlotsA := 0
	sizeSlotsB := 0
	if nodeA.Slots != nil {
		sizeSlotsA = len(nodeA.Slots)
	}
	if nodeB.Slots != nil {
		sizeSlotsB = len(nodeB.Slots)
	}
	if sizeSlotsA != sizeSlotsB {
		log.V(4).Info(fmt.Sprintf("compare Node.Slote size: %d - %d", sizeSlotsA, sizeSlotsB))
		return true
	}

	if (sizeSlotsA != 0) && !reflect.DeepEqual(nodeA.Slots, nodeB.Slots) {
		log.V(4).Info(fmt.Sprintf("compare Node.Slote deepEqual: %v - %v", nodeA.Slots, nodeB.Slots))
		return true
	}

	return false
}

func compareIntValue(name string, old, new *int32) bool {
	if old == nil && new == nil {
		return true
	} else if old == nil || new == nil {
		return false
	} else if *old != *new {
		log.V(4).Info(fmt.Sprintf("compare status.%s: %d - %d", name, *old, *new))
		return true
	}

	return false
}

func compareInts(name string, old, new int32) bool {
	if old != new {
		log.V(4).Info(fmt.Sprintf("compare status.%s: %d - %d", name, old, new))
		return true
	}

	return false
}

func compareStringValue(name string, old, new string) bool {
	if old != new {
		log.V(4).Info(fmt.Sprintf("compare %s: %s - %s", name, old, new))
		return true
	}

	return false
}
