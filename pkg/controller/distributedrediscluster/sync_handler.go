package distributedrediscluster

import (
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
	"github.com/ucloud/redis-cluster-operator/pkg/controller/clustering"
	"github.com/ucloud/redis-cluster-operator/pkg/controller/manager"
	"github.com/ucloud/redis-cluster-operator/pkg/k8sutil"
	"github.com/ucloud/redis-cluster-operator/pkg/redisutil"
)

const (
	requeueAfter  = 10 * time.Second
	requeueEnsure = 60 * time.Second
)

type syncContext struct {
	cluster      *redisv1alpha1.DistributedRedisCluster
	clusterInfos *redisutil.ClusterInfos
	admin        redisutil.IAdmin
	healer       manager.IHeal
	pods         []*corev1.Pod
	reqLogger    logr.Logger
}

func (r *ReconcileDistributedRedisCluster) ensureCluster(ctx *syncContext) error {
	cluster := ctx.cluster
	if err := r.validate(cluster); err != nil {
		if k8sutil.IsRequestRetryable(err) {
			return Kubernetes.Wrap(err, "Validate")
		}
		return StopRetry.Wrap(err, "stop retry")
	}
	labels := getLabels(cluster)
	var backup *redisv1alpha1.RedisClusterBackup
	var err error
	if cluster.Spec.Init != nil {
		backup, err = r.crController.GetRedisClusterBackup(cluster.Spec.Init.BackupSource.Namespace, cluster.Spec.Init.BackupSource.Name)
		if err != nil {
			return err
		}
	}
	if err := r.ensurer.EnsureRedisConfigMap(cluster, labels); err != nil {
		return Kubernetes.Wrap(err, "EnsureRedisConfigMap")
	}
	if err := r.ensurer.EnsureRedisStatefulset(cluster, backup, labels); err != nil {
		return Kubernetes.Wrap(err, "EnsureRedisStatefulset")
	}
	if err := r.ensurer.EnsureRedisHeadLessSvc(cluster, labels); err != nil {
		return Kubernetes.Wrap(err, "EnsureRedisHeadLessSvc")
	}
	if err := r.ensurer.EnsureRedisOSMSecret(cluster, backup, labels); err != nil {
		if k8sutil.IsRequestRetryable(err) {
			return Kubernetes.Wrap(err, "EnsureRedisOSMSecret")
		}
		return StopRetry.Wrap(err, "stop retry")
	}
	return nil
}

func (r *ReconcileDistributedRedisCluster) waitPodReady(ctx *syncContext) error {
	if _, err := ctx.healer.FixTerminatingPods(ctx.cluster, 5*time.Minute); err != nil {
		return Kubernetes.Wrap(err, "FixTerminatingPods")
	}
	if err := r.checker.CheckRedisNodeNum(ctx.cluster); err != nil {
		return Requeue.Wrap(err, "CheckRedisNodeNum")
	}

	return nil
}

func (r *ReconcileDistributedRedisCluster) validate(cluster *redisv1alpha1.DistributedRedisCluster) error {
	initSpec := cluster.Spec.Init
	if initSpec != nil {
		if initSpec.BackupSource == nil {
			return fmt.Errorf("backupSource is required")
		}
		backup, err := r.crController.GetRedisClusterBackup(initSpec.BackupSource.Namespace, initSpec.BackupSource.Name)
		if err != nil {
			return err
		}
		if backup.Status.Phase != redisv1alpha1.BackupPhaseSucceeded {
			return fmt.Errorf("backup is still running")
		}
		if cluster.Spec.Image == "" {
			cluster.Spec.Image = backup.Status.ClusterImage
		}
		cluster.Spec.MasterSize = backup.Status.MasterSize
		if cluster.Status.RestoreSucceeded <= 0 {
			cluster.Spec.ClusterReplicas = 0
		} else {
			cluster.Spec.ClusterReplicas = backup.Status.ClusterReplicas
		}
	}
	cluster.Validate()
	return nil
}

func (r *ReconcileDistributedRedisCluster) waitForClusterJoin(ctx *syncContext) error {
	//logger.Info(">>> Assign a different config epoch to each node")
	//err := admin.SetConfigEpoch()
	//if err != nil {
	//	return Redis.Wrap(err, "SetConfigEpoch")
	//}
	if _, err := ctx.admin.GetClusterInfos(); err == nil {
		return nil
	}
	var firstNode *redisutil.Node
	for _, nodeInfo := range ctx.clusterInfos.Infos {
		firstNode = nodeInfo.Node
		break
	}
	ctx.reqLogger.Info(">>> Sending CLUSTER MEET messages to join the cluster")
	err := ctx.admin.AttachNodeToCluster(firstNode.IPPort())
	if err != nil {
		return Redis.Wrap(err, "AttachNodeToCluster")
	}
	// Give one second for the join to start, in order to avoid that
	// waiting for cluster join will find all the nodes agree about
	// the config as they are still empty with unassigned slots.
	time.Sleep(1 * time.Second)
	if _, err := ctx.healer.FixFailedNodes(ctx.cluster, ctx.clusterInfos, ctx.admin); err != nil {
		return Cluster.Wrap(err, "FixFailedNodes")
	}
	if _, err := ctx.healer.FixUntrustedNodes(ctx.cluster, ctx.clusterInfos, ctx.admin); err != nil {
		return Cluster.Wrap(err, "FixUntrustedNodes")
	}
	_, err = ctx.admin.GetClusterInfos()
	if err != nil {
		return Requeue.Wrap(err, "wait for cluster join")
	}
	return nil
}

/*
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
}*/

/*
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

	if err = admin.SetConfigIfNeed(cluster.Spec.Config); err != nil {
		return Redis.Wrap(err, "SetConfigIfNeed")
	}

	return nil
}*/

func (r *ReconcileDistributedRedisCluster) sync(ctx *syncContext) error {
	cluster := ctx.cluster
	admin := ctx.admin
	clusterInfos := ctx.clusterInfos
	if err := admin.SetConfigIfNeed(cluster.Spec.Config); err != nil {
		return Redis.Wrap(err, "SetConfigIfNeed")
	}

	cNbMaster := cluster.Spec.MasterSize
	cReplicaFactor := cluster.Spec.ClusterReplicas
	rCluster, nodes, err := newRedisCluster(clusterInfos, cluster)
	if err != nil {
		return Cluster.Wrap(err, "newRedisCluster")
	}

	//currentMasterNodes := nodes.FilterByFunc(redisutil.IsMasterWithSlot)
	//if len(currentMasterNodes) == int(cluster.Spec.MasterSize) {
	//	logger.V(3).Info("cluster ok")
	//	return nil
	//}

	// First, we define the new masters
	newMasters, curMasters, allMaster, err := clustering.DispatchMasters(rCluster, nodes, cNbMaster)
	if err != nil {
		return Cluster.Wrap(err, "DispatchMasters")
	}
	ctx.reqLogger.V(4).Info("DispatchMasters Info", "newMasters", newMasters, "curMasters", curMasters, "allMaster", allMaster)

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

	if 0 == len(curMasters) {
		ctx.reqLogger.Info("Creating cluster")
		newRedisSlavesByMaster, bestEffort := clustering.PlaceSlaves(rCluster, newMasters, currentSlaveNodes, newSlave, cReplicaFactor)
		if bestEffort {
			rCluster.NodesPlacement = redisv1alpha1.NodesPlacementInfoBestEffort
		}

		if err := clustering.AttachingSlavesToMaster(rCluster, admin, newRedisSlavesByMaster); err != nil {
			return Cluster.Wrap(err, "AttachingSlavesToMaster")
		}

		if err := clustering.AllocSlots(admin, newMasters); err != nil {
			return Cluster.Wrap(err, "AllocSlots")
		}
	} else if len(newMasters) > len(curMasters) {
		ctx.reqLogger.Info("Scaling cluster")
		newRedisSlavesByMaster, bestEffort := clustering.PlaceSlaves(rCluster, newMasters, currentSlaveNodes, newSlave, cReplicaFactor)
		if bestEffort {
			rCluster.NodesPlacement = redisv1alpha1.NodesPlacementInfoBestEffort
		}

		if err := clustering.AttachingSlavesToMaster(rCluster, admin, newRedisSlavesByMaster); err != nil {
			return Cluster.Wrap(err, "AttachingSlavesToMaster")
		}

		if err := clustering.RebalancedCluster(admin, newMasters); err != nil {
			return Cluster.Wrap(err, "RebalancedCluster")
		}
	} else if len(newMasters) == len(curMasters) {
		newRedisSlavesByMaster, bestEffort := clustering.PlaceSlaves(rCluster, newMasters, currentSlaveNodes, newSlave, cReplicaFactor)
		if bestEffort {
			rCluster.NodesPlacement = redisv1alpha1.NodesPlacementInfoBestEffort
		}

		if err := clustering.AttachingSlavesToMaster(rCluster, admin, newRedisSlavesByMaster); err != nil {
			return Cluster.Wrap(err, "AttachingSlavesToMaster")
		}
	}

	return nil
}
