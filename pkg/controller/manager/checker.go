package manager

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
	"github.com/ucloud/redis-cluster-operator/pkg/k8sutil"
	"github.com/ucloud/redis-cluster-operator/pkg/resources/statefulsets"
)

type ICheck interface {
	CheckRedisNodeNum(*redisv1alpha1.DistributedRedisCluster) error
	CheckRedisMasterNum(*redisv1alpha1.DistributedRedisCluster) error
}

type realCheck struct {
	statefulSetClient  k8sutil.IStatefulSetControl
	clusterStatefulSet *appsv1.StatefulSet
}

func NewCheck(client client.Client) ICheck {
	return &realCheck{
		statefulSetClient: k8sutil.NewStatefulSetController(client),
	}
}

func (c *realCheck) init(cluster *redisv1alpha1.DistributedRedisCluster) error {
	ss, err := c.statefulSetClient.GetStatefulSet(cluster.Namespace, statefulsets.ClusterStatefulSetName(cluster.Name))
	if err != nil {
		return err
	}
	c.clusterStatefulSet = ss
	return nil
}

func (c *realCheck) CheckRedisNodeNum(cluster *redisv1alpha1.DistributedRedisCluster) error {
	err := c.init(cluster)
	if err != nil {
		return err
	}
	expectNodeNum := cluster.Spec.MasterSize * cluster.Spec.ClusterReplicas
	if expectNodeNum != *c.clusterStatefulSet.Spec.Replicas {
		return fmt.Errorf("number of redis pods is different from specification")
	}
	if expectNodeNum != c.clusterStatefulSet.Status.ReadyReplicas {
		return fmt.Errorf("redis pods are not all ready")
	}

	return nil
}

func (c *realCheck) CheckRedisMasterNum(cluster *redisv1alpha1.DistributedRedisCluster) error {
	if c.clusterStatefulSet == nil {
		c.init(cluster)
	}
	if cluster.Spec.MasterSize != cluster.Status.NumberOfMaster {
		return fmt.Errorf("number of redis master different from specification")
	}
	return nil
}

//
//func (c *realCheck) CheckRedisClusterIsEmpty(cluster *redisv1alpha1.DistributedRedisCluster, admin redisutil.IAdmin) (bool, error) {
//
//}
