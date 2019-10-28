package redisclusterbackup

import (
	"context"

	"github.com/go-logr/logr"
	batch "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
	"github.com/ucloud/redis-cluster-operator/pkg/k8sutil"
	"github.com/ucloud/redis-cluster-operator/pkg/utils"
)

var log = logf.Log.WithName("controller_redisclusterbackup")

const backupFinalizer = "finalizer.backup.redis.kun"

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new RedisClusterBackup Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	r := &ReconcileRedisClusterBackup{client: mgr.GetClient(), scheme: mgr.GetScheme()}
	r.crController = k8sutil.NewCRControl(r.client)
	r.recorder = mgr.GetEventRecorderFor("redis-cluster-operator-backup")
	return r
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("redisclusterbackup-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource RedisClusterBackup
	err = c.Watch(&source.Kind{Type: &redisv1alpha1.RedisClusterBackup{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	jobPred := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldObj := e.ObjectOld.(*batch.Job)
			newObj := e.ObjectNew.(*batch.Job)
			if isJobCompleted(oldObj, newObj) {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			job, ok := e.Object.(*batch.Job)
			if !ok {
				log.Error(nil, "Invalid Job object")
				return false
			}
			if job.Status.Succeeded == 0 && job.Status.Failed <= utils.Int32(job.Spec.BackoffLimit) {
				return true
			}
			return false
		},
		CreateFunc: func(e event.CreateEvent) bool {
			job := e.Object.(*batch.Job)
			if job.Status.Succeeded > 0 || job.Status.Failed > utils.Int32(job.Spec.BackoffLimit) {
				return true
			}
			return false
		},
	}

	// Watch for changes to secondary resource Jobs and requeue the owner RedisClusterBackup
	err = c.Watch(&source.Kind{Type: &batch.Job{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &redisv1alpha1.RedisClusterBackup{},
	}, jobPred)
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileRedisClusterBackup implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileRedisClusterBackup{}

// ReconcileRedisClusterBackup reconciles a RedisClusterBackup object
type ReconcileRedisClusterBackup struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client   client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder

	crController k8sutil.ICustomResource
}

// Reconcile reads that state of the cluster for a RedisClusterBackup object and makes changes based on the state read
// and what is in the RedisClusterBackup.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileRedisClusterBackup) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling RedisClusterBackup")

	// Fetch the RedisClusterBackup instance
	instance := &redisv1alpha1.RedisClusterBackup{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Check if the RedisClusterBackup instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	isMemcachedMarkedToBeDeleted := instance.GetDeletionTimestamp() != nil
	if isMemcachedMarkedToBeDeleted {
		if contains(instance.GetFinalizers(), backupFinalizer) {
			// Run finalization logic for backupFinalizer. If the
			// finalization logic fails, don't remove the finalizer so
			// that we can retry during the next reconciliation.
			if err := r.finalizeMemcached(reqLogger, instance); err != nil {
				return reconcile.Result{}, err
			}

			// Remove backupFinalizer. Once all finalizers have been
			// removed, the object will be deleted.
			instance.SetFinalizers(remove(instance.GetFinalizers(), backupFinalizer))
			err := r.client.Update(context.TODO(), instance)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, nil
	}

	// Add finalizer for this CR
	if !contains(instance.GetFinalizers(), backupFinalizer) {
		if err := r.addFinalizer(reqLogger, instance); err != nil {
			return reconcile.Result{}, err
		}
	}

	if err := r.create(reqLogger, instance); err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileRedisClusterBackup) finalizeMemcached(reqLogger logr.Logger, b *redisv1alpha1.RedisClusterBackup) error {
	// TODO(user): Add the cleanup steps that the operator
	// needs to do before the CR can be deleted. Examples
	// of finalizers include performing backups and deleting
	// resources that are not owned by this CR, like a PVC.
	reqLogger.Info("Successfully finalized RedisClusterBackup")
	return nil
}

func (r *ReconcileRedisClusterBackup) addFinalizer(reqLogger logr.Logger, b *redisv1alpha1.RedisClusterBackup) error {
	reqLogger.Info("Adding Finalizer for the Memcached")
	b.SetFinalizers(append(b.GetFinalizers(), backupFinalizer))

	// Update CR
	err := r.client.Update(context.TODO(), b)
	if err != nil {
		reqLogger.Error(err, "Failed to update RedisClusterBackup with finalizer")
		return err
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func remove(list []string, s string) []string {
	for i, v := range list {
		if v == s {
			list = append(list[:i], list[i+1:]...)
		}
	}
	return list
}

func isJobCompleted(old, new *batch.Job) bool {
	if old.Status.Succeeded == 0 && new.Status.Succeeded > 0 {
		return true
	}
	if old.Status.Failed < utils.Int32(old.Spec.BackoffLimit) && new.Status.Failed >= utils.Int32(new.Spec.BackoffLimit) {
		return true
	}
	return false
}
