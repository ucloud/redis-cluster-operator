package distributedrediscluster

import (
	"fmt"
	"net"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
	"github.com/ucloud/redis-cluster-operator/pkg/config"
	"github.com/ucloud/redis-cluster-operator/pkg/k8sutil"
	"github.com/ucloud/redis-cluster-operator/pkg/redisutil"
	"github.com/ucloud/redis-cluster-operator/pkg/utils"
)

var (
	defaultLabels = map[string]string{
		redisv1alpha1.LabelManagedByKey: redisv1alpha1.OperatorName,
	}
)

func getLabels(cluster *redisv1alpha1.DistributedRedisCluster) map[string]string {
	dynLabels := map[string]string{
		redisv1alpha1.LabelClusterName: cluster.Name,
	}
	return utils.MergeLabels(defaultLabels, dynLabels, cluster.Labels)
}

// newRedisAdmin builds and returns new redis.Admin from the list of pods
func newRedisAdmin(pods []*corev1.Pod, password string, cfg *config.Redis, reqLogger logr.Logger) (redisutil.IAdmin, error) {
	nodesAddrs := []string{}
	for _, pod := range pods {
		redisPort := redisutil.DefaultRedisPort
		for _, container := range pod.Spec.Containers {
			if container.Name == "redis" {
				for _, port := range container.Ports {
					if port.Name == "client" {
						redisPort = fmt.Sprintf("%d", port.ContainerPort)
					}
				}
			}
		}
		reqLogger.V(4).Info("append redis admin addr", "addr", pod.Status.PodIP, "port", redisPort)
		nodesAddrs = append(nodesAddrs, net.JoinHostPort(pod.Status.PodIP, redisPort))
	}
	adminConfig := redisutil.AdminOptions{
		ConnectionTimeout:  time.Duration(cfg.DialTimeout) * time.Millisecond,
		RenameCommandsFile: cfg.GetRenameCommandsFile(),
		Password:           password,
	}

	return redisutil.NewAdmin(nodesAddrs, &adminConfig, reqLogger), nil
}

func newRedisCluster(infos *redisutil.ClusterInfos, cluster *redisv1alpha1.DistributedRedisCluster) (*redisutil.Cluster, redisutil.Nodes, error) {
	// now we can trigger the rebalance
	nodes := infos.GetNodes()

	// build redis cluster vision
	rCluster := &redisutil.Cluster{
		Name:      cluster.Name,
		Namespace: cluster.Namespace,
		Nodes:     make(map[string]*redisutil.Node),
	}

	for _, node := range nodes {
		rCluster.Nodes[node.ID] = node
	}

	for _, node := range cluster.Status.Nodes {
		if rNode, ok := rCluster.Nodes[node.ID]; ok {
			rNode.PodName = node.PodName
			rNode.NodeName = node.NodeName
			rNode.StatefulSet = node.StatefulSet
		}
	}

	return rCluster, nodes, nil
}

func clusterPods(pods []corev1.Pod) []*corev1.Pod {
	var podSlice []*corev1.Pod
	for _, pod := range pods {
		// Only work with running pods
		if pod.Status.Phase == corev1.PodRunning {
			podPointer := pod
			podSlice = append(podSlice, &podPointer)
		}
	}
	return podSlice
}

func needClusterOperation(cluster *redisv1alpha1.DistributedRedisCluster, reqLogger logr.Logger) bool {
	if utils.CompareIntValue("NumberOfMaster", &cluster.Status.NumberOfMaster, &cluster.Spec.MasterSize, reqLogger) {
		reqLogger.V(4).Info("needClusterOperation---NumberOfMaster")
		return true
	}

	if utils.CompareIntValue("MinReplicationFactor", &cluster.Status.MinReplicationFactor, &cluster.Spec.ClusterReplicas, reqLogger) {
		reqLogger.V(4).Info("needClusterOperation---MinReplicationFactor")
		return true
	}

	if utils.CompareIntValue("MaxReplicationFactor", &cluster.Status.MaxReplicationFactor, &cluster.Spec.ClusterReplicas, reqLogger) {
		reqLogger.V(4).Info("needClusterOperation---MaxReplicationFactor")
		return true
	}

	return false
}

type IWaitHandle interface {
	Name() string
	Tick() time.Duration
	Timeout() time.Duration
	Handler() error
}

// waiting will keep trying to handler.Handler() until either
// we get a result from handler.Handler() or the timeout expires
func waiting(handler IWaitHandle, reqLogger logr.Logger) error {
	timeout := time.After(handler.Timeout())
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	// Keep trying until we're timed out or got a result or got an error
	for {
		select {
		// Got a timeout! fail with a timeout error
		case <-timeout:
			return fmt.Errorf("%s timed out", handler.Name())
		// Got a tick, we should check on Handler()
		case <-tick.C:
			err := handler.Handler()
			if err == nil {
				return nil
			}
			reqLogger.V(4).Info(err.Error())
		}
	}
}

type waitPodTerminating struct {
	name                  string
	statefulSet           string
	timeout               time.Duration
	tick                  time.Duration
	statefulSetController k8sutil.IStatefulSetControl
	cluster               *redisv1alpha1.DistributedRedisCluster
}

func (w *waitPodTerminating) Name() string {
	return w.name
}

func (w *waitPodTerminating) Tick() time.Duration {
	return w.tick
}

func (w *waitPodTerminating) Timeout() time.Duration {
	return w.timeout
}

func (w *waitPodTerminating) Handler() error {
	labels := getLabels(w.cluster)
	labels[redisv1alpha1.StatefulSetLabel] = w.statefulSet
	podList, err := w.statefulSetController.GetStatefulSetPodsByLabels(w.cluster.Namespace, labels)
	if err != nil {
		return err
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			return fmt.Errorf("[%s] pod still runing", pod.Name)
		}
	}
	return nil
}

type waitStatefulSetUpdating struct {
	name                  string
	timeout               time.Duration
	tick                  time.Duration
	statefulSetController k8sutil.IStatefulSetControl
	cluster               *redisv1alpha1.DistributedRedisCluster
}

func (w *waitStatefulSetUpdating) Name() string {
	return w.name
}

func (w *waitStatefulSetUpdating) Tick() time.Duration {
	return w.tick
}

func (w *waitStatefulSetUpdating) Timeout() time.Duration {
	return w.timeout
}

func (w *waitStatefulSetUpdating) Handler() error {
	labels := getLabels(w.cluster)
	stsList, err := w.statefulSetController.ListStatefulSetByLabels(w.cluster.Namespace, labels)
	if err != nil {
		return err
	}
	for _, sts := range stsList.Items {
		if sts.Status.ReadyReplicas != (w.cluster.Spec.ClusterReplicas + 1) {
			return nil
		}
	}
	return fmt.Errorf("statefulSet still not updated")
}
