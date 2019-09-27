package distributedrediscluster

import (
	"context"
	"github.com/ucloud/redis-cluster-operator/pkg/k8sutil"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
	clustermanger "github.com/ucloud/redis-cluster-operator/pkg/controller/manager"
)

var log = logf.Log.WithName("controller_distributedrediscluster")

const maxConcurrentReconciles = 2

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new DistributedRedisCluster Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	reconiler := &ReconcileDistributedRedisCluster{client: mgr.GetClient(), scheme: mgr.GetScheme()}
	reconiler.statefulSetController = k8sutil.NewStatefulSetController(reconiler.client)
	reconiler.clusterStatusController = k8sutil.NewClusterControl(reconiler.client)
	reconiler.ensurer = clustermanger.NewEnsureResource(reconiler.client, log)
	reconiler.checker = clustermanger.NewCheck(reconiler.client)
	return reconiler
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("distributedrediscluster-controller", mgr, controller.Options{
		Reconciler:              r,
		MaxConcurrentReconciles: maxConcurrentReconciles,
	})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource DistributedRedisCluster
	err = c.Watch(&source.Kind{Type: &redisv1alpha1.DistributedRedisCluster{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner DistributedRedisCluster
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &redisv1alpha1.DistributedRedisCluster{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileDistributedRedisCluster implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileDistributedRedisCluster{}

// ReconcileDistributedRedisCluster reconciles a DistributedRedisCluster object
type ReconcileDistributedRedisCluster struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client                  client.Client
	scheme                  *runtime.Scheme
	ensurer                 clustermanger.IEnsureResource
	checker                 clustermanger.ICheck
	statefulSetController   k8sutil.IStatefulSetControl
	clusterStatusController k8sutil.ICluster
}

// Reconcile reads that state of the cluster for a DistributedRedisCluster object and makes changes based on the state read
// and what is in the DistributedRedisCluster.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileDistributedRedisCluster) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling DistributedRedisCluster")

	// Fetch the DistributedRedisCluster instance
	instance := &redisv1alpha1.DistributedRedisCluster{}
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

	err = r.sync(instance)
	status := buildClusterStatus(err)
	r.updateClusterIfNeed(instance, status)
	if err != nil {
		reqLogger.WithValues("err", err).Info("requeue")
		switch GetType(err) {
		case Requeue:
			return reconcile.Result{RequeueAfter: requeueAfter}, nil
		}
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
	//// Define a new Pod object
	//pod := newPodForCR(instance)
	//
	//// Set DistributedRedisCluster instance as the owner and controller
	//if err := controllerutil.SetControllerReference(instance, pod, r.scheme); err != nil {
	//	return reconcile.Result{}, err
	//}

	// Check if this Pod already exists
	//found := &corev1.Pod{}
	//err = r.client.Get(context.TODO(), types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, found)
	//if err != nil && errors.IsNotFound(err) {
	//	reqLogger.Info("Creating a new Pod", "Pod.Namespace", pod.Namespace, "Pod.Name", pod.Name)
	//	err = r.client.Create(context.TODO(), pod)
	//	if err != nil {
	//		return reconcile.Result{}, err
	//	}
	//
	//	// Pod created successfully - don't requeue
	//	return reconcile.Result{}, nil
	//} else if err != nil {
	//	return reconcile.Result{}, err
	//}

	// Pod already exists - don't requeue
	//reqLogger.Info("Skip reconcile: Pod already exists", "Pod.Namespace", found.Namespace, "Pod.Name", found.Name)
	//return reconcile.Result{}, nil
}
