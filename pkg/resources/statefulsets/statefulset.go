package statefulsets

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
	"github.com/ucloud/redis-cluster-operator/pkg/osm"
	"github.com/ucloud/redis-cluster-operator/pkg/resources/configmaps"
)

const (
	redisStorageVolumeName = "redis-data"
	redisServerName        = "redis"
	hostnameTopologyKey    = "kubernetes.io/hostname"

	graceTime = 30

	passwordENV = "REDIS_PASSWORD"

	configMapVolumeName = "conf"
)

// NewStatefulSetForCR creates a new StatefulSet for the given Cluster.
func NewStatefulSetForCR(cluster *redisv1alpha1.DistributedRedisCluster, backup *redisv1alpha1.RedisClusterBackup, labels map[string]string) (*appsv1.StatefulSet, error) {
	password := redisPassword(cluster)
	volumes := redisVolumes(cluster, backup)
	name := ClusterStatefulSetName(cluster.Name)
	namespace := cluster.Namespace
	spec := cluster.Spec
	size := spec.MasterSize * (spec.ClusterReplicas + 1)
	ss := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: redisv1alpha1.DefaultOwnerReferences(cluster),
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
					Labels:      labels,
					Annotations: cluster.Spec.Annotations,
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
			ss.Spec.VolumeClaimTemplates[0].OwnerReferences = redisv1alpha1.DefaultOwnerReferences(cluster)
		}
	}
	if spec.Monitor != nil {
		ss.Spec.Template.Spec.Containers = append(ss.Spec.Template.Spec.Containers, redisExporterContainer(cluster, password))
	}
	if spec.Init != nil {
		initContainer, err := redisInitContainer(backup, password)
		if err != nil {
			return nil, err
		}
		ss.Spec.Template.Spec.InitContainers = append(ss.Spec.Template.Spec.InitContainers, initContainer)
	}
	return ss, nil
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

func ClusterStatefulSetName(clusterName string) string {
	return fmt.Sprintf("drc-%s", clusterName)
}

func getRedisCommand(cluster *redisv1alpha1.DistributedRedisCluster, password *corev1.EnvVar) []string {
	cmd := []string{
		"/conf/fix-ip.sh",
		"redis-server",
		"--cluster-enabled yes",
		"--cluster-config-file /data/nodes.conf",
	}
	if password != nil {
		cmd = append(cmd, fmt.Sprintf("--requirepass '$(%s)'", passwordENV),
			fmt.Sprintf("--masterauth '$(%s)'", passwordENV))
	}
	if len(cluster.Spec.Command) > 0 {
		cmd = append(cmd, cluster.Spec.Command...)
	}
	return cmd
}

func redisServerContainer(cluster *redisv1alpha1.DistributedRedisCluster, password *corev1.EnvVar) corev1.Container {
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
		Command:      getRedisCommand(cluster, password),
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
		Env: []corev1.EnvVar{
			{
				Name: "POD_IP",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "status.podIP",
					},
				},
			},
		},
		Resources: *cluster.Spec.Resources,
		// TODO store redis data when pod stop
		Lifecycle: &corev1.Lifecycle{
			PreStop: &corev1.Handler{
				Exec: &corev1.ExecAction{
					Command: []string{"/bin/sh", "/conf/shutdown.sh"},
				},
			},
		},
	}

	if password != nil {
		container.Env = append(container.Env, *password)
	}

	return container
}

func redisExporterContainer(cluster *redisv1alpha1.DistributedRedisCluster, password *corev1.EnvVar) corev1.Container {
	container := corev1.Container{
		Name: "exporter",
		Args: append([]string{
			fmt.Sprintf("--web.listen-address=:%v", cluster.Spec.Monitor.Prometheus.Port),
			fmt.Sprintf("--web.telemetry-path=%v", redisv1alpha1.PrometheusExporterTelemetryPath),
		}, cluster.Spec.Monitor.Args...),
		Image:           cluster.Spec.Monitor.Image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Ports: []corev1.ContainerPort{
			{
				Name:          "prom-http",
				Protocol:      corev1.ProtocolTCP,
				ContainerPort: cluster.Spec.Monitor.Prometheus.Port,
			},
		},
		Env:             cluster.Spec.Monitor.Env,
		Resources:       cluster.Spec.Monitor.Resources,
		SecurityContext: cluster.Spec.Monitor.SecurityContext,
	}
	if password != nil {
		container.Env = append(container.Env, *password)
	}
	return container
}

func redisInitContainer(backup *redisv1alpha1.RedisClusterBackup, password *corev1.EnvVar) (corev1.Container, error) {
	backupSpec := backup.Spec.Backend
	bucket, err := backupSpec.Container()
	if err != nil {
		return corev1.Container{}, err
	}
	folderName, err := backup.Location()
	if err != nil {
		return corev1.Container{}, err
	}
	container := corev1.Container{
		Name:            redisv1alpha1.JobTypeRestore,
		Image:           backup.Spec.Image,
		ImagePullPolicy: "Always",
		Args: []string{
			redisv1alpha1.JobTypeRestore,
			fmt.Sprintf(`--data-dir=%s`, redisv1alpha1.BackupDumpDir),
			fmt.Sprintf(`--bucket=%s`, bucket),
			fmt.Sprintf(`--enable-analytics=%v`, "false"),
			fmt.Sprintf(`--folder=%s`, folderName),
			fmt.Sprintf(`--snapshot=%s`, backup.Name),
			"--",
		},
		Env: []corev1.EnvVar{
			{
				Name: "POD_NAME",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.name",
					},
				},
			},
		},
		Resources:      backup.Spec.PodSpec.Resources,
		LivenessProbe:  backup.Spec.PodSpec.LivenessProbe,
		ReadinessProbe: backup.Spec.PodSpec.ReadinessProbe,
		Lifecycle:      backup.Spec.PodSpec.Lifecycle,
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      redisStorageVolumeName,
				MountPath: redisv1alpha1.BackupDumpDir,
			},
			{
				Name:      "osmconfig",
				ReadOnly:  true,
				MountPath: osm.SecretMountPath,
			},
		},
	}
	if password != nil {
		container.Env = append(container.Env, *password)
	}
	return container, nil
}

func volumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      redisStorageVolumeName,
			MountPath: "/data",
		},
		{
			Name:      configMapVolumeName,
			MountPath: "/conf",
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

func redisVolumes(cluster *redisv1alpha1.DistributedRedisCluster, backup *redisv1alpha1.RedisClusterBackup) []corev1.Volume {
	executeMode := int32(0755)
	volumes := []corev1.Volume{
		{
			Name: configMapVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: configmaps.RedisConfigMapName(cluster.Name),
					},
					DefaultMode: &executeMode,
				},
			},
		},
	}

	dataVolume := redisDataVolume(cluster)
	if dataVolume != nil {
		volumes = append(volumes, *dataVolume)
	}
	if cluster.Spec.Init != nil {
		volumes = append(volumes, corev1.Volume{
			Name: "osmconfig",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: backup.OSMSecretName(),
				},
			},
		})
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
