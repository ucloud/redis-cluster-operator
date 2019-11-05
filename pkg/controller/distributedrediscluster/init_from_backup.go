package distributedrediscluster

import (
	"github.com/go-logr/logr"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
)

func (r *ReconcileDistributedRedisCluster) initFromBackup(reqLogger logr.Logger, cluster *redisv1alpha1.DistributedRedisCluster) error {
	return nil
}
