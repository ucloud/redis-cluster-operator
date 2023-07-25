package k8sutil

import (
	"context"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IPvcControl defines the interface that uses to create, update, and delete PersistentVolumeClaim.
type IPvcControl interface {
	DeletePvc(claim *corev1.PersistentVolumeClaim) error
	DeletePvcByLabels(namespace string, labels map[string]string) error
	GetPvc(namespace, name string) (*corev1.PersistentVolumeClaim, error)
	UpdatPvcByLabels(cluster *redisv1alpha1.DistributedRedisCluster, pvc *corev1.PersistentVolumeClaim) error
	ListPvcByLabels(namespace string, labels map[string]string) (*corev1.PersistentVolumeClaimList, error)
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

func (s *pvcController) ListPvcByLabels(namespace string, labels map[string]string) (*corev1.PersistentVolumeClaimList, error) {
	foundPvcs := &corev1.PersistentVolumeClaimList{}
	err := s.client.List(context.TODO(), foundPvcs, client.InNamespace(namespace), client.MatchingLabels(labels))
	if err != nil {
		return nil, err
	}
	return foundPvcs, nil
}

func (s *pvcController) UpdatPvcByLabels(cluster *redisv1alpha1.DistributedRedisCluster, pvc *corev1.PersistentVolumeClaim) error {

	mode := corev1.PersistentVolumeFilesystem

	pvcSpec := &corev1.PersistentVolumeClaimSpec{
		AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: cluster.Spec.Storage.Size,
			},
		},
		StorageClassName: &cluster.Spec.Storage.Class,
		VolumeMode:       &mode,
	}
	pvcSpec.Resources.DeepCopyInto(&pvc.Spec.Resources)
	if err := s.client.Update(context.TODO(), pvc); err != nil {
		return err
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
