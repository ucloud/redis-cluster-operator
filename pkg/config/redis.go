package config

import (
	"path"

	"github.com/spf13/pflag"
)

const (
	// DefaultRedisTimeout default redis timeout (ms)
	DefaultRedisTimeout = 2000
	//DefaultClusterNodeTimeout default cluster node timeout (ms)
	//The maximum amount of time a Redis Cluster node can be unavailable, without it being considered as failing
	DefaultClusterNodeTimeout = 2000
	// RedisRenameCommandsDefaultPath default path to volume storing rename commands
	RedisRenameCommandsDefaultPath = "/etc/secret-volume"
	// RedisRenameCommandsDefaultFile default file name containing rename commands
	RedisRenameCommandsDefaultFile = ""
	// RedisConfigFileDefault default config file path
	RedisConfigFileDefault = "/redis-conf/redis.conf"
	// RedisServerBinDefault default binary name
	RedisServerBinDefault = "redis-server"
	// RedisServerPortDefault default redis port
	RedisServerPortDefault = "6379"
	// RedisMaxMemoryDefault default redis max memory
	RedisMaxMemoryDefault = 0
	// RedisMaxMemoryPolicyDefault default redis max memory evition policy
	RedisMaxMemoryPolicyDefault = "noeviction"
)

//var redisFlagSet *pflag.FlagSet
//
//func init() {
//	redisFlagSet = pflag.NewFlagSet("redis", pflag.ExitOnError)
//}

var redisConf *Redis

func init() {
	redisConf = &Redis{}
}

func RedisConf() *Redis {
	return redisConf
}

// Redis used to store all Redis configuration information
type Redis struct {
	DialTimeout        int
	ClusterNodeTimeout int
	ConfigFileName     string
	RenameCommandsPath string
	RenameCommandsFile string
	HTTPServerAddr     string
	ServerBin          string
	ServerPort         string
	ServerIP           string
	MaxMemory          uint32
	MaxMemoryPolicy    string
	ConfigFiles        []string
}

// AddFlags use to add the Redis Config flags to the command line
func (r *Redis) AddFlags(fs *pflag.FlagSet) {
	fs.IntVar(&r.DialTimeout, "rdt", DefaultRedisTimeout, "redis dial timeout (ms)")
	fs.IntVar(&r.ClusterNodeTimeout, "cluster-node-timeout", DefaultClusterNodeTimeout, "redis node timeout (ms)")
	fs.StringVar(&r.ConfigFileName, "c", RedisConfigFileDefault, "redis config file path")
	fs.StringVar(&r.RenameCommandsPath, "rename-command-path", RedisRenameCommandsDefaultPath, "Path to the folder where rename-commands option for redis are available")
	fs.StringVar(&r.RenameCommandsFile, "rename-command-file", RedisRenameCommandsDefaultFile, "Name of the file where rename-commands option for redis are available, disabled if empty")
	fs.Uint32Var(&r.MaxMemory, "max-memory", RedisMaxMemoryDefault, "redis max memory")
	fs.StringVar(&r.MaxMemoryPolicy, "max-memory-policy", RedisMaxMemoryPolicyDefault, "redis max memory evition policy")
	fs.StringVar(&r.ServerBin, "bin", RedisServerBinDefault, "redis server binary file name")
	fs.StringVar(&r.ServerPort, "port", RedisServerPortDefault, "redis server listen port")
	fs.StringVar(&r.ServerIP, "ip", "", "redis server listen ip")
	fs.StringArrayVar(&r.ConfigFiles, "config-file", []string{}, "Location of redis configuration file that will be include in the ")
}

// GetRenameCommandsFile return the path to the rename command file, or empty string if not define
func (r *Redis) GetRenameCommandsFile() string {
	if r.RenameCommandsFile == "" {
		return ""
	}
	return path.Join(r.RenameCommandsPath, r.RenameCommandsFile)
}
