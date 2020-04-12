package distributedrediscluster

import (
	"fmt"
	"math"
	"reflect"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
	"github.com/ucloud/redis-cluster-operator/pkg/redisutil"
	"github.com/ucloud/redis-cluster-operator/pkg/utils"
)

func SetClusterFailed(status *redisv1alpha1.DistributedRedisClusterStatus, reason string) {
	status.Status = redisv1alpha1.ClusterStatusKO
	status.Reason = reason
}

func SetClusterOK(status *redisv1alpha1.DistributedRedisClusterStatus, reason string) {
	status.Status = redisv1alpha1.ClusterStatusOK
	status.Reason = reason
}

func SetClusterRebalancing(status *redisv1alpha1.DistributedRedisClusterStatus, reason string) {
	status.Status = redisv1alpha1.ClusterStatusRebalancing
	status.Reason = reason
}

func SetClusterScaling(status *redisv1alpha1.DistributedRedisClusterStatus, reason string) {
	status.Status = redisv1alpha1.ClusterStatusScaling
	status.Reason = reason
}

func SetClusterUpdating(status *redisv1alpha1.DistributedRedisClusterStatus, reason string) {
	status.Status = redisv1alpha1.ClusterStatusRollingUpdate
	status.Reason = reason
}

func SetClusterResetPassword(status *redisv1alpha1.DistributedRedisClusterStatus, reason string) {
	status.Status = redisv1alpha1.ClusterStatusResetPassword
	status.Reason = reason
}

func buildClusterStatus(clusterInfos *redisutil.ClusterInfos, pods []*corev1.Pod,
	cluster *redisv1alpha1.DistributedRedisCluster, reqLogger logr.Logger) *redisv1alpha1.DistributedRedisClusterStatus {
	oldStatus := cluster.Status
	status := &redisv1alpha1.DistributedRedisClusterStatus{
		Status:  oldStatus.Status,
		Reason:  oldStatus.Reason,
		Restore: oldStatus.Restore,
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
		if len(pod.OwnerReferences) > 0 {
			if pod.OwnerReferences[0].Kind == "StatefulSet" {
				newNode.StatefulSet = pod.OwnerReferences[0].Name
			}
		}
		redisNodes, err := clusterInfos.GetNodes().GetNodesByFunc(func(node *redisutil.Node) bool {
			return node.IP == pod.Status.PodIP
		})
		if err != nil {
			reqLogger.Error(err, fmt.Sprintf("unable to retrieve the associated redis node with the pod: %s, ip:%s", pod.Name, pod.Status.PodIP))
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

	minReplicationFactor := math.MaxInt32
	maxReplicationFactor := 0
	for _, counter := range nbSlaveByMaster {
		if counter > maxReplicationFactor {
			maxReplicationFactor = counter
		}
		if counter < minReplicationFactor {
			minReplicationFactor = counter
		}
	}
	if len(nbSlaveByMaster) == 0 {
		minReplicationFactor = 0
	}
	status.MaxReplicationFactor = int32(maxReplicationFactor)
	status.MinReplicationFactor = int32(minReplicationFactor)

	return status
}

func (r *ReconcileDistributedRedisCluster) updateClusterIfNeed(cluster *redisv1alpha1.DistributedRedisCluster,
	newStatus *redisv1alpha1.DistributedRedisClusterStatus,
	reqLogger logr.Logger) {
	if compareStatus(&cluster.Status, newStatus, reqLogger) {
		reqLogger.WithValues("namespace", cluster.Namespace, "name", cluster.Name).
			V(3).Info("status changed")
		cluster.Status = *newStatus
		r.crController.UpdateCRStatus(cluster)
	}
}

func compareStatus(old, new *redisv1alpha1.DistributedRedisClusterStatus, reqLogger logr.Logger) bool {
	if utils.CompareStringValue("ClusterStatus", string(old.Status), string(new.Status), reqLogger) {
		return true
	}

	if utils.CompareStringValue("ClusterStatusReason", old.Reason, new.Reason, reqLogger) {
		return true
	}

	if utils.CompareInt32("NumberOfMaster", old.NumberOfMaster, new.NumberOfMaster, reqLogger) {
		return true
	}

	if utils.CompareInt32("len(Nodes)", int32(len(old.Nodes)), int32(len(new.Nodes)), reqLogger) {
		return true
	}

	if utils.CompareStringValue("restoreSucceeded", string(old.Restore.Phase), string(new.Restore.Phase), reqLogger) {
		return true
	}

	for _, nodeA := range old.Nodes {
		found := false
		for _, nodeB := range new.Nodes {
			if nodeA.ID == nodeB.ID {
				found = true
				if compareNodes(&nodeA, &nodeB, reqLogger) {
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

func compareNodes(nodeA, nodeB *redisv1alpha1.RedisClusterNode, reqLogger logr.Logger) bool {
	if utils.CompareStringValue("Node.IP", nodeA.IP, nodeB.IP, reqLogger) {
		return true
	}
	if utils.CompareStringValue("Node.MasterRef", nodeA.MasterRef, nodeB.MasterRef, reqLogger) {
		return true
	}
	if utils.CompareStringValue("Node.PodName", nodeA.PodName, nodeB.PodName, reqLogger) {
		return true
	}
	if utils.CompareStringValue("Node.Port", nodeA.Port, nodeB.Port, reqLogger) {
		return true
	}
	if utils.CompareStringValue("Node.Role", string(nodeA.Role), string(nodeB.Role), reqLogger) {
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
		reqLogger.V(4).Info(fmt.Sprintf("compare Node.Slote size: %d - %d", sizeSlotsA, sizeSlotsB))
		return true
	}

	if (sizeSlotsA != 0) && !reflect.DeepEqual(nodeA.Slots, nodeB.Slots) {
		reqLogger.V(4).Info(fmt.Sprintf("compare Node.Slote deepEqual: %v - %v", nodeA.Slots, nodeB.Slots))
		return true
	}

	return false
}
