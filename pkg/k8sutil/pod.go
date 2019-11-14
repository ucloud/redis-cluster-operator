package k8sutil

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IPodControl defines the interface that uses to create, update, and delete Pods.
type IPodControl interface {
	// CreatePod creates a Pod in a DistributedRedisCluster.
	CreatePod(*corev1.Pod) error
	// UpdatePod updates a Pod in a DistributedRedisCluster.
	UpdatePod(*corev1.Pod) error
	// DeletePod deletes a Pod in a DistributedRedisCluster.
	DeletePod(*corev1.Pod) error
	DeletePodByName(namespace, name string) error
	// GetPod get Pod in a DistributedRedisCluster.
	GetPod(namespace, name string) (*corev1.Pod, error)
}

type PodController struct {
	client client.Client
}

// NewPodController creates a concrete implementation of the
// IPodControl.
func NewPodController(client client.Client) IPodControl {
	return &PodController{client: client}
}

// CreatePod implement the IPodControl.Interface.
func (p *PodController) CreatePod(pod *corev1.Pod) error {
	return p.client.Create(context.TODO(), pod)
}

// UpdatePod implement the IPodControl.Interface.
func (p *PodController) UpdatePod(pod *corev1.Pod) error {
	return p.client.Update(context.TODO(), pod)
}

// DeletePod implement the IPodControl.Interface.
func (p *PodController) DeletePod(pod *corev1.Pod) error {
	return p.client.Delete(context.TODO(), pod)
}

// DeletePod implement the IPodControl.Interface.
func (p *PodController) DeletePodByName(namespace, name string) error {
	pod, err := p.GetPod(namespace, name)
	if err != nil {
		return err
	}
	return p.client.Delete(context.TODO(), pod)
}

// GetPod implement the IPodControl.Interface.
func (p *PodController) GetPod(namespace, name string) (*corev1.Pod, error) {
	pod := &corev1.Pod{}
	err := p.client.Get(context.TODO(), types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, pod)
	return pod, err
}
