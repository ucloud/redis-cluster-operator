package v1alpha1

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/go-logr/logr"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/ucloud/redis-cluster-operator/pkg/config"
	"github.com/ucloud/redis-cluster-operator/pkg/utils"
)

const (
	minMasterSize      = 3
	minClusterReplicas = 1
	defaultRedisImage  = "redis:5.0.4-alpine"
)

func (in *DistributedRedisCluster) Validate(log logr.Logger) {
	if in.Spec.MasterSize < minMasterSize {
		in.Spec.MasterSize = minMasterSize
	}

	//if in.Spec.ClusterReplicas < minClusterReplicas {
	//	in.Spec.ClusterReplicas = minClusterReplicas
	//}

	if in.Spec.Image == "" {
		in.Spec.Image = defaultRedisImage
	}

	if in.Spec.ServiceName == "" {
		in.Spec.ServiceName = in.Name
	}

	if in.Spec.Resources == nil || in.Spec.Resources.Size() == 0 {
		in.Spec.Resources = defaultResource()
	}

	renameCmdMap := utils.BuildCommandReplaceMapping(config.RedisConf().GetRenameCommandsFile(), log)
	renameCmdSlice := make([]string, len(renameCmdMap))
	i := 0
	for key, value := range renameCmdMap {
		cmd := fmt.Sprintf("--rename-command %s %s", key, value)
		renameCmdSlice[i] = cmd
		i++
	}
	sort.Strings(renameCmdSlice)
	for _, cmd := range renameCmdSlice {
		in.Spec.Command = append(in.Spec.Command, cmd)
	}

	mon := in.Spec.Monitor
	if mon != nil {
		if mon.Prometheus == nil {
			mon.Prometheus = &PrometheusSpec{}
		}
		if mon.Prometheus.Port == 0 {
			mon.Prometheus.Port = PrometheusExporterPortNumber
		}
		if in.Spec.Annotations == nil {
			in.Spec.Annotations = make(map[string]string)
		}

		in.Spec.Annotations["prometheus.io/scrape"] = "true"
		in.Spec.Annotations["prometheus.io/path"] = PrometheusExporterTelemetryPath
		in.Spec.Annotations["prometheus.io/port"] = fmt.Sprintf("%d", mon.Prometheus.Port)
	}
}

func defaultResource() *v1.ResourceRequirements {
	return &v1.ResourceRequirements{
		Requests: v1.ResourceList{
			v1.ResourceCPU:    resource.MustParse("200m"),
			v1.ResourceMemory: resource.MustParse("2Gi"),
		},
		Limits: v1.ResourceList{
			v1.ResourceCPU:    resource.MustParse("1000m"),
			v1.ResourceMemory: resource.MustParse("4Gi"),
		},
	}
}

func DefaultOwnerReferences(cluster *DistributedRedisCluster) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		*metav1.NewControllerRef(cluster, schema.GroupVersionKind{
			Group:   SchemeGroupVersion.Group,
			Version: SchemeGroupVersion.Version,
			Kind:    DistributedRedisClusterKind,
		}),
	}
}

func (in *RedisClusterBackup) Validate() error {
	clusterName := in.Spec.RedisClusterName
	if clusterName == "" {
		return fmt.Errorf("bakcup [RedisClusterName] is missing")
	}
	// BucketName can't be empty
	if in.Spec.S3 == nil && in.Spec.GCS == nil && in.Spec.Azure == nil && in.Spec.Swift == nil && in.Spec.Local == nil {
		return fmt.Errorf("no storage provider is configured")
	}

	if in.Spec.Azure != nil || in.Spec.Swift != nil {
		if in.Spec.StorageSecretName == "" {
			return fmt.Errorf("bakcup [SecretName] is missing")
		}
	}
	return nil
}

func (in *RedisClusterBackup) Location() (string, error) {
	spec := in.Spec.Backend
	timePrefix := in.Status.StartTime.Format("20060102150405")
	if spec.S3 != nil {
		return filepath.Join(spec.S3.Prefix, DatabaseNamePrefix, in.Namespace, in.Spec.RedisClusterName, timePrefix), nil
	} else if spec.GCS != nil {
		return filepath.Join(spec.GCS.Prefix, DatabaseNamePrefix, in.Namespace, in.Spec.RedisClusterName, timePrefix), nil
	} else if spec.Azure != nil {
		return filepath.Join(spec.Azure.Prefix, DatabaseNamePrefix, in.Namespace, in.Spec.RedisClusterName, timePrefix), nil
	} else if spec.Local != nil {
		return filepath.Join(DatabaseNamePrefix, in.Namespace, in.Spec.RedisClusterName, timePrefix), nil
	} else if spec.Swift != nil {
		return filepath.Join(spec.Swift.Prefix, DatabaseNamePrefix, in.Namespace, in.Spec.RedisClusterName, timePrefix), nil
	}
	return "", fmt.Errorf("no storage provider is configured")
}

func (in *RedisClusterBackup) OSMSecretName() string {
	return fmt.Sprintf("osmconfig-%v", in.Name)
}

func (in *RedisClusterBackup) JobName() string {
	return fmt.Sprintf("redisbackup-%v", in.Name)
}
