package k8sutil

import (
	"context"

	"github.com/go-logr/logr"
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

func CreateSecret(client client.Client, secret *corev1.Secret, logger logr.Logger) error {
	ctx := context.TODO()
	s := &corev1.Secret{}
	err := client.Get(ctx, types.NamespacedName{
		Namespace: secret.Namespace,
		Name:      secret.Name,
	}, s)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.WithValues("Secret.Namespace", secret.Namespace, "Secret.Name", secret.Name).
				Info("creating a new secret")
			return client.Create(ctx, secret)
		}
	}
	return err
}
