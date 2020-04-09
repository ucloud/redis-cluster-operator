package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	store "kmodules.xyz/objectstore-api/api/v1"
)

const (
	ResourceSingularBackup = "backup"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// RedisClusterBackupSpec defines the desired state of RedisClusterBackup
// +k8s:openapi-gen=true
type RedisClusterBackupSpec struct {
	Image                 string        `json:"image,omitempty"`
	RedisClusterName      string        `json:"redisClusterName"`
	Storage               *RedisStorage `json:"storage,omitempty"`
	store.Backend         `json:",inline"`
	PodSpec               *PodSpec `json:"podSpec,omitempty"`
	ActiveDeadlineSeconds *int64   `json:"activeDeadlineSeconds,omitempty"`
}

type PodSpec struct {
	// ServiceAccountName is the name of the ServiceAccount to use to run this pod.
	// More info: https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// Arguments to the entrypoint.
	// The docker image's CMD is used if this is not provided.
	// Variable references $(VAR_NAME) are expanded using the container's environment. If a variable
	// cannot be resolved, the reference in the input string will be unchanged. The $(VAR_NAME) syntax
	// can be escaped with a double $$, ie: $$(VAR_NAME). Escaped references will never be expanded,
	// regardless of whether the variable exists or not.
	// Cannot be updated.
	// More info: https://kubernetes.io/docs/tasks/inject-data-application/define-command-argument-container/#running-a-command-in-a-shell
	// +optional
	Args []string `json:"args,omitempty"`

	// NodeSelector is a selector which must be true for the pod to fit on a node.
	// Selector which must match a node's labels for the pod to be scheduled on that node.
	// More info: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Compute Resources required by the sidecar container.
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// If specified, the pod's scheduling constraints
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// If specified, the pod will be dispatched by specified scheduler.
	// If not specified, the pod will be dispatched by default scheduler.
	// +optional
	SchedulerName string `json:"schedulerName,omitempty"`

	// If specified, the pod's tolerations.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// ImagePullSecrets is an optional list of references to secrets in the same namespace to use for pulling any of the images used by this PodSpec.
	// If specified, these secrets will be passed to individual puller implementations for them to use.
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// List of environment variables to set in the container.
	// Cannot be updated.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// List of initialization containers belonging to the pod.
	// Init containers are executed in order prior to containers being started. If any
	// init container fails, the pod is considered to have failed and is handled according
	// to its restartPolicy. The name for an init container or normal container must be
	// unique among all containers.
	// Init containers may not have Lifecycle actions, Readiness probes, or Liveness probes.
	// The resourceRequirements of an init container are taken into account during scheduling
	// by finding the highest request/limit for each resource type, and then using the max of
	// of that value or the sum of the normal containers. Limits are applied to init containers
	// in a similar fashion.
	// Init containers cannot currently be added or removed.
	// Cannot be updated.
	// More info: https://kubernetes.io/docs/concepts/workloads/pods/init-containers/
	// +patchMergeKey=name
	// +patchStrategy=merge
	InitContainers []corev1.Container `json:"initContainers,omitempty" patchStrategy:"merge" patchMergeKey:"name"`

	// If specified, indicates the pod's priority. "system-node-critical" and
	// "system-cluster-critical" are two special keywords which indicate the
	// highest priorities with the former being the highest priority. Any other
	// name must be defined by creating a PriorityClass object with that name.
	// If not specified, the pod priority will be default or zero if there is no
	// default.
	// +optional
	PriorityClassName string `json:"priorityClassName,omitempty"`
	// The priority value. Various system components use this field to find the
	// priority of the pod. When Priority Admission Controller is enabled, it
	// prevents users from setting this field. The admission controller populates
	// this field from PriorityClassName.
	// The higher the value, the higher the priority.
	// +optional
	Priority *int32 `json:"priority,omitempty"`

	// SecurityContext holds pod-level security attributes and common container settings.
	// Optional: Defaults to empty.  See type description for default values of each field.
	// +optional
	SecurityContext *corev1.PodSecurityContext `json:"securityContext,omitempty"`

	// Periodic probe of container liveness.
	// Container will be restarted if the probe fails.
	// Controllers may set default LivenessProbe if no liveness probe is provided.
	// To ignore defaulting, set the value to empty LivenessProbe "{}".
	// Cannot be updated.
	// More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes
	// +optional
	LivenessProbe *corev1.Probe `json:"livenessProbe,omitempty"`

	// Periodic probe of container service readiness.
	// Container will be removed from service endpoints if the probe fails.
	// Cannot be updated.
	// Controllers may set default ReadinessProbe if no readyness probe is provided.
	// To ignore defaulting, set the value to empty ReadynessProbe "{}".
	// More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes
	// +optional
	ReadinessProbe *corev1.Probe `json:"readinessProbe,omitempty"`

	// Actions that the management system should take in response to container lifecycle events.
	// Cannot be updated.
	// +optional
	Lifecycle *corev1.Lifecycle `json:"lifecycle,omitempty"`
}

type BackupPhase string

const (
	// used for Backup that are currently running
	BackupPhaseRunning BackupPhase = "Running"
	// used for Backup that are Succeeded
	BackupPhaseSucceeded BackupPhase = "Succeeded"
	// used for Backup that are Failed
	BackupPhaseFailed BackupPhase = "Failed"
	// used for Backup that are Ignored
	BackupPhaseIgnored BackupPhase = "Ignored"
)

// RedisClusterBackupStatus defines the observed state of RedisClusterBackup
// +k8s:openapi-gen=true
type RedisClusterBackupStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	StartTime       *metav1.Time `json:"startTime,omitempty"`
	CompletionTime  *metav1.Time `json:"completionTime,omitempty"`
	Phase           BackupPhase  `json:"phase,omitempty"`
	Reason          string       `json:"reason,omitempty"`
	MasterSize      int32        `json:"masterSize,omitempty"`
	ClusterReplicas int32        `json:"clusterReplicas,omitempty"`
	ClusterImage    string       `json:"clusterImage,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RedisClusterBackup is the Schema for the redisclusterbackups API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=redisclusterbackups,scope=Namespaced
type RedisClusterBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RedisClusterBackupSpec   `json:"spec,omitempty"`
	Status RedisClusterBackupStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RedisClusterBackupList contains a list of RedisClusterBackup
type RedisClusterBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RedisClusterBackup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RedisClusterBackup{}, &RedisClusterBackupList{})
}
