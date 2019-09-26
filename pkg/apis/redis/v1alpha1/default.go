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

func (d *DistributedRedisCluster) Validate() {
	if d.Spec.MasterSize < minMasterSize {
		d.Spec.MasterSize = minMasterSize
	}

	if d.Spec.ClusterReplicas < minClusterReplicas {
		d.Spec.ClusterReplicas = minClusterReplicas
	}

	if d.Spec.Image == "" {
		d.Spec.Image = defaultRedisImage
	}

	if d.Spec.Resources == nil || d.Spec.Resources.Size() == 0 {
		d.Spec.Resources = defaultResource()
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
