package k8sutil

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IServiceControl defines the interface that uses to create, update, and delete Services.
type IServiceControl interface {
	// CreateService creates a Service in a DistributedRedisCluster.
	CreateService(*corev1.Service) error
	// UpdateService updates a Service in a DistributedRedisCluster.
	UpdateService(*corev1.Service) error
	// DeleteService deletes a Service in a DistributedRedisCluster.
	DeleteService(*corev1.Service) error
	DeleteServiceByName(namespace, name string) error
	// GetService get Service in a DistributedRedisCluster.
	GetService(namespace, name string) (*corev1.Service, error)
}

type serviceController struct {
	client client.Client
}

// NewRealServiceControl creates a concrete implementation of the
// IServiceControl.
func NewServiceController(client client.Client) IServiceControl {
	return &serviceController{client: client}
}

// CreateService implement the IServiceControl.Interface.
func (s *serviceController) CreateService(svc *corev1.Service) error {
	return s.client.Create(context.TODO(), svc)
}

// UpdateService implement the IServiceControl.Interface.
func (s *serviceController) UpdateService(svc *corev1.Service) error {
	return s.client.Update(context.TODO(), svc)
}

// DeleteService implement the IServiceControl.Interface.
func (s *serviceController) DeleteService(svc *corev1.Service) error {
	return s.client.Delete(context.TODO(), svc)
}

func (s *serviceController) DeleteServiceByName(namespace, name string) error {
	svc, err := s.GetService(namespace, name)
	if err != nil {
		return err
	}
	return s.DeleteService(svc)
}

// GetService implement the IServiceControl.Interface.
func (s *serviceController) GetService(namespace, name string) (*corev1.Service, error) {
	svc := &corev1.Service{}
	err := s.client.Get(context.TODO(), types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, svc)
	return svc, err
}
