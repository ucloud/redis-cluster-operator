package statefulsets

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
)

const (
	redisStorageVolumeName = "redis-data"
	redisServerName        = "redis"
	hostnameTopologyKey    = "kubernetes.io/hostname"

	graceTime = 30

	passwordENV = "REDIS_PASSWORD"
)

// NewStatefulSetForCR creates a new StatefulSet for the given Cluster.
func NewStatefulSetForCR(cluster *redisv1alpha1.DistributedRedisCluster, labels map[string]string) *appsv1.StatefulSet {
	password := redisPassword(cluster)
	volumes := redisVolumes(cluster)
	name := ClusterStatefulSetName(cluster.Name)
	namespace := cluster.Namespace
	spec := cluster.Spec
	size := spec.MasterSize * (spec.ClusterReplicas + 1)
	ss := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: ownerReferences(cluster),
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: cluster.Spec.ServiceName,
			Replicas:    &size,
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Affinity:        getAffinity(spec.Affinity, labels),
					Tolerations:     spec.ToleRations,
					SecurityContext: spec.SecurityContext,
					NodeSelector:    cluster.Spec.NodeSelector,
					Containers: []corev1.Container{
						redisServerContainer(cluster, password),
					},
					Volumes: volumes,
				},
			},
		},
	}

	if spec.Storage != nil && spec.Storage.Type == redisv1alpha1.PersistentClaim {
		ss.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{
			persistentClaim(cluster, labels),
		}
		if spec.Storage.DeleteClaim {
			// set an owner reference so the persistent volumes are deleted when the cluster be deleted.
			ss.Spec.VolumeClaimTemplates[0].OwnerReferences = ownerReferences(cluster)
		}
	}
	return ss
}

func getAffinity(affinity *corev1.Affinity, labels map[string]string) *corev1.Affinity {
	if affinity != nil {
		return affinity
	}

	// return a SOFT anti-affinity by default
	return &corev1.Affinity{
		PodAntiAffinity: &corev1.PodAntiAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
				{
					Weight: 100,
					PodAffinityTerm: corev1.PodAffinityTerm{
						TopologyKey: hostnameTopologyKey,
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: labels,
						},
					},
				},
			},
		},
	}
}

func persistentClaim(cluster *redisv1alpha1.DistributedRedisCluster, labels map[string]string) corev1.PersistentVolumeClaim {
	mode := corev1.PersistentVolumeFilesystem
	return corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:   redisStorageVolumeName,
			Labels: labels,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: cluster.Spec.Storage.Size,
				},
			},
			StorageClassName: &cluster.Spec.Storage.Class,
			VolumeMode:       &mode,
		},
	}
}

func ownerReferences(cluster *redisv1alpha1.DistributedRedisCluster) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		*metav1.NewControllerRef(cluster, schema.GroupVersionKind{
			Group:   redisv1alpha1.SchemeGroupVersion.Group,
			Version: redisv1alpha1.SchemeGroupVersion.Version,
			Kind:    redisv1alpha1.DistributedRedisClusterKind,
		}),
	}
}

func ClusterStatefulSetName(clusterName string) string {
	return fmt.Sprintf("drc-%s", clusterName)
}

func redisServerContainer(cluster *redisv1alpha1.DistributedRedisCluster, password *corev1.EnvVar) corev1.Container {
	args := []string{
		"--cluster-enabled yes",
		"--cluster-config-file /data/nodes.conf",
	}
	if password != nil {
		args = append(args, fmt.Sprintf("--requirepass '$(%s)'", passwordENV),
			fmt.Sprintf("--masterauth '$(%s)'", passwordENV))
	}

	cmd := []string{
		"redis-server",
	}

	probeArg := "redis-cli -h $(hostname)"

	container := corev1.Container{
		Name:  redisServerName,
		Image: cluster.Spec.Image,
		Ports: []corev1.ContainerPort{
			{
				Name:          "client",
				ContainerPort: 6379,
				Protocol:      corev1.ProtocolTCP,
			},
			{
				Name:          "gossip",
				ContainerPort: 16379,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		VolumeMounts: volumeMounts(),
		Command:      cmd,
		Args:         args,
		LivenessProbe: &corev1.Probe{
			InitialDelaySeconds: graceTime,
			TimeoutSeconds:      5,
			Handler: corev1.Handler{
				Exec: &corev1.ExecAction{
					Command: []string{
						"sh",
						"-c",
						probeArg,
					},
				},
			},
		},
		ReadinessProbe: &corev1.Probe{
			InitialDelaySeconds: graceTime,
			TimeoutSeconds:      5,
			Handler: corev1.Handler{
				Exec: &corev1.ExecAction{
					Command: []string{
						"sh",
						"-c",
						probeArg,
					},
				},
			},
		},
		Resources: *cluster.Spec.Resources,
		// TODO store redis data when pod stop
		//Lifecycle: &corev1.Lifecycle{
		//	PreStop: &corev1.Handler{
		//		Exec: &corev1.ExecAction{
		//			Command: []string{"/bin/sh", "-c", "/redis-shutdown/shutdown.sh"},
		//		},
		//	},
		//},
	}

	if password != nil {
		container.Env = []corev1.EnvVar{*password}
	}

	return container
}

func volumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      redisStorageVolumeName,
			MountPath: "/data",
		},
	}
}

// Returns the REDIS_PASSWORD environment variable.
func redisPassword(cluster *redisv1alpha1.DistributedRedisCluster) *corev1.EnvVar {
	if cluster.Spec.PasswordSecret == nil {
		return nil
	}
	secretName := cluster.Spec.PasswordSecret.Name

	return &corev1.EnvVar{
		Name: passwordENV,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
				},
				Key: "password",
			},
		},
	}
}

func redisVolumes(cluster *redisv1alpha1.DistributedRedisCluster) []corev1.Volume {
	var volumes []corev1.Volume
	dataVolume := redisDataVolume(cluster)
	if dataVolume != nil {
		volumes = append(volumes, *dataVolume)
	}
	return volumes
}

func emptyVolume() *corev1.Volume {
	return &corev1.Volume{
		Name: redisStorageVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}
func redisDataVolume(cluster *redisv1alpha1.DistributedRedisCluster) *corev1.Volume {
	// This will find the volumed desired by the user. If no volume defined
	// an EmptyDir will be used by default
	if cluster.Spec.Storage == nil {
		return emptyVolume()
	}

	switch cluster.Spec.Storage.Type {
	case redisv1alpha1.Ephemeral:
		return emptyVolume()
	case redisv1alpha1.PersistentClaim:
		return nil
	default:
		return emptyVolume()
	}
}
