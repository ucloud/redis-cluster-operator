package distributedrediscluster

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
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
	"github.com/ucloud/redis-cluster-operator/pkg/config"
	clustermanger "github.com/ucloud/redis-cluster-operator/pkg/controller/manager"
	"github.com/ucloud/redis-cluster-operator/pkg/k8sutil"
	"github.com/ucloud/redis-cluster-operator/pkg/redisutil"
	"github.com/ucloud/redis-cluster-operator/pkg/resources/statefulsets"
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

	pred := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			log.WithValues("namespace", e.MetaNew.GetNamespace(), "name", e.MetaNew.GetName()).Info("Call UpdateFunc")
			// Ignore updates to CR status in which case metadata.Generation does not change
			if e.MetaOld.GetGeneration() != e.MetaNew.GetGeneration() {
				log.WithValues("namespace", e.MetaNew.GetNamespace(), "name", e.MetaNew.GetName()).Info("Generation change return true")
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			log.WithValues("namespace", e.Meta.GetNamespace(), "name", e.Meta.GetName()).Info("Call DeleteFunc")
			// Evaluates to false if the object has been confirmed deleted.
			return !e.DeleteStateUnknown
		},
		CreateFunc: func(e event.CreateEvent) bool {
			log.WithValues("namespace", e.Meta.GetNamespace(), "name", e.Meta.GetName()).Info("Call CreateFunc")
			return true
		},
	}

	// Watch for changes to primary resource DistributedRedisCluster
	err = c.Watch(&source.Kind{Type: &redisv1alpha1.DistributedRedisCluster{}}, &handler.EnqueueRequestForObject{}, pred)
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

	err = r.waitPodReady(instance)
	if err != nil {
		reqLogger.WithValues("err", err).Info("requeue")
		new := instance.Status.DeepCopy()
		SetClusterScaling(new, err.Error())
		r.updateClusterIfNeed(instance, new)
		return reconcile.Result{RequeueAfter: requeueAfter}, nil
	}

	redisClusterPods, err := r.statefulSetController.GetStatefulSetPods(instance.Namespace, statefulsets.ClusterStatefulSetName(instance.Name))
	if err != nil {
		return reconcile.Result{}, Kubernetes.Wrap(err, "GetStatefulSetPods")
	}
	password, err := getClusterPassword(r.client, instance)
	if err != nil {
		return reconcile.Result{}, Kubernetes.Wrap(err, "getClusterPassword")
	}

	admin, err := newRedisAdmin(redisClusterPods.Items, password, config.RedisConf())
	if err != nil {
		return reconcile.Result{}, Redis.Wrap(err, "newRedisAdmin")
	}
	defer admin.Close()

	clusterInfos, err := admin.GetClusterInfos()
	if err != nil {
		if clusterInfos.Status == redisutil.ClusterInfosPartial {
			return reconcile.Result{}, Redis.Wrap(err, "GetClusterInfos")
		}
	}
	status := buildClusterStatus(clusterInfos, redisClusterPods.Items)
	r.updateClusterIfNeed(instance, status)

	err = r.sync(instance, clusterInfos, admin)
	if err != nil {
		new := instance.Status.DeepCopy()
		SetClusterFailed(new, err.Error())
		r.updateClusterIfNeed(instance, new)
		reqLogger.WithValues("err", err).Info("requeue")
		return reconcile.Result{}, err
	}
	new := instance.Status.DeepCopy()
	SetClusterOK(new, "OK")
	r.updateClusterIfNeed(instance, new)
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
