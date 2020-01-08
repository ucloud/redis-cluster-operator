package statefulsets

import (
	"reflect"
	"testing"
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
