package v1alpha1

import (
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
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
	Image            string        `json:"image,omitempty"`
	RedisClusterName string        `json:"redisClusterName"`
	Storage          *RedisStorage `json:"storage,omitempty"`
	// Snapshot Spec
	store.Backend `json:",inline"`
}

type BackupPhase string

const (
	// used for Backup that are currently running
	BackupPhaseRunning BackupPhase = "Running"
	// used for Backup that are Succeeded
	BackupPhaseSucceeded BackupPhase = "Succeeded"
	// used for Backup that are Failed
	BackupPhaseFailed BackupPhase = "Failed"
)

// RedisClusterBackupStatus defines the observed state of RedisClusterBackup
// +k8s:openapi-gen=true
type RedisClusterBackupStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
	StartTime      *metav1.Time `json:"startTime,omitempty"`
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
	Phase          BackupPhase  `json:"phase,omitempty"`
	Reason         string       `json:"reason,omitempty"`
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
