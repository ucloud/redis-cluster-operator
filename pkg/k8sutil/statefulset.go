package k8sutil

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IStatefulSetControl defines the interface that uses to create, update, and delete StatefulSets.
type IStatefulSetControl interface {
	// CreateStatefulSet creates a StatefulSet in a DistributedRedisCluster.
	CreateStatefulSet(*appsv1.StatefulSet) error
	// UpdateStatefulSet updates a StatefulSet in a DistributedRedisCluster.
	UpdateStatefulSet(*appsv1.StatefulSet) error
	// DeleteStatefulSet deletes a StatefulSet in a DistributedRedisCluster.
	DeleteStatefulSet(*appsv1.StatefulSet) error
	DeleteStatefulSetByName(namespace, name string) error
	// GetStatefulSet get StatefulSet in a DistributedRedisCluster.
	GetStatefulSet(namespace, name string) (*appsv1.StatefulSet, error)
	ListStatefulSetByLabels(namespace string, labels map[string]string) (*appsv1.StatefulSetList, error)
	// GetStatefulSetPods will retrieve the pods managed by a given StatefulSet.
	GetStatefulSetPods(namespace, name string) (*corev1.PodList, error)
	GetStatefulSetPodsByLabels(namespace string, labels map[string]string) (*corev1.PodList, error)
}

type stateFulSetController struct {
	client client.Client
}

// NewRealStatefulSetControl creates a concrete implementation of the
// IStatefulSetControl.
func NewStatefulSetController(client client.Client) IStatefulSetControl {
	return &stateFulSetController{client: client}
}

// CreateStatefulSet implement the IStatefulSetControl.Interface.
func (s *stateFulSetController) CreateStatefulSet(ss *appsv1.StatefulSet) error {
	return s.client.Create(context.TODO(), ss)
}

// UpdateStatefulSet implement the IStatefulSetControl.Interface.
func (s *stateFulSetController) UpdateStatefulSet(ss *appsv1.StatefulSet) error {
	return s.client.Update(context.TODO(), ss)
}

// DeleteStatefulSet implement the IStatefulSetControl.Interface.
func (s *stateFulSetController) DeleteStatefulSet(ss *appsv1.StatefulSet) error {
	return s.client.Delete(context.TODO(), ss)
}

func (s *stateFulSetController) DeleteStatefulSetByName(namespace, name string) error {
	sts, err := s.GetStatefulSet(namespace, name)
	if err != nil {
		return err
	}
	return s.DeleteStatefulSet(sts)
}

// GetStatefulSet implement the IStatefulSetControl.Interface.
func (s *stateFulSetController) GetStatefulSet(namespace, name string) (*appsv1.StatefulSet, error) {
	statefulSet := &appsv1.StatefulSet{}
	err := s.client.Get(context.TODO(), types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, statefulSet)
	return statefulSet, err
}

// GetStatefulSetPods implement the IStatefulSetControl.Interface.
func (s *stateFulSetController) GetStatefulSetPods(namespace, name string) (*corev1.PodList, error) {
	statefulSet, err := s.GetStatefulSet(namespace, name)
	if err != nil {
		return nil, err
	}

	match := make(client.MatchingLabels)
	for k, v := range statefulSet.Spec.Selector.MatchLabels {
		match[k] = v
	}
	foundPods := &corev1.PodList{}
	err = s.client.List(context.TODO(), foundPods, client.InNamespace(namespace), match)
	return foundPods, err
}

// GetStatefulSetPodsByLabels implement the IStatefulSetControl.Interface.
func (s *stateFulSetController) GetStatefulSetPodsByLabels(namespace string, labels map[string]string) (*corev1.PodList, error) {
	foundPods := &corev1.PodList{}
	err := s.client.List(context.TODO(), foundPods, client.InNamespace(namespace), client.MatchingLabels(labels))
	return foundPods, err
}

func (s *stateFulSetController) ListStatefulSetByLabels(namespace string, labels map[string]string) (*appsv1.StatefulSetList, error) {
	foundSts := &appsv1.StatefulSetList{}
	err := s.client.List(context.TODO(), foundSts, client.InNamespace(namespace), client.MatchingLabels(labels))
	return foundSts, err
}
