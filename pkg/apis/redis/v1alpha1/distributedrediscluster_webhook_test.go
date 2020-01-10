package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDistributedRedisCluster_ValidateCreate(t *testing.T) {
	type fields struct {
		TypeMeta   v1.TypeMeta
		ObjectMeta v1.ObjectMeta
		Spec       DistributedRedisClusterSpec
		Status     DistributedRedisClusterStatus
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "invalid ServiceName",
			fields: fields{
				Spec: DistributedRedisClusterSpec{
					Image:           "",
					Command:         nil,
					MasterSize:      0,
					ClusterReplicas: 0,
					ServiceName:     "fad_ad",
					Config:          nil,
					Affinity:        nil,
					NodeSelector:    nil,
					ToleRations:     nil,
					SecurityContext: nil,
					Annotations:     nil,
					Storage:         nil,
					Resources:       nil,
					PasswordSecret:  nil,
					Monitor:         nil,
					Init:            nil,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid Resources",
			fields: fields{
				Spec: DistributedRedisClusterSpec{
					Image:           "",
					MasterSize:      0,
					ClusterReplicas: 0,
					ServiceName:     "fad-fad",
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("200m"),
							corev1.ResourceMemory: resource.MustParse("2Gi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "",
			fields: fields{
				Spec: DistributedRedisClusterSpec{
					Image:           "",
					MasterSize:      0,
					ClusterReplicas: 0,
					ServiceName:     "fad-fad",
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("200m"),
							corev1.ResourceMemory: resource.MustParse("2Gi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("200m"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := &DistributedRedisCluster{
				TypeMeta:   tt.fields.TypeMeta,
				ObjectMeta: tt.fields.ObjectMeta,
				Spec:       tt.fields.Spec,
				Status:     tt.fields.Status,
			}
			if err := in.ValidateCreate(); (err != nil) != tt.wantErr {
				t.Errorf("ValidateCreate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDistributedRedisCluster_ValidateUpdate(t *testing.T) {
	type fields struct {
		TypeMeta   v1.TypeMeta
		ObjectMeta v1.ObjectMeta
		Spec       DistributedRedisClusterSpec
		Status     DistributedRedisClusterStatus
	}
	type args struct {
		old runtime.Object
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "invalid update masterSize",
			fields: fields{
				Spec: DistributedRedisClusterSpec{
					Image:           "",
					MasterSize:      3,
					ClusterReplicas: 1,
					ServiceName:     "",
					Resources:       nil,
					PasswordSecret:  nil,
				},
				Status: DistributedRedisClusterStatus{
					Status: ClusterStatusScaling,
				},
			},
			args: args{
				old: &DistributedRedisCluster{
					Spec: DistributedRedisClusterSpec{
						Image:           "",
						MasterSize:      4,
						ClusterReplicas: 1,
						ServiceName:     "",
						Resources:       nil,
						PasswordSecret:  nil,
					},
					Status: DistributedRedisClusterStatus{},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid update image",
			fields: fields{
				Spec: DistributedRedisClusterSpec{
					Image:           "redis:5.0.6",
					MasterSize:      3,
					ClusterReplicas: 1,
					ServiceName:     "",
					Resources:       nil,
					PasswordSecret:  nil,
				},
				Status: DistributedRedisClusterStatus{
					Status: ClusterStatusScaling,
				},
			},
			args: args{
				old: &DistributedRedisCluster{
					Spec: DistributedRedisClusterSpec{
						Image:           "redis:5.0.4",
						MasterSize:      3,
						ClusterReplicas: 1,
						ServiceName:     "",
						Resources:       nil,
						PasswordSecret:  nil,
					},
					Status: DistributedRedisClusterStatus{},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid update password 1",
			fields: fields{
				Spec: DistributedRedisClusterSpec{
					Image:           "redis:5.0.4",
					MasterSize:      3,
					ClusterReplicas: 1,
					ServiceName:     "",
					Resources:       nil,
					PasswordSecret:  &corev1.LocalObjectReference{Name: "pass"},
				},
				Status: DistributedRedisClusterStatus{
					Status: ClusterStatusScaling,
				},
			},
			args: args{
				old: &DistributedRedisCluster{
					Spec: DistributedRedisClusterSpec{
						Image:           "redis:5.0.4",
						MasterSize:      3,
						ClusterReplicas: 1,
						ServiceName:     "",
						Resources:       nil,
						PasswordSecret:  &corev1.LocalObjectReference{Name: "password"},
					},
					Status: DistributedRedisClusterStatus{},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid update password 2",
			fields: fields{
				Spec: DistributedRedisClusterSpec{
					Image:           "redis:5.0.4",
					MasterSize:      3,
					ClusterReplicas: 1,
					ServiceName:     "",
					Resources:       nil,
					PasswordSecret:  &corev1.LocalObjectReference{Name: "pass"},
				},
				Status: DistributedRedisClusterStatus{
					Status: ClusterStatusScaling,
				},
			},
			args: args{
				old: &DistributedRedisCluster{
					Spec: DistributedRedisClusterSpec{
						Image:           "redis:5.0.4",
						MasterSize:      3,
						ClusterReplicas: 1,
						ServiceName:     "",
						Resources:       nil,
						PasswordSecret:  nil,
					},
					Status: DistributedRedisClusterStatus{},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid update resource",
			fields: fields{
				Spec: DistributedRedisClusterSpec{
					Image:           "redis:5.0.4",
					MasterSize:      3,
					ClusterReplicas: 1,
					ServiceName:     "",
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("200m"),
							corev1.ResourceMemory: resource.MustParse("2Gi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("200m"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
					PasswordSecret: nil,
				},
				Status: DistributedRedisClusterStatus{
					Status: ClusterStatusScaling,
				},
			},
			args: args{
				old: &DistributedRedisCluster{
					Spec: DistributedRedisClusterSpec{
						Image:           "redis:5.0.4",
						MasterSize:      3,
						ClusterReplicas: 1,
						ServiceName:     "",
						Resources: &corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("300m"),
								corev1.ResourceMemory: resource.MustParse("2Gi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("300m"),
								corev1.ResourceMemory: resource.MustParse("4Gi"),
							},
						},
						PasswordSecret: nil,
					},
					Status: DistributedRedisClusterStatus{},
				},
			},
			wantErr: true,
		},

		{
			name: "update masterSize",
			fields: fields{
				Spec: DistributedRedisClusterSpec{
					Image:           "",
					MasterSize:      3,
					ClusterReplicas: 1,
					ServiceName:     "",
					Resources:       nil,
					PasswordSecret:  nil,
				},
				Status: DistributedRedisClusterStatus{
					Status: ClusterStatusOK,
				},
			},
			args: args{
				old: &DistributedRedisCluster{
					Spec: DistributedRedisClusterSpec{
						Image:           "",
						MasterSize:      4,
						ClusterReplicas: 1,
						ServiceName:     "",
						Resources:       nil,
						PasswordSecret:  nil,
					},
					Status: DistributedRedisClusterStatus{},
				},
			},
			wantErr: false,
		},
		{
			name: "update image",
			fields: fields{
				Spec: DistributedRedisClusterSpec{
					Image:           "redis:5.0.6",
					MasterSize:      3,
					ClusterReplicas: 1,
					ServiceName:     "",
					Resources:       nil,
					PasswordSecret:  nil,
				},
				Status: DistributedRedisClusterStatus{
					Status: ClusterStatusOK,
				},
			},
			args: args{
				old: &DistributedRedisCluster{
					Spec: DistributedRedisClusterSpec{
						Image:           "redis:5.0.4",
						MasterSize:      3,
						ClusterReplicas: 1,
						ServiceName:     "",
						Resources:       nil,
						PasswordSecret:  nil,
					},
					Status: DistributedRedisClusterStatus{},
				},
			},
			wantErr: false,
		},
		{
			name: "update password 1",
			fields: fields{
				Spec: DistributedRedisClusterSpec{
					Image:           "redis:5.0.4",
					MasterSize:      3,
					ClusterReplicas: 1,
					ServiceName:     "",
					Resources:       nil,
					PasswordSecret:  &corev1.LocalObjectReference{Name: "pass"},
				},
				Status: DistributedRedisClusterStatus{
					Status: ClusterStatusOK,
				},
			},
			args: args{
				old: &DistributedRedisCluster{
					Spec: DistributedRedisClusterSpec{
						Image:           "redis:5.0.4",
						MasterSize:      3,
						ClusterReplicas: 1,
						ServiceName:     "",
						Resources:       nil,
						PasswordSecret:  &corev1.LocalObjectReference{Name: "password"},
					},
					Status: DistributedRedisClusterStatus{},
				},
			},
			wantErr: false,
		},
		{
			name: "update password 2",
			fields: fields{
				Spec: DistributedRedisClusterSpec{
					Image:           "redis:5.0.4",
					MasterSize:      3,
					ClusterReplicas: 1,
					ServiceName:     "",
					Resources:       nil,
					PasswordSecret:  &corev1.LocalObjectReference{Name: "pass"},
				},
				Status: DistributedRedisClusterStatus{
					Status: ClusterStatusOK,
				},
			},
			args: args{
				old: &DistributedRedisCluster{
					Spec: DistributedRedisClusterSpec{
						Image:           "redis:5.0.4",
						MasterSize:      3,
						ClusterReplicas: 1,
						ServiceName:     "",
						Resources:       nil,
						PasswordSecret:  nil,
					},
					Status: DistributedRedisClusterStatus{},
				},
			},
			wantErr: false,
		},
		{
			name: "update resource",
			fields: fields{
				Spec: DistributedRedisClusterSpec{
					Image:           "redis:5.0.4",
					MasterSize:      3,
					ClusterReplicas: 1,
					ServiceName:     "",
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("200m"),
							corev1.ResourceMemory: resource.MustParse("2Gi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("200m"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
					PasswordSecret: nil,
				},
				Status: DistributedRedisClusterStatus{
					Status: ClusterStatusOK,
				},
			},
			args: args{
				old: &DistributedRedisCluster{
					Spec: DistributedRedisClusterSpec{
						Image:           "redis:5.0.4",
						MasterSize:      3,
						ClusterReplicas: 1,
						ServiceName:     "",
						Resources: &corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("300m"),
								corev1.ResourceMemory: resource.MustParse("2Gi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("300m"),
								corev1.ResourceMemory: resource.MustParse("4Gi"),
							},
						},
						PasswordSecret: nil,
					},
					Status: DistributedRedisClusterStatus{},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := &DistributedRedisCluster{
				TypeMeta:   tt.fields.TypeMeta,
				ObjectMeta: tt.fields.ObjectMeta,
				Spec:       tt.fields.Spec,
				Status:     tt.fields.Status,
			}
			if err := in.ValidateUpdate(tt.args.old); (err != nil) != tt.wantErr {
				t.Errorf("ValidateUpdate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
