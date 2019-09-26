package k8sutil

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
)

// ICluster defines the interface that uses to update cluster
type ICluster interface {
	// UpdateCluster update the RedisCluster status
	UpdateCluster(cluster *redisv1alpha1.DistributedRedisCluster) error
}

type clusterControl struct {
	client client.Client
}

// NewClusterControl creates a concrete implementation of the
// ICluster.
func NewClusterControl(client client.Client) ICluster {
	return &clusterControl{client: client}
}

func (c *clusterControl) UpdateCluster(cluster *redisv1alpha1.DistributedRedisCluster) error {
	return c.client.Status().Update(context.TODO(), cluster)
}
