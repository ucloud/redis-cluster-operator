package configmaps

import (
	"testing"
)

func Test_generateRedisConfContent(t *testing.T) {
	confMap := map[string]string{
		"activerehashing":               "yes",
		"appendfsync":                   "everysec",
		"appendonly":                    "yes",
		"auto-aof-rewrite-min-size":     "67108864",
		"auto-aof-rewrite-percentage":   "100",
		"cluster-node-timeout":          "15000",
		"cluster-require-full-coverage": "yes",
		"hash-max-ziplist-entries":      "512",
		"hash-max-ziplist-value":        "64",
		"hll-sparse-max-bytes":          "3000",
		"list-compress-depth":           "0",
		"maxmemory":                     "1000000000",
		"maxmemory-policy":              "noeviction",
		"maxmemory-samples":             "5",
		"no-appendfsync-on-rewrite":     "no",
		"notify-keyspace-events":        "",
		"repl-backlog-size":             "1048576",
		"repl-backlog-ttl":              "3600",
		"set-max-intset-entries":        "512",
		"slowlog-log-slower-than":       "10000",
		"slowlog-max-len":               "128",
		"stop-writes-on-bgsave-error":   "yes",
		"tcp-keepalive":                 "0",
		"timeout":                       "0",
		"zset-max-ziplist-entries":      "128",
		"zset-max-ziplist-value":        "64",
	}
	want := `activerehashing yes
appendfsync everysec
appendonly yes
auto-aof-rewrite-min-size 67108864
auto-aof-rewrite-percentage 100
cluster-node-timeout 15000
cluster-require-full-coverage yes
hash-max-ziplist-entries 512
hash-max-ziplist-value 64
hll-sparse-max-bytes 3000
list-compress-depth 0
maxmemory 1000000000
maxmemory-policy noeviction
maxmemory-samples 5
no-appendfsync-on-rewrite no
repl-backlog-size 1048576
repl-backlog-ttl 3600
set-max-intset-entries 512
slowlog-log-slower-than 10000
slowlog-max-len 128
stop-writes-on-bgsave-error yes
tcp-keepalive 0
timeout 0
zset-max-ziplist-entries 128
zset-max-ziplist-value 64
`
	type args struct {
		configMap map[string]string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "test",
			args: struct{ configMap map[string]string }{configMap: confMap},
			want: want,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := generateRedisConfContent(tt.args.configMap); got != tt.want {
				t.Errorf("generateRedisConfContent()\n[%v], want\n[%v]", got, tt.want)
			}
		})
	}
}
