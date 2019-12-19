package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var log = logf.Log.WithName("drc-resource")

var _ webhook.Validator = &DistributedRedisCluster{}

func (in *DistributedRedisCluster) ValidateCreate() error {
	log.Info("ValidateCreate")
	return nil
}

func (in *DistributedRedisCluster) ValidateUpdate(old runtime.Object) error {
	log.Info("ValidateUpdate")
	return nil
}

func (in *DistributedRedisCluster) ValidateDelete() error {
	log.Info("ValidateDelete")
	return nil
}

func (in *DistributedRedisCluster) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(in).
		Complete()
}
