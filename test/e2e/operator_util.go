package e2e

import (
	"context"
	"flag"
	"fmt"
	"net"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
	"github.com/ucloud/redis-cluster-operator/pkg/config"
	"github.com/ucloud/redis-cluster-operator/pkg/redisutil"
	"github.com/ucloud/redis-cluster-operator/pkg/utils"
)

const (
	Redis3_1_12 = "uhub.service.ucloud.cn/operator/redis:3.2.12-alpine"
	Redis4_0_14 = "uhub.service.ucloud.cn/operator/redis:4.0.14-alpine"
	Redis5_0_4  = "uhub.service.ucloud.cn/operator/redis:5.0.4-alpine"
	Redis5_0_6  = "uhub.service.ucloud.cn/operator/redis:5.0.6-alpine"

	exporterImage = "uhub.service.ucloud.cn/operator/redis_exporter:latest"

	passwordKey = "password"

	// RedisRenameCommandsDefaultPath default path to volume storing rename commands
	RedisRenameCommandsDefaultPath = "/etc/secret-volume"
	// RedisRenameCommandsDefaultFile default file name containing rename commands
	RedisRenameCommandsDefaultFile = ""
)

var (
	renameCommandsPath string
	renameCommandsFile string
)

func init() {
	flag.StringVar(&renameCommandsPath, "rename-command-path", RedisRenameCommandsDefaultPath, "Path to the folder where rename-commands option for redis are available")
	flag.StringVar(&renameCommandsFile, "rename-command-file", RedisRenameCommandsDefaultFile, "Name of the file where rename-commands option for redis are available, disabled if empty")
}

var logger = logf.Log.WithName("e2e-test")

func NewDistributedRedisCluster(name, namespace, image, passwordName string, masterSize, clusterReplicas int32) *redisv1alpha1.DistributedRedisCluster {
	configParams := map[string]string{
		"hz":         "11",
		"maxclients": "101",
	}
	return &redisv1alpha1.DistributedRedisCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"redis.kun/scope": "cluster-scoped",
			},
		},
		Spec: redisv1alpha1.DistributedRedisClusterSpec{
			Image:           image,
			MasterSize:      masterSize,
			ClusterReplicas: clusterReplicas,
			Command:         []string{},
			Config:          configParams,
			PasswordSecret:  &corev1.LocalObjectReference{Name: passwordName},
			Resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("512Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("1024Mi"),
				},
			},
			//Storage: &redisv1alpha1.RedisStorage{
			//	Type:        "persistent-claim",
			//	Size:        resource.MustParse("1Gi"),
			//	Class:       "storageClass",
			//	DeleteClaim: true,
			//},
			Monitor: &redisv1alpha1.AgentSpec{
				Image: exporterImage,
				Prometheus: &redisv1alpha1.PrometheusSpec{
					Port: 9121,
				},
			},
			Annotations: map[string]string{
				"prometheus.io/app-metrics":      "true",
				"prometheus.io/app-metrics-path": "/metrics",
				"prometheus.io/app-metrics-port": "9121",
				"prometheus.io/scrape":           "true",
			},
			Affinity: &corev1.Affinity{},
		},
	}
}

func IsDistributedRedisClusterProperly(f *Framework, drc *redisv1alpha1.DistributedRedisCluster) func() error {
	return func() error {
		result := &redisv1alpha1.DistributedRedisCluster{}
		if err := f.Client.Get(context.TODO(), types.NamespacedName{
			Namespace: f.Namespace(),
			Name:      drc.Name,
		}, result); err != nil {
			f.Logf("can not get DistributedRedisCluster err: %s", err.Error())
			return err
		}
		if result.Status.Status != redisv1alpha1.ClusterStatusOK {
			return LogAndReturnErrorf("DistributedRedisCluster %s status not healthy, current: %s", drc.Name, result.Status.Status)
		}
		password, err := getClusterPassword(f.Client, drc)
		if err != nil {
			return LogAndReturnErrorf("getClusterPassword from secret err: ", err)
		}
		podList, err := f.GetDRCPodsByLabels(getLabels(drc))
		if err != nil {
			return LogAndReturnErrorf("GetDRCPodsByLabels err: ", err)
		}
		if len(podList.Items) != int(drc.Spec.MasterSize*(drc.Spec.ClusterReplicas+1)) {
			return LogAndReturnErrorf("DistributedRedisCluster %s wrong node number, masterSize %d, clusterReplicas %d, got node number %d",
				drc.Name, drc.Spec.MasterSize, drc.Spec.ClusterReplicas, len(podList.Items))
		}
		redisconf := &config.Redis{
			DialTimeout:        5000,
			RenameCommandsFile: renameCommandsFile,
			RenameCommandsPath: renameCommandsPath,
		}
		redisAdmin, err := NewRedisAdmin(podList.Items, password, redisconf, logger)
		if err != nil {
			f.Logf("NewRedisAdmin err: %s", err)
			return err
		}
		if _, err := redisAdmin.GetClusterInfos(); err != nil {
			f.Logf("DistributedRedisCluster Cluster nodes: %s", err)
			return err
		}

		return nil
	}
}

func getLabels(cluster *redisv1alpha1.DistributedRedisCluster) map[string]string {
	dynLabels := map[string]string{
		redisv1alpha1.LabelClusterName:  cluster.Name,
		redisv1alpha1.LabelManagedByKey: redisv1alpha1.OperatorName,
	}
	return utils.MergeLabels(dynLabels, cluster.Labels)
}

// NewRedisAdmin builds and returns new redis.Admin from the list of pods
func NewRedisAdmin(pods []corev1.Pod, password string, cfg *config.Redis, reqLogger logr.Logger) (redisutil.IAdmin, error) {
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

func getClusterPassword(client client.Client, cluster *redisv1alpha1.DistributedRedisCluster) (string, error) {
	if cluster.Spec.PasswordSecret == nil {
		return "", nil
	}
	secret := &corev1.Secret{}
	err := client.Get(context.TODO(), types.NamespacedName{
		Name:      cluster.Spec.PasswordSecret.Name,
		Namespace: cluster.Namespace,
	}, secret)
	if err != nil {
		return "", err
	}
	return string(secret.Data[passwordKey]), nil
}
