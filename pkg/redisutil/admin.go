package redisutil

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"time"

	"github.com/go-logr/logr"

	"github.com/ucloud/redis-cluster-operator/pkg/utils"
)

const (
	// defaultHashMaxSlots higher value of slot
	// as slots start at 0, total number of slots is defaultHashMaxSlots+1
	DefaultHashMaxSlots = 16383

	// ResetHard HARD mode for RESET command
	ResetHard = "HARD"
	// ResetSoft SOFT mode for RESET command
	ResetSoft = "SOFT"
)

const (
	clusterKnownNodesREString = "cluster_known_nodes:([0-9]+)"
)

var (
	clusterKnownNodesRE = regexp.MustCompile(clusterKnownNodesREString)
)

// IAdmin redis cluster admin interface
type IAdmin interface {
	// Connections returns the connection map of all clients
	Connections() IAdminConnections
	// Close the admin connections
	Close()
	// GetClusterInfos get node infos for all nodes
	GetClusterInfos() (*ClusterInfos, error)
	// ClusterManagerNodeIsEmpty Checks whether the node is empty. Node is considered not-empty if it has
	// some key or if it already knows other nodes
	ClusterManagerNodeIsEmpty() (bool, error)
	// SetConfigEpoch Assign a different config epoch to each node
	SetConfigEpoch() error
	// SetConfigIfNeed set redis config
	SetConfigIfNeed(newConfig map[string]string) error
	// GetAllConfig get redis config by CONFIG GET *
	GetAllConfig(c IClient, addr string) (map[string]string, error)
	// AttachNodeToCluster command use to connect a Node to the cluster
	// the connection will be done on a random node part of the connection pool
	AttachNodeToCluster(addr string) error
	// AttachSlaveToMaster attach a slave to a master node
	AttachSlaveToMaster(slave *Node, masterID string) error
	// DetachSlave dettach a slave to its master
	DetachSlave(slave *Node) error
	// ForgetNode execute the Redis command to force the cluster to forgot the the Node
	ForgetNode(id string) error
	// SetSlots exec the redis command to set slots in a pipeline, provide
	// and empty nodeID if the set slots commands doesn't take a nodeID in parameter
	SetSlots(addr string, action string, slots []Slot, nodeID string) error
	// AddSlots exec the redis command to add slots in a pipeline
	AddSlots(addr string, slots []Slot) error
	// SetSlot use to set SETSLOT command on a slot
	SetSlot(addr, action string, slot Slot, nodeID string) error
	// MigrateKeys from addr to destination node. returns number of slot migrated. If replace is true, replace key on busy error
	MigrateKeys(addr string, dest *Node, slots []Slot, batch, timeout int, replace bool) (int, error)
	// MigrateKeys use to migrate keys from slot to other slot. if replace is true, replace key on busy error
	// timeout is in milliseconds
	MigrateKeysInSlot(addr string, dest *Node, slot Slot, batch int, timeout int, replace bool) (int, error)
	// FlushAndReset reset the cluster configuration of the node, the node is flushed in the same pipe to ensure reset works
	FlushAndReset(addr string, mode string) error
	// GetHashMaxSlot get the max slot value
	GetHashMaxSlot() Slot
	// ResetPassword reset redis node masterauth and requirepass.
	ResetPassword(newPassword string) error
}

// AdminOptions optional options for redis admin
type AdminOptions struct {
	ConnectionTimeout  time.Duration
	ClientName         string
	RenameCommandsFile string
	Password           string
}

// Admin wraps redis cluster admin logic
type Admin struct {
	hashMaxSlots Slot
	cnx          IAdminConnections
	log          logr.Logger
}

// NewAdmin returns new AdminInterface instance
// at the same time it connects to all Redis Nodes thanks to the addrs list
func NewAdmin(addrs []string, options *AdminOptions, log logr.Logger) IAdmin {
	a := &Admin{
		hashMaxSlots: DefaultHashMaxSlots,
		log:          log.WithName("redis_util"),
	}

	// perform initial connections
	a.cnx = NewAdminConnections(addrs, options, log)

	return a
}

// Connections returns the connection map of all clients
func (a *Admin) Connections() IAdminConnections {
	return a.cnx
}

// Close used to close all possible resources instanciate by the Admin
func (a *Admin) Close() {
	a.Connections().Reset()
}

// GetClusterInfos return the Nodes infos for all nodes
func (a *Admin) GetClusterInfos() (*ClusterInfos, error) {
	infos := NewClusterInfos()
	clusterErr := NewClusterInfosError()

	for addr, c := range a.Connections().GetAll() {
		nodeinfos, err := a.getInfos(c, addr)
		if err != nil {
			a.log.WithValues("err", err).Info("get redis info failed")
			infos.Status = ClusterInfosPartial
			clusterErr.partial = true
			clusterErr.errs[addr] = err
			continue
		}
		if nodeinfos.Node != nil && nodeinfos.Node.IPPort() == addr {
			infos.Infos[addr] = nodeinfos
		} else {
			a.log.Info("bad node info retrieved from", "addr", addr)
		}
	}

	if len(clusterErr.errs) == 0 {
		clusterErr.inconsistent = !infos.ComputeStatus(a.log)
	}
	if infos.Status == ClusterInfosConsistent {
		return infos, nil
	}
	return infos, clusterErr
}

func (a *Admin) getInfos(c IClient, addr string) (*NodeInfos, error) {
	resp := c.Cmd("CLUSTER", "NODES")
	if err := a.Connections().ValidateResp(resp, addr, "unable to retrieve node info"); err != nil {
		return nil, err
	}

	var raw string
	var err error
	raw, err = resp.Str()

	if err != nil {
		return nil, fmt.Errorf("wrong format from CLUSTER NODES: %v", err)
	}

	nodeInfos := DecodeNodeInfos(&raw, addr, a.log)

	return nodeInfos, nil
}

// ClusterManagerNodeIsEmpty Checks whether the node is empty. Node is considered not-empty if it has
// some key or if it already knows other nodes
func (a *Admin) ClusterManagerNodeIsEmpty() (bool, error) {
	for addr, c := range a.Connections().GetAll() {
		knowNodes, err := a.clusterKnowNodes(c, addr)
		if err != nil {
			return false, err
		}
		if knowNodes != 1 {
			return false, nil
		}
	}
	return true, nil
}

func (a *Admin) clusterKnowNodes(c IClient, addr string) (int, error) {
	resp := c.Cmd("CLUSTER", "INFO")
	if err := a.Connections().ValidateResp(resp, addr, "unable to retrieve cluster info"); err != nil {
		return 0, err
	}

	var raw string
	var err error
	raw, err = resp.Str()

	if err != nil {
		return 0, fmt.Errorf("wrong format from CLUSTER INFO: %v", err)
	}

	match := clusterKnownNodesRE.FindStringSubmatch(raw)
	if len(match) == 0 {
		return 0, fmt.Errorf("cluster_known_nodes regex not found")
	}
	return strconv.Atoi(match[1])
}

// AttachSlaveToMaster attach a slave to a master node
func (a *Admin) AttachSlaveToMaster(slave *Node, masterID string) error {
	c, err := a.Connections().Get(slave.IPPort())
	if err != nil {
		return err
	}

	resp := c.Cmd("CLUSTER", "REPLICATE", masterID)
	if err := a.Connections().ValidateResp(resp, slave.IPPort(), "unable to run command REPLICATE"); err != nil {
		return err
	}

	slave.SetReferentMaster(masterID)
	slave.SetRole(RedisSlaveRole)

	return nil
}

// AddSlots use to ADDSLOT commands on several slots
func (a *Admin) AddSlots(addr string, slots []Slot) error {
	if len(slots) == 0 {
		return nil
	}
	c, err := a.Connections().Get(addr)
	if err != nil {
		return err
	}

	resp := c.Cmd("CLUSTER", "ADDSLOTS", slots)

	return a.Connections().ValidateResp(resp, addr, "unable to run CLUSTER ADDSLOTS")
}

// SetSlots use to set SETSLOT command on several slots
func (a *Admin) SetSlots(addr, action string, slots []Slot, nodeID string) error {
	if len(slots) == 0 {
		return nil
	}
	c, err := a.Connections().Get(addr)
	if err != nil {
		return err
	}
	for _, slot := range slots {
		if nodeID == "" {
			c.PipeAppend("CLUSTER", "SETSLOT", slot, action)
		} else {
			c.PipeAppend("CLUSTER", "SETSLOT", slot, action, nodeID)
		}
	}
	if !a.Connections().ValidatePipeResp(c, addr, "Cannot SETSLOT") {
		return fmt.Errorf("Error occured during CLUSTER SETSLOT %s", action)
	}
	c.PipeClear()

	return nil
}

// SetSlot use to set SETSLOT command on a slot
func (a *Admin) SetSlot(addr, action string, slot Slot, nodeID string) error {
	c, err := a.Connections().Get(addr)
	if err != nil {
		return err
	}
	if nodeID == "" {
		c.PipeAppend("CLUSTER", "SETSLOT", slot, action)
	} else {
		c.PipeAppend("CLUSTER", "SETSLOT", slot, action, nodeID)
	}
	if !a.Connections().ValidatePipeResp(c, addr, "Cannot SETSLOT") {
		return fmt.Errorf("Error occured during CLUSTER SETSLOT %s", action)
	}
	c.PipeClear()

	return nil
}

func (a *Admin) SetConfigEpoch() error {
	configEpoch := 1
	for addr, c := range a.Connections().GetAll() {
		resp := c.Cmd("CLUSTER", "SET-CONFIG-EPOCH", configEpoch)
		if err := a.Connections().ValidateResp(resp, addr, "unable to run command SET-CONFIG-EPOCH"); err != nil {
			return err
		}
		configEpoch++
	}
	return nil
}

// AttachNodeToCluster command use to connect a Node to the cluster
func (a *Admin) AttachNodeToCluster(addr string) error {
	ip, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}

	all := a.Connections().GetAll()
	if len(all) == 0 {
		return fmt.Errorf("no connection for other redis-node found")
	}
	for cAddr, c := range a.Connections().GetAll() {
		if cAddr == addr {
			continue
		}
		a.log.V(3).Info("CLUSTER MEET", "from addr", cAddr, "to", addr)
		resp := c.Cmd("CLUSTER", "MEET", ip, port)
		if err = a.Connections().ValidateResp(resp, addr, "cannot attach node to cluster"); err != nil {
			return err
		}
	}

	a.Connections().Add(addr)

	a.log.Info(fmt.Sprintf("node %s attached properly", addr))
	return nil
}

// GetAllConfig get redis config by CONFIG GET *
func (a *Admin) GetAllConfig(c IClient, addr string) (map[string]string, error) {
	resp := c.Cmd("CONFIG", "GET", "*")
	if err := a.Connections().ValidateResp(resp, addr, "unable to retrieve config"); err != nil {
		return nil, err
	}

	var raw map[string]string
	var err error
	raw, err = resp.Map()

	if err != nil {
		return nil, fmt.Errorf("wrong format from CONFIG GET *: %v", err)
	}

	return raw, nil
}

var parseConfigMap = map[string]int8{
	"maxmemory":                  0,
	"proto-max-bulk-len":         0,
	"client-query-buffer-limit":  0,
	"repl-backlog-size":          0,
	"auto-aof-rewrite-min-size":  0,
	"active-defrag-ignore-bytes": 0,
	"hash-max-ziplist-entries":   0,
	"hash-max-ziplist-value":     0,
	"stream-node-max-bytes":      0,
	"set-max-intset-entries":     0,
	"zset-max-ziplist-entries":   0,
	"zset-max-ziplist-value":     0,
	"hll-sparse-max-bytes":       0,
	// TODO parse client-output-buffer-limit
	//"client-output-buffer-limit": 0,
}

// SetConfigIfNeed set redis config
func (a *Admin) SetConfigIfNeed(newConfig map[string]string) error {
	for addr, c := range a.Connections().GetAll() {
		oldConfig, err := a.GetAllConfig(c, addr)
		if err != nil {
			return err
		}

		for key, value := range newConfig {
			var err error
			if _, ok := parseConfigMap[key]; ok {
				value, err = utils.ParseRedisMemConf(value)
				if err != nil {
					a.log.Error(err, "redis config format err", "key", key, "value", value)
					continue
				}
			}
			if value != oldConfig[key] {
				a.log.V(3).Info("CONFIG SET", key, value)
				resp := c.Cmd("CONFIG", "SET", key, value)
				if err := a.Connections().ValidateResp(resp, addr, "unable to retrieve config"); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// GetHashMaxSlot get the max slot value
func (a *Admin) GetHashMaxSlot() Slot {
	return a.hashMaxSlots
}

// MigrateKeys use to migrate keys from slots to other slots. if replace is true, replace key on busy error
// timeout is in milliseconds
func (a *Admin) MigrateKeys(addr string, dest *Node, slots []Slot, batch int, timeout int, replace bool) (int, error) {
	if len(slots) == 0 {
		return 0, nil
	}
	keyCount := 0
	c, err := a.Connections().Get(addr)
	if err != nil {
		return keyCount, err
	}
	timeoutStr := strconv.Itoa(timeout)
	batchStr := strconv.Itoa(batch)

	for _, slot := range slots {
		for {
			resp := c.Cmd("CLUSTER", "GETKEYSINSLOT", slot, batchStr)
			if err := a.Connections().ValidateResp(resp, addr, "Unable to run command GETKEYSINSLOT"); err != nil {
				return keyCount, err
			}
			keys, err := resp.List()
			if err != nil {
				a.log.Error(err, "wrong returned format for CLUSTER GETKEYSINSLOT")
				return keyCount, err
			}

			keyCount += len(keys)
			if len(keys) == 0 {
				break
			}

			args := a.migrateCmdArgs(dest, timeoutStr, replace, keys)

			resp = c.Cmd("MIGRATE", args)
			if err := a.Connections().ValidateResp(resp, addr, "Unable to run command MIGRATE"); err != nil {
				return keyCount, err
			}
		}
	}

	return keyCount, nil
}

// MigrateKeys use to migrate keys from slot to other slot. if replace is true, replace key on busy error
// timeout is in milliseconds
func (a *Admin) MigrateKeysInSlot(addr string, dest *Node, slot Slot, batch int, timeout int, replace bool) (int, error) {
	keyCount := 0
	c, err := a.Connections().Get(addr)
	if err != nil {
		return keyCount, err
	}
	timeoutStr := strconv.Itoa(timeout)
	batchStr := strconv.Itoa(batch)

	for {
		resp := c.Cmd("CLUSTER", "GETKEYSINSLOT", slot, batchStr)
		if err := a.Connections().ValidateResp(resp, addr, "Unable to run command GETKEYSINSLOT"); err != nil {
			return keyCount, err
		}
		keys, err := resp.List()
		if err != nil {
			a.log.Error(err, "wrong returned format for CLUSTER GETKEYSINSLOT")
			return keyCount, err
		}

		keyCount += len(keys)
		if len(keys) == 0 {
			break
		}

		args := a.migrateCmdArgs(dest, timeoutStr, replace, keys)

		resp = c.Cmd("MIGRATE", args)
		if err := a.Connections().ValidateResp(resp, addr, "Unable to run command MIGRATE"); err != nil {
			return keyCount, err
		}
	}

	return keyCount, nil
}

func (a *Admin) migrateCmdArgs(dest *Node, timeoutStr string, replace bool, keys []string) []string {
	args := []string{dest.IP, dest.Port, "", "0", timeoutStr}
	if password, ok := a.Connections().GetAUTH(); ok {
		args = append(args, "AUTH", password)
	}
	if replace {
		args = append(args, "REPLACE", "KEYS")
	} else {
		args = append(args, "KEYS")
	}
	args = append(args, keys...)
	return args
}

// ForgetNode used to force other redis cluster node to forget a specific node
func (a *Admin) ForgetNode(id string) error {
	infos, _ := a.GetClusterInfos()
	for nodeAddr, nodeinfos := range infos.Infos {
		if nodeinfos.Node.ID == id {
			continue
		}

		if IsSlave(nodeinfos.Node) && nodeinfos.Node.MasterReferent == id {
			if err := a.DetachSlave(nodeinfos.Node); err != nil {
				a.log.Error(err, "DetachSlave", "node", nodeAddr)
			}
			a.log.Info(fmt.Sprintf("detach slave id: %s of master: %s", nodeinfos.Node.ID, id))
		}

		c, err := a.Connections().Get(nodeAddr)
		if err != nil {
			a.log.Error(err, fmt.Sprintf("cannot force a forget on node %s, for node %s", nodeAddr, id))
			continue
		}

		a.log.Info("CLUSTER FORGET", "id", id, "from", nodeAddr)
		resp := c.Cmd("CLUSTER", "FORGET", id)
		a.Connections().ValidateResp(resp, nodeAddr, "Unable to execute FORGET command")
	}

	a.log.Info("Forget node done", "node", id)
	return nil
}

// DetachSlave use to detach a slave to a master
func (a *Admin) DetachSlave(slave *Node) error {
	c, err := a.Connections().Get(slave.IPPort())
	if err != nil {
		a.log.Error(err, fmt.Sprintf("unable to get the connection for slave ID:%s, addr:%s", slave.ID, slave.IPPort()))
		return err
	}

	resp := c.Cmd("CLUSTER", "RESET", "SOFT")
	if err = a.Connections().ValidateResp(resp, slave.IPPort(), "cannot attach node to cluster"); err != nil {
		return err
	}

	if err = a.AttachNodeToCluster(slave.IPPort()); err != nil {
		a.log.Error(err, fmt.Sprintf("[DetachSlave] unable to AttachNodeToCluster the Slave id: %s addr:%s", slave.ID, slave.IPPort()))
		return err
	}

	slave.SetReferentMaster("")
	slave.SetRole(RedisMasterRole)

	return nil
}

// FlushAndReset flush the cluster and reset the cluster configuration of the node. Commands are piped, to ensure no items arrived between flush and reset
func (a *Admin) FlushAndReset(addr string, mode string) error {
	c, err := a.Connections().Get(addr)
	if err != nil {
		return err
	}
	c.PipeAppend("FLUSHALL")
	c.PipeAppend("CLUSTER", "RESET", mode)

	if !a.Connections().ValidatePipeResp(c, addr, "Cannot reset node") {
		return fmt.Errorf("Cannot reset node %s", addr)
	}

	return nil
}

// ResetPassword reset redis node masterauth and requirepass.
func (a *Admin) ResetPassword(newPassword string) error {
	all := a.Connections().GetAll()
	if len(all) == 0 {
		return fmt.Errorf("no connection for other redis-node found")
	}
	for addr, c := range a.Connections().GetAll() {
		a.log.Info("reset password", "addr", addr)
		setMasterauth := c.Cmd("CONFIG", "SET", "masterauth", newPassword)
		if err := a.Connections().ValidateResp(setMasterauth, addr, "cannot set new masterauth"); err != nil {
			return err
		}
		setPasswdResp := c.Cmd("CONFIG", "SET", "requirepass", newPassword)
		if err := a.Connections().ValidateResp(setPasswdResp, addr, "cannot set new requirepass"); err != nil {
			return err
		}
	}
	return nil
}
