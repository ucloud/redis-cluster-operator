package k8sutil

import (
	"context"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
)

// ICustomResource defines the interface that uses to update cr status
type ICustomResource interface {
	// UpdateCRStatus update the RedisCluster status
	UpdateCRStatus(runtime.Object) error
	UpdateCR(runtime.Object) error
	GetRedisClusterBackup(namespace, name string) (*redisv1alpha1.RedisClusterBackup, error)
	GetDistributedRedisCluster(namespace, name string) (*redisv1alpha1.DistributedRedisCluster, error)
}

type clusterControl struct {
	client client.Client
}

// NewCRControl creates a concrete implementation of the
// ICustomResource.
func NewCRControl(client client.Client) ICustomResource {
	return &clusterControl{client: client}
}

func (c *clusterControl) UpdateCRStatus(obj runtime.Object) error {
	return c.client.Status().Update(context.TODO(), obj)
}

func (c *clusterControl) UpdateCR(obj runtime.Object) error {
	return c.client.Update(context.TODO(), obj)
}

func (c *clusterControl) GetRedisClusterBackup(namespace, name string) (*redisv1alpha1.RedisClusterBackup, error) {
	backup := &redisv1alpha1.RedisClusterBackup{}
	if err := c.client.Get(context.TODO(), types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, backup); err != nil {
		return nil, err
	}
	return backup, nil
}

func (c *clusterControl) GetDistributedRedisCluster(namespace, name string) (*redisv1alpha1.DistributedRedisCluster, error) {
	drc := &redisv1alpha1.DistributedRedisCluster{}
	if err := c.client.Get(context.TODO(), types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, drc); err != nil {
		return nil, err
	}
	return drc, nil
}
