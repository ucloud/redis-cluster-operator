package heal

import (
	"k8s.io/apimachinery/pkg/util/errors"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
	"github.com/ucloud/redis-cluster-operator/pkg/redisutil"
)

// FixFailedNodes fix failed nodes: in some cases (cluster without enough master after crash or scale down), some nodes may still know about fail nodes
func (c *CheckAndHeal) FixFailedNodes(cluster *redisv1alpha1.DistributedRedisCluster, infos *redisutil.ClusterInfos, admin redisutil.IAdmin) (bool, error) {
	forgetSet := listGhostNodes(cluster, infos)
	var errs []error
	doneAnAction := false
	for id := range forgetSet {
		doneAnAction = true
		c.Logger.Info("[FixFailedNodes] Forgetting failed node, this command might fail, this is not an error", "node", id)
		if !c.DryRun {
			c.Logger.Info("[FixFailedNodes] try to forget node", "nodeId", id)
			if err := admin.ForgetNode(id); err != nil {
				errs = append(errs, err)
			}
		}
	}

	return doneAnAction, errors.NewAggregate(errs)
}

// listGhostNodes : A Ghost node is a node still known by some redis node but which doesn't exists anymore
// meaning it is failed, and pod not in kubernetes, or without targetable IP
func listGhostNodes(cluster *redisv1alpha1.DistributedRedisCluster, infos *redisutil.ClusterInfos) map[string]bool {
	ghostNodesSet := map[string]bool{}
	if infos == nil || infos.Infos == nil {
		return ghostNodesSet
	}
	for _, nodeinfos := range infos.Infos {
		for _, node := range nodeinfos.Friends {
			// only forget it when no more part of kubernetes, or if noaddress
			if node.HasStatus(redisutil.NodeStatusNoAddr) {
				ghostNodesSet[node.ID] = true
			}
			if node.HasStatus(redisutil.NodeStatusFail) || node.HasStatus(redisutil.NodeStatusPFail) {
				found := false
				for _, pod := range cluster.Status.Nodes {
					if pod.ID == node.ID {
						found = true
					}
				}
				if !found {
					ghostNodesSet[node.ID] = true
				}
			}
		}
	}
	return ghostNodesSet
}
