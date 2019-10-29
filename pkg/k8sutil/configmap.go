package k8sutil

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IConfigMapControl defines the interface that uses to create, update, and delete ConfigMaps.
type IConfigMapControl interface {
	// CreateConfigMap creates a ConfigMap in a DistributedRedisCluster.
	CreateConfigMap(*corev1.ConfigMap) error
	// UpdateConfigMap updates a ConfigMap in a DistributedRedisCluster.
	UpdateConfigMap(*corev1.ConfigMap) error
	// DeleteConfigMap deletes a ConfigMap in a DistributedRedisCluster.
	DeleteConfigMap(*corev1.ConfigMap) error
	// GetConfigMap get ConfigMap in a DistributedRedisCluster.
	GetConfigMap(namespace, name string) (*corev1.ConfigMap, error)
}

type ConfigMapController struct {
	client client.Client
}

// NewRealConfigMapControl creates a concrete implementation of the
// IConfigMapControl.
func NewConfigMapController(client client.Client) IConfigMapControl {
	return &ConfigMapController{client: client}
}

// CreateConfigMap implement the IConfigMapControl.Interface.
func (s *ConfigMapController) CreateConfigMap(cm *corev1.ConfigMap) error {
	return s.client.Create(context.TODO(), cm)
}

// UpdateConfigMap implement the IConfigMapControl.Interface.
func (s *ConfigMapController) UpdateConfigMap(cm *corev1.ConfigMap) error {
	return s.client.Update(context.TODO(), cm)
}

// DeleteConfigMap implement the IConfigMapControl.Interface.
func (s *ConfigMapController) DeleteConfigMap(cm *corev1.ConfigMap) error {
	return s.client.Delete(context.TODO(), cm)
}

// GetConfigMap implement the IConfigMapControl.Interface.
func (s *ConfigMapController) GetConfigMap(namespace, name string) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{}
	err := s.client.Get(context.TODO(), types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, cm)
	return cm, err
}
