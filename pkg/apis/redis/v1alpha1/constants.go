package v1alpha1

type StorageType string

const (
	PersistentClaim StorageType = "persistent-claim"
	Ephemeral       StorageType = "ephemeral"
)

const (
	OperatorName      = "redis-cluster-operator"
	LabelManagedByKey = "managed-by"
	LabelNameKey      = "distributed-redis-cluster"
	StatefulSetLabel  = "statefulSet"
	PasswordENV       = "REDIS_PASSWORD"
)

// RedisRole RedisCluster Node Role type
type RedisRole string

const (
	// RedisClusterNodeRoleMaster RedisCluster Master node role
	RedisClusterNodeRoleMaster RedisRole = "Master"
	// RedisClusterNodeRoleSlave RedisCluster Master node role
	RedisClusterNodeRoleSlave RedisRole = "Slave"
	// RedisClusterNodeRoleNone None node role
	RedisClusterNodeRoleNone RedisRole = "None"
)

// ClusterStatus Redis Cluster status
type ClusterStatus string

const (
	// ClusterStatusOK ClusterStatus OK
	ClusterStatusOK ClusterStatus = "Healthy"
	// ClusterStatusKO ClusterStatus KO
	ClusterStatusKO ClusterStatus = "Failed"
	// ClusterStatusCreating ClusterStatus Creating
	ClusterStatusCreating = "Creating"
	// ClusterStatusScaling ClusterStatus Scaling
	ClusterStatusScaling ClusterStatus = "Scaling"
	// ClusterStatusCalculatingRebalancing ClusterStatus Rebalancing
	ClusterStatusCalculatingRebalancing ClusterStatus = "Calculating Rebalancing"
	// ClusterStatusRebalancing ClusterStatus Rebalancing
	ClusterStatusRebalancing ClusterStatus = "Rebalancing"
	// ClusterStatusRollingUpdate ClusterStatus RollingUpdate
	ClusterStatusRollingUpdate ClusterStatus = "RollingUpdate"
	// ClusterStatusResetPassword ClusterStatus ResetPassword
	ClusterStatusResetPassword ClusterStatus = "ResetPassword"
)

// NodesPlacementInfo Redis Nodes placement mode information
type NodesPlacementInfo string

const (
	// NodesPlacementInfoBestEffort the cluster nodes placement is in best effort,
	// it means you can have 2 masters (or more) on the same VM.
	NodesPlacementInfoBestEffort NodesPlacementInfo = "BestEffort"
	// NodesPlacementInfoOptimal the cluster nodes placement is optimal,
	// it means on master by VM
	NodesPlacementInfoOptimal NodesPlacementInfo = "Optimal"
)

type RestorePhase string

const (
	// RestorePhaseRunning used for Restore that are currently running.
	RestorePhaseRunning RestorePhase = "Running"
	// RestorePhaseRestart used for Restore that are restart master nodes.
	RestorePhaseRestart RestorePhase = "Restart"
	// RestorePhaseSucceeded used for Restore that are Succeeded.
	RestorePhaseSucceeded RestorePhase = "Succeeded"
)

const (
	DatabaseNamePrefix = "redis"

	GenericKey = "redis.kun"

	LabelClusterName = GenericKey + "/name"

	BackupKey         = ResourceSingularBackup + "." + GenericKey
	LabelBackupStatus = BackupKey + "/status"

	AnnotationJobType = GenericKey + "/job-type"

	JobTypeBackup  = "backup"
	JobTypeRestore = "restore"

	PrometheusExporterPortNumber    = 9100
	PrometheusExporterTelemetryPath = "/metrics"

	BackupDumpDir  = "/data"
	UtilVolumeName = "util-volume"
)
