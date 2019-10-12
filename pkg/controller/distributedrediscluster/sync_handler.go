package distributedrediscluster

import (
	"net"
	"time"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
	"github.com/ucloud/redis-cluster-operator/pkg/controller/clustering"
	"github.com/ucloud/redis-cluster-operator/pkg/redisutil"
)

const (
	requeueAfter = 10 * time.Second
)

func (r *ReconcileDistributedRedisCluster) waitPodReady(cluster *redisv1alpha1.DistributedRedisCluster) error {
	cluster.Validate()
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

	return nil
}

func (r *ReconcileDistributedRedisCluster) waitForClusterJoin(cluster *redisv1alpha1.DistributedRedisCluster, clusterInfos *redisutil.ClusterInfos, admin redisutil.IAdmin) error {
	logger := log.WithValues("namespace", cluster.Namespace, "name", cluster.Name)
	logger.Info(">>> Assign a different config epoch to each node")
	err := admin.SetConfigEpoch()
	if err != nil {
		return Redis.Wrap(err, "SetConfigEpoch")
	}
	var firstNode *redisutil.Node
	for _, nodeInfo := range clusterInfos.Infos {
		firstNode = nodeInfo.Node
		break
	}
	logger.Info(">>> Sending CLUSTER MEET messages to join the cluster")
	err = admin.AttachNodeToCluster(firstNode.IPPort())
	if err != nil {
		return Redis.Wrap(err, "AttachNodeToCluster")
	}
	// Give one second for the join to start, in order to avoid that
	// waiting for cluster join will find all the nodes agree about
	// the config as they are still empty with unassigned slots.
	time.Sleep(1 * time.Second)
	_, err = admin.GetClusterInfos()
	if err == nil {
		return Requeue.Wrap(err, "wait for cluster join")
	}
}

func (r *ReconcileDistributedRedisCluster) sync(cluster *redisv1alpha1.DistributedRedisCluster, clusterInfos *redisutil.ClusterInfos, admin redisutil.IAdmin) error {
	logger := log.WithValues("namespace", cluster.Namespace, "name", cluster.Name)
	// step 3. check if the cluster is empty, if it is empty, init the cluster
	isEmpty, err := admin.ClusterManagerNodeIsEmpty()
	if err != nil {
		return Redis.Wrap(err, "ClusterManagerNodeIsEmpty")
	}
	if isEmpty {
		logger.Info("cluster nodes", "info", clusterInfos.GetNodes().String())

		if err := makeCluster(cluster, clusterInfos); err != nil {
			return NoType.Wrap(err, "makeCluster")
		}
		var firstNode *redisutil.Node
		for _, nodeInfo := range clusterInfos.Infos {
			if len(nodeInfo.Node.MasterReferent) == 0 {
				if firstNode == nil {
					firstNode = nodeInfo.Node
				}
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
		err = admin.AttachNodeToCluster(firstNode.IPPort())
		if err != nil {
			return Redis.Wrap(err, "AttachNodeToCluster")
		}

		time.Sleep(1 * time.Second)
		for {
			_, err = admin.GetClusterInfos()
			if err == nil {
				break
			}
			logger.Info("wait custer consistent")
			time.Sleep(1 * time.Second)
		}

		for _, nodeInfo := range clusterInfos.Infos {
			if len(nodeInfo.Node.MasterReferent) != 0 {
				err = admin.AttachSlaveToMaster(nodeInfo.Node, nodeInfo.Node.MasterReferent)
				if err != nil {
					return Redis.Wrap(err, "AttachSlaveToMaster")
				}
			}
		}
	}

	if err = admin.SetConfigIfNeed(cluster.Spec.Config); err != nil {
		return Redis.Wrap(err, "SetConfigIfNeed")
	}

	return nil
}

func (r *ReconcileDistributedRedisCluster) syncCluster(cluster *redisv1alpha1.DistributedRedisCluster, clusterInfos *redisutil.ClusterInfos, admin redisutil.IAdmin) error {
	logger := log.WithValues("namespace", cluster.Namespace, "name", cluster.Name)
	cNbMaster := cluster.Spec.MasterSize
	cReplicaFactor := cluster.Spec.ClusterReplicas
	rCluster, nodes, err := newRedisCluster(clusterInfos, cluster)
	if err != nil {
		return Cluster.Wrap(err, "newRedisCluster")
	}

	// First, we define the new masters
	newMasters, curMasters, allMaster, err := clustering.DispatchMasters(rCluster, nodes, cNbMaster)
	if err != nil {
		return Cluster.Wrap(err, "DispatchMasters")
	}
	logger.V(4).Info("DispatchMasters Info", "newMasters", newMasters, "curMasters", curMasters, "allMaster", allMaster)

	// Second select Node that is already a slave
	currentSlaveNodes := nodes.FilterByFunc(redisutil.IsSlave)

	// New slaves are slaves which is currently a master with no slots
	newSlave := nodes.FilterByFunc(func(nodeA *redisutil.Node) bool {
		for _, nodeB := range newMasters {
			if nodeA.ID == nodeB.ID {
				return false
			}
		}
		for _, nodeB := range currentSlaveNodes {
			if nodeA.ID == nodeB.ID {
				return false
			}
		}
		return true
	})

	// Depending on whether we scale up or down, we will dispatch slaves before/after the dispatch of slots
	if cNbMaster < int32(len(curMasters)) {
		logger.Info("current masters > specification", "curMasters", curMasters, "masterSize", cNbMaster)
		// this happens usually after a scale down of the cluster
		// we should dispatch slots before dispatching slaves
		if err := clustering.DispatchSlotToNewMasters(rCluster, admin, newMasters, curMasters, allMaster); err != nil {
			return Cluster.Wrap(err, "DispatchSlotToNewMasters")
		}

		// assign master/slave roles
		newRedisSlavesByMaster, bestEffort := clustering.PlaceSlaves(rCluster, newMasters, currentSlaveNodes, newSlave, cReplicaFactor)
		if bestEffort {
			rCluster.NodesPlacement = redisv1alpha1.NodesPlacementInfoBestEffort
		}

		if err := clustering.AttachingSlavesToMaster(rCluster, admin, newRedisSlavesByMaster); err != nil {
			return Cluster.Wrap(err, "AttachingSlavesToMaster")
		}
	} else {
		logger.Info("current masters < specification", "curMasters", curMasters, "masterSize", cNbMaster)
		// We are scaling up the nbmaster or the nbmaster doesn't change.
		// assign master/slave roles
		newRedisSlavesByMaster, bestEffort := clustering.PlaceSlaves(rCluster, newMasters, currentSlaveNodes, newSlave, cReplicaFactor)
		if bestEffort {
			rCluster.NodesPlacement = redisv1alpha1.NodesPlacementInfoBestEffort
		}

		if err := clustering.AttachingSlavesToMaster(rCluster, admin, newRedisSlavesByMaster); err != nil {
			return Cluster.Wrap(err, "AttachingSlavesToMaster")
		}

		if err := clustering.DispatchSlotToNewMasters(rCluster, admin, newMasters, curMasters, allMaster); err != nil {
			return Cluster.Wrap(err, "DispatchSlotToNewMasters")
		}
	}

	return nil
}
