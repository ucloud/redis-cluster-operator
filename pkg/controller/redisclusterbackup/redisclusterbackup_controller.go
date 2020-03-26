package redisclusterbackup

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/spf13/pflag"
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

var (
	log = logf.Log.WithName("controller_redisclusterbackup")

	controllerFlagSet *pflag.FlagSet
	// maxConcurrentReconciles is the maximum number of concurrent Reconciles which can be run. Defaults to 1.
	maxConcurrentReconciles int
)

const backupFinalizer = "finalizer.backup.redis.kun"

func init() {
	controllerFlagSet = pflag.NewFlagSet("controller", pflag.ExitOnError)
	controllerFlagSet.IntVar(&maxConcurrentReconciles, "backupctr-maxconcurrent", 2, "the maximum number of concurrent Reconciles which can be run. Defaults to 1.")
}

func FlagSet() *pflag.FlagSet {
	return controllerFlagSet
}

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
	r.directClient = newDirectClient(mgr.GetConfig())
	r.jobController = k8sutil.NewJobController(r.directClient)
	r.recorder = mgr.GetEventRecorderFor("redis-cluster-operator-backup")
	return r
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("redisclusterbackup-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: maxConcurrentReconciles})
	if err != nil {
		return err
	}

	pred := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			// returns false if DistributedRedisCluster is ignored (not managed) by this operator.
			if !utils.ShoudManage(e.MetaNew) {
				return false
			}
			log.WithValues("namespace", e.MetaNew.GetNamespace(), "name", e.MetaNew.GetName()).V(5).Info("Call UpdateFunc")
			// Ignore updates to CR status in which case metadata.Generation does not change
			if e.MetaOld.GetGeneration() != e.MetaNew.GetGeneration() {
				log.WithValues("namespace", e.MetaNew.GetNamespace(), "name", e.MetaNew.GetName()).Info("Generation change return true",
					"old", e.ObjectOld, "new", e.ObjectNew)
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// returns false if DistributedRedisCluster is ignored (not managed) by this operator.
			if !utils.ShoudManage(e.Meta) {
				return false
			}
			log.WithValues("namespace", e.Meta.GetNamespace(), "name", e.Meta.GetName()).Info("Call DeleteFunc")
			// Evaluates to false if the object has been confirmed deleted.
			return !e.DeleteStateUnknown
		},
		CreateFunc: func(e event.CreateEvent) bool {
			// returns false if DistributedRedisCluster is ignored (not managed) by this operator.
			if !utils.ShoudManage(e.Meta) {
				return false
			}
			log.WithValues("namespace", e.Meta.GetNamespace(), "name", e.Meta.GetName()).Info("Call CreateFunc")
			return true
		},
	}

	// Watch for changes to primary resource RedisClusterBackup
	err = c.Watch(&source.Kind{Type: &redisv1alpha1.RedisClusterBackup{}}, &handler.EnqueueRequestForObject{}, pred)
	if err != nil {
		return err
	}

	jobPred := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			log.WithValues("namespace", e.MetaNew.GetNamespace(), "name", e.MetaNew.GetName()).V(4).Info("Call Job UpdateFunc")
			if !utils.ShoudManage(e.MetaNew) {
				log.WithValues("namespace", e.MetaNew.GetNamespace(), "name", e.MetaNew.GetName()).V(4).Info("Job UpdateFunc Not Manage")
				return false
			}
			newObj := e.ObjectNew.(*batch.Job)
			if isJobCompleted(newObj) {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if !utils.ShoudManage(e.Meta) {
				return false
			}
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
			log.WithValues("namespace", e.Meta.GetNamespace(), "name", e.Meta.GetName()).V(4).Info("Call Job CreateFunc")
			if !utils.ShoudManage(e.Meta) {
				log.WithValues("namespace", e.Meta.GetNamespace(), "name", e.Meta.GetName()).V(4).Info("Job CreateFunc Not Manage")
				return false
			}
			job := e.Object.(*batch.Job)
			if job.Status.Succeeded > 0 || job.Status.Failed >= utils.Int32(job.Spec.BackoffLimit) {
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
	client       client.Client
	directClient client.Client
	scheme       *runtime.Scheme
	recorder     record.EventRecorder

	crController  k8sutil.ICustomResource
	jobController k8sutil.IJobControl
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

	//// Check if the RedisClusterBackup instance is marked to be deleted, which is
	//// indicated by the deletion timestamp being set.
	//isBackupMarkedToBeDeleted := instance.GetDeletionTimestamp() != nil
	//if isBackupMarkedToBeDeleted {
	//	if contains(instance.GetFinalizers(), backupFinalizer) {
	//		// Run finalization logic for backupFinalizer. If the
	//		// finalization logic fails, don't remove the finalizer so
	//		// that we can retry during the next reconciliation.
	//		if err := r.finalizeBackup(reqLogger, instance); err != nil {
	//			return reconcile.Result{}, err
	//		}
	//
	//		// Remove backupFinalizer. Once all finalizers have been
	//		// removed, the object will be deleted.
	//		instance.SetFinalizers(remove(instance.GetFinalizers(), backupFinalizer))
	//		err := r.client.Update(context.TODO(), instance)
	//		if err != nil {
	//			return reconcile.Result{}, err
	//		}
	//	}
	//	return reconcile.Result{}, nil
	//}

	//// Add finalizer for this CR
	//if !contains(instance.GetFinalizers(), backupFinalizer) {
	//	if err := r.addFinalizer(reqLogger, instance); err != nil {
	//		return reconcile.Result{}, err
	//	}
	//}

	if err := r.create(reqLogger, instance); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileRedisClusterBackup) finalizeBackup(reqLogger logr.Logger, b *redisv1alpha1.RedisClusterBackup) error {
	// TODO(user): Add the cleanup steps that the operator
	// needs to do before the CR can be deleted. Examples
	// of finalizers include performing backups and deleting
	// resources that are not owned by this CR, like a PVC.
	reqLogger.Info("Successfully finalized RedisClusterBackup")
	return nil
}

func (r *ReconcileRedisClusterBackup) addFinalizer(reqLogger logr.Logger, b *redisv1alpha1.RedisClusterBackup) error {
	reqLogger.Info("Adding Finalizer for the backup")
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

func isJobCompleted(newJob *batch.Job) bool {
	if isJobFinished(newJob) {
		log.WithValues("Request.Namespace", newJob.Namespace).Info("JobFinished", "type", newJob.Status.Conditions[0].Type, "job", newJob.Name)
		return true
	}
	return false
}
