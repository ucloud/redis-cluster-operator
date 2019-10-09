package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DistributedRedisClusterSpec defines the desired state of DistributedRedisCluster
// +k8s:openapi-gen=true
type DistributedRedisClusterSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
	Image           string                       `json:"image,omitempty"`
	Command         []string                     `json:"command,omitempty"`
	MasterSize      int32                        `json:"masterSize,omitempty"`
	ClusterReplicas int32                        `json:"clusterReplicas,omitempty"`
	ServiceName     string                       `json:"serviceName,omitempty"`
	Config          map[string]string            `json:"config,omitempty"`
	Affinity        *corev1.Affinity             `json:"affinity,omitempty"`
	NodeSelector    map[string]string            `json:"nodeSelector,omitempty"`
	ToleRations     []corev1.Toleration          `json:"toleRations,omitempty"`
	SecurityContext *corev1.PodSecurityContext   `json:"securityContext,omitempty"`
	Annotations     map[string]string            `json:"annotations,omitempty"`
	Storage         *RedisStorage                `json:"storage,omitempty"`
	Resources       *corev1.ResourceRequirements `json:"resources,omitempty"`
	PasswordSecret  *corev1.LocalObjectReference `json:"rootPasswordSecret,omitempty"`
}

// RedisStorage defines the structure used to store the Redis Data
type RedisStorage struct {
	Size        resource.Quantity `json:"size"`
	Type        StorageType       `json:"type"`
	Class       string            `json:"class"`
	DeleteClaim bool              `json:"deleteClaim",omitempty"`
}

// DistributedRedisClusterStatus defines the observed state of DistributedRedisCluster
// +k8s:openapi-gen=true
type DistributedRedisClusterStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
	Status         ClusterStatus      `json:"status"`
	Reason         string             `json:"reason,omitempty"`
	NumberOfMaster int32              `json:"numberOfMaster,omitempty"`
	Nodes          []RedisClusterNode `json:"nodes"`
}

// RedisClusterNode represent a RedisCluster Node
type RedisClusterNode struct {
	ID        string    `json:"id"`
	Role      RedisRole `json:"role"`
	IP        string    `json:"ip"`
	Port      string    `json:"port"`
	Slots     []string  `json:"slots,omitempty"`
	MasterRef string    `json:"masterRef,omitempty"`
	PodName   string    `json:"podName"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DistributedRedisCluster is the Schema for the distributedredisclusters API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=distributedredisclusters,scope=Namespaced
type DistributedRedisCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DistributedRedisClusterSpec   `json:"spec,omitempty"`
	Status DistributedRedisClusterStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DistributedRedisClusterList contains a list of DistributedRedisCluster
type DistributedRedisClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DistributedRedisCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DistributedRedisCluster{}, &DistributedRedisClusterList{})
}
