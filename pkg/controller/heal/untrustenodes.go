package heal

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/errors"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
	"github.com/ucloud/redis-cluster-operator/pkg/redisutil"
)

// FixUntrustedNodes used to remove Nodes that are not trusted by other nodes. It can append when a node
// are removed from the cluster (with the "forget nodes" command) but try to rejoins the cluster.
func (c *CheckAndHeal) FixUntrustedNodes(cluster *redisv1alpha1.DistributedRedisCluster, infos *redisutil.ClusterInfos, admin redisutil.IAdmin) (bool, error) {
	untrustedNode := listUntrustedNodes(infos)
	var errs []error
	doneAnAction := false

	for id, uNode := range untrustedNode {
		c.Logger.Info("[FixUntrustedNodes] found untrust node", "node", uNode)
		getByIPFunc := func(n *redisutil.Node) bool {
			if n.IP == uNode.IP && n.ID != uNode.ID {
				return true
			}
			return false
		}
		node2, err := infos.GetNodes().GetNodesByFunc(getByIPFunc)
		if err != nil && !redisutil.IsNodeNotFoundedError(err) {
			c.Logger.Error(err, "error with GetNodesByFunc(getByIPFunc) search function")
			errs = append(errs, err)
			continue
		}
		if len(node2) > 0 {
			// it means the POD is used by another Redis node ID so we should not delete the pod.
			continue
		}
		exist, reused := checkIfPodNameExistAndIsReused(uNode, c.Pods)
		if exist && !reused {
			c.Logger.Info("[FixUntrustedNodes] try to delete pod", "podName", uNode.PodName)
			if err := c.PodControl.DeletePodByName(cluster.Namespace, uNode.PodName); err != nil {
				errs = append(errs, err)
			}
		}
		doneAnAction = true
		if !c.DryRun {
			c.Logger.Info("[FixUntrustedNodes] try to forget node", "nodeId", id)
			if err := admin.ForgetNode(id); err != nil {
				errs = append(errs, err)
			}
		}
	}

	return doneAnAction, errors.NewAggregate(errs)
}

func listUntrustedNodes(infos *redisutil.ClusterInfos) map[string]*redisutil.Node {
	untrustedNodes := make(map[string]*redisutil.Node)
	if infos == nil || infos.Infos == nil {
		return untrustedNodes
	}
	for _, nodeinfos := range infos.Infos {
		for _, node := range nodeinfos.Friends {
			if node.HasStatus(redisutil.NodeStatusHandshake) {
				if _, found := untrustedNodes[node.ID]; !found {
					untrustedNodes[node.ID] = node
				}
			}
		}
	}
	return untrustedNodes
}

func checkIfPodNameExistAndIsReused(node *redisutil.Node, podlist []*corev1.Pod) (exist bool, reused bool) {
	if node.PodName == "" {
		return
	}
	for _, currentPod := range podlist {
		if currentPod.Name == node.PodName {
			exist = true
			if currentPod.Status.PodIP == node.IP {
				// this check is use to see if the Pod name is not use by another RedisNode.
				// for that we check the the Pod name from the Redis node is not used by another
				// Redis node, by comparing the IP of the current Pod with the Pod from the cluster bom.
				// if the Pod  IP and Name from the redis info is equal to the IP/NAME from the getPod; it
				// means that the Pod is still use and the Redis Node is not a ghost
				reused = true
				break
			}
		}
	}
	return
}
