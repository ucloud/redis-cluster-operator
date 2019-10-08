package distributedrediscluster

import (
	"net"
	"time"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
	"github.com/ucloud/redis-cluster-operator/pkg/config"
	"github.com/ucloud/redis-cluster-operator/pkg/redisutil"
	"github.com/ucloud/redis-cluster-operator/pkg/resources/statefulsets"
)

const (
	requeueAfter   = 10 * time.Second
	reconcileAfter = 30 * time.Second
)

func (r *ReconcileDistributedRedisCluster) sync(cluster *redisv1alpha1.DistributedRedisCluster) error {
	cluster.Validate()
	logger := log.WithValues("namespace", cluster.Namespace, "name", cluster.Name)
	// step 1. apply statefulSet for cluster
	labels := getLabels(cluster)
	if err := r.ensurer.EnsureRedisStatefulset(cluster, labels); err != nil {
		return Kubernetes.Wrap(err, "EnsureRedisStatefulset")
	}
	if err := r.ensurer.EnsureRedisHeadLessSvc(cluster, labels); err != nil {
		return Kubernetes.Wrap(err, "EnsureRedisHeadLessSvc")
	}

	// step 2. wait for all redis node ready
	if err := r.checker.CheckRedisNodeNum(cluster); err != nil {
		return Requeue.Wrap(err, "CheckRedisNodeNum")
	}

	// step 3. check if the cluster is empty, if it is empty, init the cluster
	redisClusterPods, err := r.statefulSetController.GetStatefulSetPods(cluster.Namespace, statefulsets.ClusterStatefulSetName(cluster.Name))
	if err != nil {
		return Kubernetes.Wrap(err, "GetStatefulSetPods")
	}
	password, err := getClusterPassword(r.client, cluster)
	if err != nil {
		return Kubernetes.Wrap(err, "getClusterPassword")
	}

	admin, err := newRedisAdmin(redisClusterPods.Items, password, config.RedisConf())
	if err != nil {
		return Redis.Wrap(err, "newRedisAdmin")
	}
	defer admin.Close()

	isEmpty, err := admin.ClusterManagerNodeIsEmpty()
	if err != nil {
		return Redis.Wrap(err, "ClusterManagerNodeIsEmpty")
	}
	if isEmpty {
		clusterInfos, err := admin.GetClusterInfos()
		if err != nil {
			cerr := err.(redisutil.ClusterInfosError)
			if !cerr.Inconsistent() {
				return Redis.Wrap(err, "GetClusterInfos")
			}
		}
		logger.Info(clusterInfos.GetNodes().String())

		if err := makeCluster(cluster, clusterInfos); err != nil {
			return NoType.Wrap(err, "makeCluster")
		}
		for _, nodeInfo := range clusterInfos.Infos {
			if len(nodeInfo.Node.MasterReferent) == 0 {
				err = admin.AddSlots(net.JoinHostPort(nodeInfo.Node.IP, nodeInfo.Node.Port), nodeInfo.Node.Slots)
				if err != nil {
					return Redis.Wrap(err, "AddSlots")
				}
			}
		}
		logger.Info(">>> Nodes configuration updated")
		logger.Info(">>> Assign a different config epoch to each node")
		err = admin.SetConfigEpoch()
		if err != nil {
			return Redis.Wrap(err, "SetConfigEpoch")
		}
		logger.Info(">>> Sending CLUSTER MEET messages to join the cluster")
		err = admin.AttachNodeToCluster()
		if err != nil {
			return Redis.Wrap(err, "AttachNodeToCluster")
		}

		time.Sleep(3 * time.Second)

		for _, nodeInfo := range clusterInfos.Infos {
			if len(nodeInfo.Node.MasterReferent) != 0 {
				err = admin.AttachSlaveToMaster(nodeInfo.Node, nodeInfo.Node.MasterReferent)
				if err != nil {
					return Redis.Wrap(err, "AttachSlaveToMaster")
				}
			}
		}
	}

	return nil
}

func (r *ReconcileDistributedRedisCluster) createCluster() error {
	return nil
}
