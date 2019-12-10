package poddisruptionbudgets

import (
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
)

func NewPodDisruptionBudgetForCR(cluster *redisv1alpha1.DistributedRedisCluster, name string, labels map[string]string) *policyv1beta1.PodDisruptionBudget {
	maxUnavailable := intstr.FromInt(1)

	return &policyv1beta1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Labels:          labels,
			Name:            name,
			Namespace:       cluster.Namespace,
			OwnerReferences: redisv1alpha1.DefaultOwnerReferences(cluster),
		},
		Spec: policyv1beta1.PodDisruptionBudgetSpec{
			MaxUnavailable: &maxUnavailable,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
		},
	}
}
