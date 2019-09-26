package v1alpha1

import (
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	minMasterSize      = 3
	minClusterReplicas = 1
	defaultRedisImage  = "redis:5.0.4-alpine"
)

func (in *DistributedRedisCluster) Validate() {
	if in.Spec.MasterSize < minMasterSize {
		in.Spec.MasterSize = minMasterSize
	}

	if in.Spec.ClusterReplicas < minClusterReplicas {
		in.Spec.ClusterReplicas = minClusterReplicas
	}

	if in.Spec.Image == "" {
		in.Spec.Image = defaultRedisImage
	}

	if in.Spec.ServiceName == "" {
		in.Spec.ServiceName = in.Name
	}

	if in.Spec.Resources == nil || in.Spec.Resources.Size() == 0 {
		in.Spec.Resources = defaultResource()
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
