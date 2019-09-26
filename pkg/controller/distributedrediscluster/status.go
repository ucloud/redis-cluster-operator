package distributedrediscluster

import (
	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
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

func buildClusterStatus(err error) *redisv1alpha1.DistributedRedisClusterStatus{
	status := &redisv1alpha1.DistributedRedisClusterStatus{}
	if err == nil {
		status.Status = redisv1alpha1.ClusterStatusOK
		status.Reason = "OK"
		return status
	}
	switch GetType(err) {
	case Requeue:
		status.Status = redisv1alpha1.ClusterStatusCreating
		status.Reason = err.Error()
	default:
		status.Status = redisv1alpha1.ClusterStatusKO
		status.Reason = err.Error()
	}
	return status
}

func (r *ReconcileDistributedRedisCluster) updateClusterIfNeed(cluster *redisv1alpha1.DistributedRedisCluster, newStatus *redisv1alpha1.DistributedRedisClusterStatus) {
	if cluster.Status.Status != newStatus.Status || cluster.Status.Reason != newStatus.Reason {
		cluster.Status = *newStatus
		r.clusterStatusController.UpdateCluster(cluster)
	}
}