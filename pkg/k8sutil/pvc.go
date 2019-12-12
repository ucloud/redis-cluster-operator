package k8sutil

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IPvcControl defines the interface that uses to create, update, and delete PersistentVolumeClaim.
type IPvcControl interface {
	DeletePvc(claim *corev1.PersistentVolumeClaim) error
	DeletePvcByLabels(namespace string, labels map[string]string) error
	GetPvc(namespace, name string) (*corev1.PersistentVolumeClaim, error)
}

type pvcController struct {
	client client.Client
}

// NewPvcController creates a concrete implementation of the
// IPvcControl.
func NewPvcController(client client.Client) IPvcControl {
	return &pvcController{client: client}
}

// DeletePvc implement the IPvcControl.Interface.
func (s *pvcController) DeletePvc(pvc *corev1.PersistentVolumeClaim) error {
	return s.client.Delete(context.TODO(), pvc)
}

func (s *pvcController) DeletePvcByLabels(namespace string, labels map[string]string) error {
	foundPvcs := &corev1.PersistentVolumeClaimList{}
	err := s.client.List(context.TODO(), foundPvcs, client.InNamespace(namespace), client.MatchingLabels(labels))
	if err != nil {
		return err
	}

	for _, pvc := range foundPvcs.Items {
		if err := s.client.Delete(context.TODO(), &pvc); err != nil {
			return err
		}
	}
	return nil
}

// GetPvc implement the IPvcControl.Interface.
func (s *pvcController) GetPvc(namespace, name string) (*corev1.PersistentVolumeClaim, error) {
	pvc := &corev1.PersistentVolumeClaim{}
	err := s.client.Get(context.TODO(), types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, pvc)
	return pvc, err
}
