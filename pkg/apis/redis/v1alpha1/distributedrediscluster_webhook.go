package v1alpha1

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	utilvalidation "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/kubernetes/pkg/apis/core/v1/validation"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/ucloud/redis-cluster-operator/pkg/utils"
)

var log = logf.Log.WithName("drc-resource")

var _ webhook.Validator = &DistributedRedisCluster{}

func (in *DistributedRedisCluster) ValidateCreate() error {
	log := log.WithValues("namespace", in.Namespace, "name", in.Name)
	log.Info("ValidateCreate")
	if errs := utilvalidation.IsDNS1035Label(in.Spec.ServiceName); len(in.Spec.ServiceName) > 0 && len(errs) > 0 {
		return fmt.Errorf("the custom service is invalid: invalid value: %s, %s", in.Spec.ServiceName, strings.Join(errs, ","))
	}

	if in.Spec.Resources != nil {
		if errs := validation.ValidateResourceRequirements(in.Spec.Resources, field.NewPath("resources")); len(errs) > 0 {
			return errs.ToAggregate()
		}
	}

	return nil
}

func (in *DistributedRedisCluster) ValidateUpdate(old runtime.Object) error {
	log := log.WithValues("namespace", in.Namespace, "name", in.Name)
	log.Info("ValidateUpdate")

	oldObj, ok := old.(*DistributedRedisCluster)
	if !ok {
		err := fmt.Errorf("invalid obj type")
		log.Error(err, "can not reflect type")
		return err
	}

	if errs := utilvalidation.IsDNS1035Label(in.Spec.ServiceName); len(in.Spec.ServiceName) > 0 && len(errs) > 0 {
		return fmt.Errorf("the custom service is invalid: invalid value: %s, %s", in.Spec.ServiceName, strings.Join(errs, ","))
	}

	if in.Spec.Resources != nil {
		if errs := validation.ValidateResourceRequirements(in.Spec.Resources, field.NewPath("resources")); len(errs) > 0 {
			return errs.ToAggregate()
		}
	}

	if oldObj.Status.Status == "" {
		return nil
	}
	if compareObj(in, oldObj, log) && oldObj.Status.Status != ClusterStatusOK {
		return fmt.Errorf("redis cluster status: [%s], wait for the status to become %s before operating", oldObj.Status.Status, ClusterStatusOK)
	}

	return nil
}

func compareObj(new, old *DistributedRedisCluster, log logr.Logger) bool {
	if utils.CompareInt32("MasterSize", new.Spec.MasterSize, old.Spec.MasterSize, log) {
		return true
	}

	if utils.CompareStringValue("Image", new.Spec.Image, old.Spec.Image, log) {
		return true
	}

	if !reflect.DeepEqual(new.Spec.Resources, old.Spec.Resources) {
		log.Info("compare resource", "new", new.Spec.Resources, "old", old.Spec.Resources)
		return true
	}

	if !reflect.DeepEqual(new.Spec.PasswordSecret, old.Spec.PasswordSecret) {
		log.Info("compare password", "new", new.Spec.PasswordSecret, "old", old.Spec.PasswordSecret)
		return true
	}

	return false
}

func (in *DistributedRedisCluster) ValidateDelete() error {
	log := log.WithValues("namespace", in.Namespace, "name", in.Name)
	log.Info("ValidateDelete")
	return nil
}

func (in *DistributedRedisCluster) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(in).
		Complete()
}
