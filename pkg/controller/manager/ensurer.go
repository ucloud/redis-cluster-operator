package manager

import (
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
	"github.com/ucloud/redis-cluster-operator/pkg/k8sutil"
	"github.com/ucloud/redis-cluster-operator/pkg/resources/configmaps"
	"github.com/ucloud/redis-cluster-operator/pkg/resources/services"
	"github.com/ucloud/redis-cluster-operator/pkg/resources/statefulsets"
)

type IEnsureResource interface {
	EnsureRedisStatefulset(cluster *redisv1alpha1.DistributedRedisCluster, labels map[string]string) error
	EnsureRedisHeadLessSvc(cluster *redisv1alpha1.DistributedRedisCluster, labels map[string]string) error
	EnsureRedisConfigMap(cluster *redisv1alpha1.DistributedRedisCluster, labels map[string]string) error
}

type realEnsureResource struct {
	statefulSetClient k8sutil.IStatefulSetControl
	svcClient         k8sutil.IServiceControl
	configMapClient   k8sutil.IConfigMapControl
	logger            logr.Logger
}

func NewEnsureResource(client client.Client, logger logr.Logger) IEnsureResource {
	return &realEnsureResource{
		statefulSetClient: k8sutil.NewStatefulSetController(client),
		svcClient:         k8sutil.NewServiceController(client),
		configMapClient:   k8sutil.NewConfigMapController(client),
		logger:            logger,
	}
}

func (r *realEnsureResource) EnsureRedisStatefulset(cluster *redisv1alpha1.DistributedRedisCluster, labels map[string]string) error {
	name := statefulsets.ClusterStatefulSetName(cluster.Name)
	ss, err := r.statefulSetClient.GetStatefulSet(cluster.Namespace, name)
	if err == nil {
		if (cluster.Spec.MasterSize * (cluster.Spec.ClusterReplicas + 1)) != *ss.Spec.Replicas {
			r.logger.WithValues("StatefulSet.Namespace", cluster.Namespace, "StatefulSet.Name", name).
				Info("scaling statefulSet")
			return r.statefulSetClient.UpdateStatefulSet(statefulsets.NewStatefulSetForCR(cluster, labels))
		}
	} else if err != nil && errors.IsNotFound(err) {
		r.logger.WithValues("StatefulSet.Namespace", cluster.Namespace, "StatefulSet.Name", name).
			Info("creating a new statefulSet")
		ss := statefulsets.NewStatefulSetForCR(cluster, labels)
		return r.statefulSetClient.CreateStatefulSet(ss)
	}
	return err
}

func (r *realEnsureResource) EnsureRedisHeadLessSvc(cluster *redisv1alpha1.DistributedRedisCluster, labels map[string]string) error {
	_, err := r.svcClient.GetService(cluster.Namespace, cluster.Spec.ServiceName)
	if err != nil && errors.IsNotFound(err) {
		r.logger.WithValues("Service.Namespace", cluster.Namespace, "Service.Name", cluster.Spec.ServiceName).
			Info("creating a new headless service")
		svc := services.NewHeadLessSvcForCR(cluster, labels)
		return r.svcClient.CreateService(svc)
	}
	return err
}

func (r *realEnsureResource) EnsureRedisConfigMap(cluster *redisv1alpha1.DistributedRedisCluster, labels map[string]string) error {
	cmName := configmaps.RedisConfigMapName(cluster.Name)
	_, err := r.configMapClient.GetConfigMap(cluster.Namespace, cmName)
	if err != nil && errors.IsNotFound(err) {
		r.logger.WithValues("ConfigMap.Namespace", cluster.Namespace, "ConfigMap.Name", cmName).
			Info("creating a new configMap")
		cm := configmaps.NewConfigMapForCR(cluster, labels)
		return r.configMapClient.CreateConfigMap(cm)
	} else if err != nil {
		return err
	}
	return nil
}
