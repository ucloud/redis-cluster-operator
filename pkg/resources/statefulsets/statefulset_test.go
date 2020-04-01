package statefulsets

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func Test_mergeRenameCmds(t *testing.T) {
	type args struct {
		userCmds           []string
		systemRenameCmdMap map[string]string
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "test No intersection",
			args: args{
				userCmds: []string{
					"--maxmemory 2gb",
					"--rename-command BGSAVE pp14qluk",
					"--rename-command CONFIG lni07z1p",
				},
				systemRenameCmdMap: map[string]string{
					"SAVE":  "6on30p6z",
					"DEBUG": "8a4insyv",
				},
			},
			want: []string{
				"--maxmemory 2gb",
				"--rename-command BGSAVE pp14qluk",
				"--rename-command CONFIG lni07z1p",
				"--rename-command DEBUG 8a4insyv",
				"--rename-command SAVE 6on30p6z",
			},
		},
		{
			name: "test intersection",
			args: args{
				userCmds: []string{
					"--rename-command BGSAVE pp14qluk",
					"--rename-command CONFIG lni07z1p",
				},
				systemRenameCmdMap: map[string]string{
					"BGSAVE": "fadfgad",
					"SAVE":   "6on30p6z",
					"DEBUG":  "8a4insyv",
				},
			},
			want: []string{
				"--rename-command CONFIG lni07z1p",
				"--rename-command BGSAVE fadfgad",
				"--rename-command DEBUG 8a4insyv",
				"--rename-command SAVE 6on30p6z",
			},
		},
		{
			name: "test complex",
			args: args{
				userCmds: []string{
					"--maxmemory 2gb",
					"--rename-command BGSAVE pp14qluk",
					"--rename-command CONFIG lni07z1p",
					`--rename-command FLUSHALL ""`,
				},
				systemRenameCmdMap: map[string]string{
					"BGSAVE": "fadfgad",
					"SAVE":   "6on30p6z",
					"DEBUG":  "8a4insyv",
				},
			},
			want: []string{
				"--maxmemory 2gb",
				"--rename-command CONFIG lni07z1p",
				`--rename-command FLUSHALL ""`,
				"--rename-command BGSAVE fadfgad",
				"--rename-command DEBUG 8a4insyv",
				"--rename-command SAVE 6on30p6z",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mergeRenameCmds(tt.args.userCmds, tt.args.systemRenameCmdMap); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("mergeRenameCmds() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_customContainerEnv(t *testing.T) {
	type args struct {
		env       []corev1.EnvVar
		customEnv []corev1.EnvVar
	}
	tests := []struct {
		name string
		args args
		want []corev1.EnvVar
	}{
		{
			name: "nil all",
			args: args{
				env:       nil,
				customEnv: nil,
			},
			want: nil,
		},
		{
			name: "nil env",
			args: args{
				env: nil,
				customEnv: []corev1.EnvVar{{
					Name:      "foo",
					Value:     "",
					ValueFrom: nil,
				}},
			},
			want: []corev1.EnvVar{{
				Name:      "foo",
				Value:     "",
				ValueFrom: nil,
			}},
		},
		{
			name: "nil custom env",
			args: args{
				customEnv: nil,
				env: []corev1.EnvVar{{
					Name:      "foo",
					Value:     "",
					ValueFrom: nil,
				}},
			},
			want: []corev1.EnvVar{{
				Name:      "foo",
				Value:     "",
				ValueFrom: nil,
			}},
		},
		{
			name: "env for bar",
			args: args{
				env: []corev1.EnvVar{{
					Name:      "foo",
					Value:     "",
					ValueFrom: nil,
				}},
				customEnv: []corev1.EnvVar{{
					Name:      "bar",
					Value:     "",
					ValueFrom: nil,
				}},
			},
			want: []corev1.EnvVar{{
				Name:      "foo",
				Value:     "",
				ValueFrom: nil,
			}, {
				Name:      "bar",
				Value:     "",
				ValueFrom: nil,
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := customContainerEnv(tt.args.env, tt.args.customEnv); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("customContainerEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}
