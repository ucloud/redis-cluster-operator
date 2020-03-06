package manager

import (
	"testing"

	"github.com/go-logr/logr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("test")

func Test_isRedisConfChanged(t *testing.T) {
	type args struct {
		confInCm    string
		currentConf map[string]string
		log         logr.Logger
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "should false",
			args: args{
				confInCm: `appendfsync everysec
appendonly yes
auto-aof-rewrite-min-size 67108864
save 900 1 300 10`,
				currentConf: map[string]string{
					"appendfsync":               "everysec",
					"appendonly":                "yes",
					"auto-aof-rewrite-min-size": "67108864",
					"save":                      "900 1 300 10",
				},
				log: log,
			},
			want: false,
		},
		{
			name: "should false with newline",
			args: args{
				confInCm: `appendfsync everysec
appendonly yes
auto-aof-rewrite-min-size 67108864
save 900 1 300 10
`,
				currentConf: map[string]string{
					"appendfsync":               "everysec",
					"appendonly":                "yes",
					"auto-aof-rewrite-min-size": "67108864",
					"save":                      "900 1 300 10",
				},
				log: log,
			},
			want: false,
		},
		{
			name: "should true, compare value",
			args: args{
				confInCm: `appendfsync everysec
appendonly yes
auto-aof-rewrite-min-size 6710886
save 900 1 300 10
`,
				currentConf: map[string]string{
					"appendfsync":               "everysec",
					"appendonly":                "yes",
					"auto-aof-rewrite-min-size": "67108864",
					"save":                      "900 1 300 10",
				},
				log: log,
			},
			want: true,
		},
		{
			name: "should true, add current",
			args: args{
				confInCm: `appendfsync everysec
appendonly yes
save 900 1 300 10
`,
				currentConf: map[string]string{
					"appendfsync":               "everysec",
					"appendonly":                "yes",
					"auto-aof-rewrite-min-size": "67108864",
					"save":                      "900 1 300 10",
				},
				log: log,
			},
			want: true,
		},
		{
			name: "should true, del current",
			args: args{
				confInCm: `appendfsync everysec
appendonly yes
auto-aof-rewrite-min-size 67108864
save 900 1 300 10
`,
				currentConf: map[string]string{
					"appendfsync": "everysec",
					"appendonly":  "yes",
					"save":        "900 1 300 10",
				},
				log: log,
			},
			want: true,
		},
		{
			name: "should true, compare key",
			args: args{
				confInCm: `appendfsync everysec
appendonly yes
save 900 1 300 10
`,
				currentConf: map[string]string{
					"appendonly":                "yes",
					"auto-aof-rewrite-min-size": "67108864",
					"save":                      "900 1 300 10",
				},
				log: log,
			},
			want: true,
		},
		{
			name: "should true, compare save",
			args: args{
				confInCm: `appendfsync everysec
appendonly yes
auto-aof-rewrite-min-size 67108864
save 900 1 300 10
`,
				currentConf: map[string]string{
					"appendfsync":               "everysec",
					"appendonly":                "yes",
					"auto-aof-rewrite-min-size": "67108864",
					"save":                      "900 1",
				},
				log: log,
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRedisConfChanged(tt.args.confInCm, tt.args.currentConf, tt.args.log); got != tt.want {
				t.Errorf("isRedisConfChanged() = %v, want %v", got, tt.want)
			}
		})
	}
}
