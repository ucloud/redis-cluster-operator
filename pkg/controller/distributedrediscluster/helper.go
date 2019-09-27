package distributedrediscluster

import (
	"fmt"
	"net"
	"time"

	corev1 "k8s.io/api/core/v1"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
	"github.com/ucloud/redis-cluster-operator/pkg/config"
	"github.com/ucloud/redis-cluster-operator/pkg/redisutil"
	"github.com/ucloud/redis-cluster-operator/pkg/utils"
)

var (
	defaultLabels = map[string]string{
		redisv1alpha1.LabelManagedByKey: redisv1alpha1.OperatorName,
	}
)

func getLabels(cluster *redisv1alpha1.DistributedRedisCluster) map[string]string {
	dynLabels := map[string]string{
		redisv1alpha1.LabelNameKey: fmt.Sprintf("%s%c%s", cluster.Namespace, '_', cluster.Name),
	}
	return utils.MergeLabels(defaultLabels, dynLabels, cluster.Labels)
}

// newRedisAdmin builds and returns new redis.Admin from the list of pods
func newRedisAdmin(pods []corev1.Pod, cfg *config.Redis) (redisutil.IAdmin, error) {
	nodesAddrs := []string{}
	for _, pod := range pods {
		redisPort := redisutil.DefaultRedisPort
		for _, container := range pod.Spec.Containers {
			if container.Name == "redis-node" {
				for _, port := range container.Ports {
					if port.Name == "redis" {
						redisPort = fmt.Sprintf("%d", port.ContainerPort)
					}
				}
			}
		}
		nodesAddrs = append(nodesAddrs, net.JoinHostPort(pod.Status.PodIP, redisPort))
	}
	adminConfig := redisutil.AdminOptions{
		ConnectionTimeout:  time.Duration(cfg.DialTimeout) * time.Millisecond,
		RenameCommandsFile: cfg.GetRenameCommandsFile(),
	}

	return redisutil.NewAdmin(nodesAddrs, &adminConfig), nil
}

func makeCluster(cluster *redisv1alpha1.DistributedRedisCluster, clusterInfos *redisutil.ClusterInfos) error {
	logger := log.WithValues("namespace", cluster.Namespace, "name", cluster.Name)
	mastersCount := int(cluster.Spec.MasterSize)
	clusterReplicas := cluster.Spec.ClusterReplicas
	expectPodNum := mastersCount * int(clusterReplicas + 1)

	if len(clusterInfos.Infos) != expectPodNum {
		return fmt.Errorf("node num different from expectation")
	}

	logger.Info(fmt.Sprintf(">>> Performing hash slots allocation on %d nodes...", expectPodNum))

	masterNodes := make(redisutil.Nodes, mastersCount)
	i := 0
	k := 0
	slotsPerNode := redisutil.DefaultHashMaxSlots / mastersCount
	first := 0
	cursor := 0
	for _, nodeInfo := range clusterInfos.Infos {
		if i < mastersCount {
			nodeInfo.Node.Role = redisutil.RedisMasterRole
			last := cursor + slotsPerNode - 1
			if last > redisutil.DefaultHashMaxSlots+1 || i == mastersCount-1 {
				last = redisutil.DefaultHashMaxSlots
			}
			logger.Info(fmt.Sprintf("Master[%d] -> Slots %d - %d", i, first, last))
			nodeInfo.Node.Slots = redisutil.BuildSlotSlice(redisutil.Slot(first), redisutil.Slot(last))
			first = last + 1
			cursor += slotsPerNode
			masterNodes[i] = nodeInfo.Node
		} else {
			if k > mastersCount {
				k = 0
			}
			logger.Info(fmt.Sprintf("Adding replica %s:%s to %s:%s", nodeInfo.Node.IP, nodeInfo.Node.Port,
				masterNodes[k].IP, masterNodes[k].Port))
			nodeInfo.Node.Role = redisutil.RedisSlaveRole
			nodeInfo.Node.MasterReferent = masterNodes[k].ID
			k++
		}
		i++
	}

	log.Info(clusterInfos.GetNodes().String())

	return nil
}

//func makeCluster(cluster *redisv1alpha1.DistributedRedisCluster, pods []corev1.Pod) (*redisutil.Cluster, error) {
//	redisCluster := redisutil.NewCluster(cluster.Name, cluster.Namespace)
//	mastersCount := int(cluster.Spec.MasterSize)
//	clusterReplicas := cluster.Spec.ClusterReplicas
//	expectPodNum := mastersCount * int(clusterReplicas)
//
//	if len(pods) != expectPodNum {
//		return nil, fmt.Errorf("pod num different from expectation")
//	}
//
//	log.Info(fmt.Sprintf(">>> Performing hash slots allocation on %d nodes...", len(pods)))
//
//	slotsPerNode := redisutil.DefaultHashMaxSlots / mastersCount
//	first := 0
//	cursor := 0
//
//	for i := 0; i < mastersCount; i++ {
//		node := redisutil.NewNode(pods[i].Name, pods[i].Status.PodIP, &pods[i])
//		node.Role = redisutil.RedisMasterRole
//		last := cursor + slotsPerNode - 1
//		if last > redisutil.DefaultHashMaxSlots+1 || i == mastersCount-1 {
//			last = redisutil.DefaultHashMaxSlots
//		}
//		log.Info(fmt.Sprintf("Master[%d] -> Slots %d - %d", i, first, last))
//		node.Slots = redisutil.BuildSlotSlice(redisutil.Slot(first), redisutil.Slot(last))
//		first = last + 1
//		cursor += slotsPerNode
//
//		redisCluster.AddNode(node)
//	}
//
//	for i := mastersCount - 1; i < expectPodNum; i++ {
//
//	}
//
//	return redisCluster, nil
//}
