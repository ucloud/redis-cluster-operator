package services

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
)

// NewStatefulSetForCR creates a new StatefulSet for the given Cluster.
func NewHeadLessSvcForCR(cluster *redisv1alpha1.DistributedRedisCluster, labels map[string]string) *corev1.Service {
	clientPort := corev1.ServicePort{Name: "client", Port: 6379}
	gossipPort := corev1.ServicePort{Name: "gossip", Port: 16379}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels:          labels,
			Name:            cluster.Spec.ServiceName,
			Namespace:       cluster.Namespace,
			OwnerReferences: redisv1alpha1.DefaultOwnerReferences(cluster),
		},
		Spec: corev1.ServiceSpec{
			Ports:     []corev1.ServicePort{clientPort, gossipPort},
			Selector:  labels,
			ClusterIP: corev1.ClusterIPNone,
		},
	}

	return svc
}
