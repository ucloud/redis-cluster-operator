package distributedrediscluster

import (
	"context"

	"github.com/go-logr/logr"
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
	"github.com/ucloud/redis-cluster-operator/pkg/controller/heal"
	clustermanger "github.com/ucloud/redis-cluster-operator/pkg/controller/manager"
	"github.com/ucloud/redis-cluster-operator/pkg/k8sutil"
	"github.com/ucloud/redis-cluster-operator/pkg/redisutil"
	"github.com/ucloud/redis-cluster-operator/pkg/utils"
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
	reconiler.serviceController = k8sutil.NewServiceController(reconiler.client)
	reconiler.pdbController = k8sutil.NewPodDisruptionBudgetController(reconiler.client)
	reconiler.pvcController = k8sutil.NewPvcController(reconiler.client)
	reconiler.crController = k8sutil.NewCRControl(reconiler.client)
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
	client                client.Client
	scheme                *runtime.Scheme
	ensurer               clustermanger.IEnsureResource
	checker               clustermanger.ICheck
	statefulSetController k8sutil.IStatefulSetControl
	serviceController     k8sutil.IServiceControl
	pdbController         k8sutil.IPodDisruptionBudgetControl
	pvcController         k8sutil.IPvcControl
	crController          k8sutil.ICustomResource
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
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	ctx := &syncContext{
		cluster:   instance,
		reqLogger: reqLogger,
	}

	err = r.ensureCluster(ctx)
	if err != nil {
		switch GetType(err) {
		case StopRetry:
			reqLogger.Info("invalid", "err", err)
			return reconcile.Result{}, nil
		}
		reqLogger.WithValues("err", err).Info("ensureCluster")
		new := instance.Status.DeepCopy()
		SetClusterScaling(new, err.Error())
		r.updateClusterIfNeed(instance, new)
		return reconcile.Result{RequeueAfter: requeueAfter}, nil
	}

	matchLabels := getLabels(instance)
	redisClusterPods, err := r.statefulSetController.GetStatefulSetPodsByLabels(instance.Namespace, matchLabels)
	if err != nil {
		return reconcile.Result{}, Kubernetes.Wrap(err, "GetStatefulSetPods")
	}

	ctx.pods = clusterPods(redisClusterPods.Items)
	reqLogger.V(6).Info("debug cluster pods", "", ctx.pods)
	ctx.healer = clustermanger.NewHealer(&heal.CheckAndHeal{
		Logger:     reqLogger,
		PodControl: k8sutil.NewPodController(r.client),
		Pods:       ctx.pods,
		DryRun:     false,
	})
	err = r.waitPodReady(ctx)
	if err != nil {
		switch GetType(err) {
		case Kubernetes:
			return reconcile.Result{}, err
		}
		reqLogger.WithValues("err", err).Info("waitPodReady")
		new := instance.Status.DeepCopy()
		SetClusterScaling(new, err.Error())
		r.updateClusterIfNeed(instance, new)
		return reconcile.Result{RequeueAfter: requeueAfter}, nil
	}

	password, err := getClusterPassword(r.client, instance)
	if err != nil {
		return reconcile.Result{}, Kubernetes.Wrap(err, "getClusterPassword")
	}

	admin, err := newRedisAdmin(ctx.pods, password, config.RedisConf(), reqLogger)
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

	requeue, err := ctx.healer.Heal(instance, clusterInfos, admin)
	if err != nil {
		return reconcile.Result{}, Redis.Wrap(err, "Heal")
	}
	if requeue {
		return reconcile.Result{RequeueAfter: requeueAfter}, nil
	}

	ctx.admin = admin
	ctx.clusterInfos = clusterInfos
	err = r.waitForClusterJoin(ctx)
	if err != nil {
		switch GetType(err) {
		case Requeue:
			reqLogger.WithValues("err", err).Info("requeue")
			return reconcile.Result{RequeueAfter: requeueAfter}, nil
		}
		new := instance.Status.DeepCopy()
		SetClusterFailed(new, err.Error())
		r.updateClusterIfNeed(instance, new)
		return reconcile.Result{}, err
	}

	// update cr and wait for the next Reconcile loop
	if instance.Spec.Init != nil && instance.Status.RestoreSucceeded <= 0 {
		reqLogger.Info("update restore redis cluster cr")
		instance.Status.RestoreSucceeded = 1
		if err := r.crController.UpdateCRStatus(instance); err != nil {
			return reconcile.Result{}, err
		}
		if err := r.crController.UpdateCR(instance); err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	if err := admin.SetConfigIfNeed(instance.Spec.Config); err != nil {
		return reconcile.Result{}, Redis.Wrap(err, "SetConfigIfNeed")
	}

	status := buildClusterStatus(clusterInfos, ctx.pods, &instance.Status)
	if is := r.isScalingDown(instance, reqLogger); is {
		SetClusterRebalancing(status, "scaling down")
	}
	reqLogger.V(4).Info("buildClusterStatus", "status", status)
	r.updateClusterIfNeed(instance, status)

	instance.Status = *status
	if needClusterOperation(instance, reqLogger) {
		reqLogger.Info(">>>>>> clustering")
		err = r.syncCluster(ctx)
		if err != nil {
			new := instance.Status.DeepCopy()
			SetClusterFailed(new, err.Error())
			r.updateClusterIfNeed(instance, new)
			return reconcile.Result{}, err
		}
	}

	newClusterInfos, err := admin.GetClusterInfos()
	if err != nil {
		if clusterInfos.Status == redisutil.ClusterInfosPartial {
			return reconcile.Result{}, Redis.Wrap(err, "GetClusterInfos")
		}
	}
	newStatus := buildClusterStatus(newClusterInfos, ctx.pods, &instance.Status)
	SetClusterOK(newStatus, "OK")
	r.updateClusterIfNeed(instance, newStatus)
	return reconcile.Result{RequeueAfter: requeueEnsure}, nil
}

func (r *ReconcileDistributedRedisCluster) isScalingDown(cluster *redisv1alpha1.DistributedRedisCluster, reqLogger logr.Logger) bool {
	stsList, err := r.statefulSetController.ListStatefulSetByLabels(cluster.Namespace, getLabels(cluster))
	if err != nil {
		reqLogger.Error(err, "ListStatefulSetByLabels")
		return false
	}
	if len(stsList.Items) > int(cluster.Spec.MasterSize) {
		return true
	}
	return false
}
