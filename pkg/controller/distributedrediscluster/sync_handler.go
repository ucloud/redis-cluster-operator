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
	if err := r.validate(cluster, ctx.reqLogger); err != nil {
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
	if err := r.ensurer.EnsureRedisStatefulsets(cluster, backup, labels); err != nil {
		return Kubernetes.Wrap(err, "EnsureRedisStatefulsets")
	}
	if err := r.ensurer.EnsureRedisHeadLessSvcs(cluster, labels); err != nil {
		return Kubernetes.Wrap(err, "EnsureRedisHeadLessSvcs")
	}
	if err := r.ensurer.EnsureRedisSvc(cluster, labels); err != nil {
		return Kubernetes.Wrap(err, "EnsureRedisSvc")
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

func (r *ReconcileDistributedRedisCluster) validate(cluster *redisv1alpha1.DistributedRedisCluster, reqLogger logr.Logger) error {
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
	cluster.Validate(reqLogger)
	return nil
}

func (r *ReconcileDistributedRedisCluster) waitForClusterJoin(ctx *syncContext) error {
	//logger.Info(">>> Assign a different config epoch to each node")
	//err := admin.SetConfigEpoch()
	//if err != nil {
	//	return Redis.Wrap(err, "SetConfigEpoch")
	//}
	if infos, err := ctx.admin.GetClusterInfos(); err == nil {
		ctx.reqLogger.V(6).Info("debug waitForClusterJoin", "cluster infos", infos)
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

	_, err = ctx.admin.GetClusterInfos()
	if err != nil {
		return Requeue.Wrap(err, "wait for cluster join")
	}
	return nil
}

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
	} else if cluster.Status.MinReplicationFactor < cluster.Spec.ClusterReplicas {
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

func (r *ReconcileDistributedRedisCluster) syncCluster(ctx *syncContext) error {
	cluster := ctx.cluster
	admin := ctx.admin
	clusterInfos := ctx.clusterInfos
	expectMasterNum := cluster.Spec.MasterSize
	rCluster, nodes, err := newRedisCluster(clusterInfos, cluster)
	if err != nil {
		return Cluster.Wrap(err, "newRedisCluster")
	}
	clusterCtx := clustering.NewCtx(rCluster, nodes, ctx.reqLogger)
	if err := clusterCtx.DispatchMasters(); err != nil {
		return Cluster.Wrap(err, "DispatchMasters")
	}
	curMasters := clusterCtx.GetCurrentMasters()
	newMasters := clusterCtx.GetNewMasters()
	if len(curMasters) == 0 {
		ctx.reqLogger.Info("Creating cluster")
		if err := clusterCtx.PlaceSlaves(); err != nil {
			return Cluster.Wrap(err, "PlaceSlaves")

		}
		newRedisSlavesByMaster := clusterCtx.GetSlaves()
		if err := clustering.AttachingSlavesToMaster(rCluster, admin, newRedisSlavesByMaster); err != nil {
			return Cluster.Wrap(err, "AttachingSlavesToMaster")
		}

		if err := clustering.AllocSlots(admin, newMasters); err != nil {
			return Cluster.Wrap(err, "AllocSlots")
		}
	} else if len(newMasters) > len(curMasters) {
		ctx.reqLogger.Info("Scaling up")
		if err := clusterCtx.PlaceSlaves(); err != nil {
			return Cluster.Wrap(err, "PlaceSlaves")

		}
		newRedisSlavesByMaster := clusterCtx.GetSlaves()
		if err := clustering.AttachingSlavesToMaster(rCluster, admin, newRedisSlavesByMaster); err != nil {
			return Cluster.Wrap(err, "AttachingSlavesToMaster")
		}

		if err := clustering.RebalancedCluster(admin, newMasters); err != nil {
			return Cluster.Wrap(err, "RebalancedCluster")
		}
	} else if cluster.Status.MinReplicationFactor < cluster.Spec.ClusterReplicas {
		ctx.reqLogger.Info("Scaling slave")
		if err := clusterCtx.PlaceSlaves(); err != nil {
			return Cluster.Wrap(err, "PlaceSlaves")

		}
		newRedisSlavesByMaster := clusterCtx.GetSlaves()
		if err := clustering.AttachingSlavesToMaster(rCluster, admin, newRedisSlavesByMaster); err != nil {
			return Cluster.Wrap(err, "AttachingSlavesToMaster")
		}
	} else if len(curMasters) > int(expectMasterNum) {
		ctx.reqLogger.Info("Scaling down")
	}
	return nil
}
