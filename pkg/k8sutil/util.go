package k8sutil

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func IsRequestRetryable(err error) bool {
	return kerr.IsServiceUnavailable(err) ||
		kerr.IsTimeout(err) ||
		kerr.IsServerTimeout(err) ||
		kerr.IsTooManyRequests(err)
}

func OSMSecretName(name string) string {
	return fmt.Sprintf("osm-%v", name)
}

func CreateSecret(client client.Client, secret *corev1.Secret) error {
	ctx := context.TODO()
	s := &corev1.Secret{}
	err := client.Get(ctx, types.NamespacedName{
		Namespace: secret.Namespace,
		Name:      secret.Name,
	}, s)
	if err != nil {
		if errors.IsNotFound(err) {
			return client.Create(ctx, secret)
		}
	}
	return err
}
