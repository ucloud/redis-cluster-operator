package distributedrediscluster

import (
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
	"github.com/ucloud/redis-cluster-operator/pkg/config"
	"github.com/ucloud/redis-cluster-operator/pkg/controller/clustering"
	"github.com/ucloud/redis-cluster-operator/pkg/controller/manager"
	"github.com/ucloud/redis-cluster-operator/pkg/k8sutil"
	"github.com/ucloud/redis-cluster-operator/pkg/redisutil"
	"github.com/ucloud/redis-cluster-operator/pkg/resources/statefulsets"
)

const (
	requeueAfter = 10 * time.Second
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
	if err := r.validateAndSetDefault(cluster, ctx.reqLogger); err != nil {
		if k8sutil.IsRequestRetryable(err) {
			return Kubernetes.Wrap(err, "Validate")
		}
		return StopRetry.Wrap(err, "stop retry")
	}

	// Redis only load db from append only file when AOF ON, because of
	// we only backed up the RDB file when doing data backup, so we set
	// "appendonly no" force here when do restore.
	dbLoadedFromDiskWhenRestore(cluster, ctx.reqLogger)
	labels := getLabels(cluster)
	if err := r.ensurer.EnsureRedisConfigMap(cluster, labels); err != nil {
		return Kubernetes.Wrap(err, "EnsureRedisConfigMap")
	}

	if err := r.resetClusterPassword(ctx); err != nil {
		return Cluster.Wrap(err, "ResetPassword")
	}

	if updated, err := r.ensurer.EnsureRedisStatefulsets(cluster, labels); err != nil {
		ctx.reqLogger.Error(err, "EnsureRedisStatefulSets")
		return Kubernetes.Wrap(err, "EnsureRedisStatefulSets")
	} else if updated {
		// update cluster status = RollingUpdate immediately when cluster's image or resource or password changed
		SetClusterUpdating(&cluster.Status, "cluster spec updated")
		r.crController.UpdateCRStatus(cluster)
		waiter := &waitStatefulSetUpdating{
			name:                  "waitStatefulSetUpdating",
			timeout:               30 * time.Second * time.Duration(cluster.Spec.ClusterReplicas+2),
			tick:                  5 * time.Second,
			statefulSetController: r.statefulSetController,
			cluster:               cluster,
		}
		if err := waiting(waiter, ctx.reqLogger); err != nil {
			return err
		}
	}
	if err := r.ensurer.EnsureRedisHeadLessSvcs(cluster, labels); err != nil {
		return Kubernetes.Wrap(err, "EnsureRedisHeadLessSvcs")
	}
	if err := r.ensurer.EnsureRedisSvc(cluster, labels); err != nil {
		return Kubernetes.Wrap(err, "EnsureRedisSvc")
	}
	if err := r.ensurer.EnsureRedisRCloneSecret(cluster, labels); err != nil {
		if k8sutil.IsRequestRetryable(err) {
			return Kubernetes.Wrap(err, "EnsureRedisRCloneSecret")
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

func (r *ReconcileDistributedRedisCluster) validateAndSetDefault(cluster *redisv1alpha1.DistributedRedisCluster, reqLogger logr.Logger) error {
	var update bool
	var err error

	if cluster.IsRestoreFromBackup() && cluster.ShouldInitRestorePhase() {
		update, err = r.initRestore(cluster, reqLogger)
		if err != nil {
			return err
		}
	}

	if cluster.IsRestoreFromBackup() && (cluster.IsRestoreRunning() || cluster.IsRestoreRestarting()) {
		// Set ClusterReplicas = 0, only start master node in first reconcile loop when do restore
		cluster.Spec.ClusterReplicas = 0
	}

	updateDefault := cluster.DefaultSpec(reqLogger)
	if update || updateDefault {
		return r.crController.UpdateCR(cluster)
	}

	return nil
}

func dbLoadedFromDiskWhenRestore(cluster *redisv1alpha1.DistributedRedisCluster, reqLogger logr.Logger) {
	if cluster.IsRestoreFromBackup() && !cluster.IsRestored() {
		if cluster.Spec.Config != nil {
			reqLogger.Info("force appendonly = no when do restore")
			cluster.Spec.Config["appendonly"] = "no"
		}
	}
}

func (r *ReconcileDistributedRedisCluster) initRestore(cluster *redisv1alpha1.DistributedRedisCluster, reqLogger logr.Logger) (bool, error) {
	update := false
	if cluster.Status.Restore.Backup == nil {
		initSpec := cluster.Spec.Init
		backup, err := r.crController.GetRedisClusterBackup(initSpec.BackupSource.Namespace, initSpec.BackupSource.Name)
		if err != nil {
			reqLogger.Error(err, "GetRedisClusterBackup")
			return update, err
		}
		if backup.Status.Phase != redisv1alpha1.BackupPhaseSucceeded {
			reqLogger.Error(nil, "backup is still running")
			return update, fmt.Errorf("backup is still running")
		}
		cluster.Status.Restore.Backup = backup
		cluster.Status.Restore.Phase = redisv1alpha1.RestorePhaseRunning
		if err := r.crController.UpdateCRStatus(cluster); err != nil {
			return update, err
		}
	}
	backup := cluster.Status.Restore.Backup
	if cluster.Spec.Image == "" {
		cluster.Spec.Image = backup.Status.ClusterImage
		update = true
	}
	if cluster.Spec.MasterSize != backup.Status.MasterSize {
		cluster.Spec.MasterSize = backup.Status.MasterSize
		update = true
	}

	return update, nil
}

func (r *ReconcileDistributedRedisCluster) waitForClusterJoin(ctx *syncContext) error {
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

func (r *ReconcileDistributedRedisCluster) syncCluster(ctx *syncContext) error {
	cluster := ctx.cluster
	admin := ctx.admin
	clusterInfos := ctx.clusterInfos
	expectMasterNum := cluster.Spec.MasterSize
	rCluster, nodes, err := newRedisCluster(clusterInfos, cluster)
	if err != nil {
		return Cluster.Wrap(err, "newRedisCluster")
	}
	clusterCtx := clustering.NewCtx(rCluster, nodes, cluster.Spec.MasterSize, cluster.Name, ctx.reqLogger)
	if err := clusterCtx.DispatchMasters(); err != nil {
		return Cluster.Wrap(err, "DispatchMasters")
	}
	curMasters := clusterCtx.GetCurrentMasters()
	newMasters := clusterCtx.GetNewMasters()
	ctx.reqLogger.Info("masters", "newMasters", len(newMasters), "curMasters", len(curMasters))
	if len(curMasters) == 0 {
		ctx.reqLogger.Info("Creating cluster")
		if err := clusterCtx.PlaceSlaves(); err != nil {
			return Cluster.Wrap(err, "PlaceSlaves")

		}
		if err := clusterCtx.AttachingSlavesToMaster(admin); err != nil {
			return Cluster.Wrap(err, "AttachingSlavesToMaster")
		}

		if err := clusterCtx.AllocSlots(admin, newMasters); err != nil {
			return Cluster.Wrap(err, "AllocSlots")
		}
	} else if len(newMasters) > len(curMasters) {
		ctx.reqLogger.Info("Scaling up")
		if err := clusterCtx.PlaceSlaves(); err != nil {
			return Cluster.Wrap(err, "PlaceSlaves")

		}
		if err := clusterCtx.AttachingSlavesToMaster(admin); err != nil {
			return Cluster.Wrap(err, "AttachingSlavesToMaster")
		}

		if err := clusterCtx.RebalancedCluster(admin, newMasters); err != nil {
			return Cluster.Wrap(err, "RebalancedCluster")
		}
	} else if cluster.Status.MinReplicationFactor < cluster.Spec.ClusterReplicas {
		ctx.reqLogger.Info("Scaling slave")
		if err := clusterCtx.PlaceSlaves(); err != nil {
			return Cluster.Wrap(err, "PlaceSlaves")

		}
		if err := clusterCtx.AttachingSlavesToMaster(admin); err != nil {
			return Cluster.Wrap(err, "AttachingSlavesToMaster")
		}
	} else if len(curMasters) > int(expectMasterNum) {
		ctx.reqLogger.Info("Scaling down")
		var allMaster redisutil.Nodes
		allMaster = append(allMaster, newMasters...)
		allMaster = append(allMaster, curMasters...)
		if err := clusterCtx.DispatchSlotToNewMasters(admin, newMasters, curMasters, allMaster); err != nil {
			return err
		}
		if err := r.scalingDown(ctx, len(curMasters), clusterCtx.GetStatefulsetNodes()); err != nil {
			return err
		}
	}
	return nil
}

func (r *ReconcileDistributedRedisCluster) scalingDown(ctx *syncContext, currentMasterNum int, statefulSetNodes map[string]redisutil.Nodes) error {
	cluster := ctx.cluster
	SetClusterRebalancing(&cluster.Status,
		fmt.Sprintf("scale down, currentMasterSize: %d, expectMasterSize %d", currentMasterNum, cluster.Spec.MasterSize))
	r.crController.UpdateCRStatus(cluster)
	admin := ctx.admin
	expectMasterNum := int(cluster.Spec.MasterSize)
	for i := currentMasterNum - 1; i >= expectMasterNum; i-- {
		stsName := statefulsets.ClusterStatefulSetName(cluster.Name, i)
		for _, node := range statefulSetNodes[stsName] {
			admin.Connections().Remove(node.IPPort())
		}
	}
	for i := currentMasterNum - 1; i >= expectMasterNum; i-- {
		stsName := statefulsets.ClusterStatefulSetName(cluster.Name, i)
		ctx.reqLogger.Info("scaling down", "statefulSet", stsName)
		sts, err := r.statefulSetController.GetStatefulSet(cluster.Namespace, stsName)
		if err != nil {
			return Kubernetes.Wrap(err, "GetStatefulSet")
		}
		for _, node := range statefulSetNodes[stsName] {
			ctx.reqLogger.Info("forgetNode", "id", node.ID, "ip", node.IP, "role", node.GetRole())
			if len(node.Slots) > 0 {
				return Redis.New(fmt.Sprintf("node %s is not empty! Reshard data away and try again", node.String()))
			}
			if err := admin.ForgetNode(node.ID); err != nil {
				return Redis.Wrap(err, "ForgetNode")
			}
		}
		// remove resource
		if err := r.statefulSetController.DeleteStatefulSetByName(cluster.Namespace, stsName); err != nil {
			ctx.reqLogger.Error(err, "DeleteStatefulSetByName", "statefulSet", stsName)
		}
		svcName := statefulsets.ClusterHeadlessSvcName(cluster.Name, i)
		if err := r.serviceController.DeleteServiceByName(cluster.Namespace, svcName); err != nil {
			ctx.reqLogger.Error(err, "DeleteServiceByName", "service", svcName)
		}
		if err := r.pdbController.DeletePodDisruptionBudgetByName(cluster.Namespace, stsName); err != nil {
			ctx.reqLogger.Error(err, "DeletePodDisruptionBudgetByName", "pdb", stsName)
		}
		if err := r.pvcController.DeletePvcByLabels(cluster.Namespace, sts.Labels); err != nil {
			ctx.reqLogger.Error(err, "DeletePvcByLabels", "labels", sts.Labels)
		}
		// wait pod Terminating
		waiter := &waitPodTerminating{
			name:                  "waitPodTerminating",
			statefulSet:           stsName,
			timeout:               30 * time.Second * time.Duration(cluster.Spec.ClusterReplicas+2),
			tick:                  5 * time.Second,
			statefulSetController: r.statefulSetController,
			cluster:               cluster,
		}
		if err := waiting(waiter, ctx.reqLogger); err != nil {
			ctx.reqLogger.Error(err, "waitPodTerminating")
		}

	}
	return nil
}

func (r *ReconcileDistributedRedisCluster) resetClusterPassword(ctx *syncContext) error {
	if err := r.checker.CheckRedisNodeNum(ctx.cluster); err == nil {
		namespace := ctx.cluster.Namespace
		name := ctx.cluster.Name
		sts, err := r.statefulSetController.GetStatefulSet(namespace, statefulsets.ClusterStatefulSetName(name, 0))
		if err != nil {
			return err
		}

		if !statefulsets.IsPasswordChanged(ctx.cluster, sts) {
			return nil
		}

		SetClusterResetPassword(&ctx.cluster.Status, "updating cluster's password")
		r.crController.UpdateCRStatus(ctx.cluster)

		matchLabels := getLabels(ctx.cluster)
		redisClusterPods, err := r.statefulSetController.GetStatefulSetPodsByLabels(namespace, matchLabels)
		if err != nil {
			return err
		}

		oldPassword, err := statefulsets.GetOldRedisClusterPassword(r.client, sts)
		if err != nil {
			return err
		}

		newPassword, err := statefulsets.GetClusterPassword(r.client, ctx.cluster)
		if err != nil {
			return err
		}

		podSet := clusterPods(redisClusterPods.Items)
		admin, err := newRedisAdmin(podSet, oldPassword, config.RedisConf(), ctx.reqLogger)
		if err != nil {
			return err
		}
		defer admin.Close()

		// Update the password recorded in the file /data/redis_password, redis pod preStop hook
		// need /data/redis_password do CLUSTER FAILOVER
		cmd := fmt.Sprintf("echo %s > /data/redis_password", newPassword)
		if err := r.execer.ExecCommandInPodSet(podSet, "/bin/sh", "-c", cmd); err != nil {
			return err
		}

		// Reset all redis pod's password.
		if err := admin.ResetPassword(newPassword); err != nil {
			return err
		}
	}
	return nil
}
