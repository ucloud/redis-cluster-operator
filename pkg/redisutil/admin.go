package redisutil

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"time"
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
	//// InitRedisCluster used to configure the first node of a cluster
	//InitRedisCluster(addr string) error
	//// GetClusterInfosSelected return the Nodes infos for all nodes selected in the cluster
	//GetClusterInfosSelected(addrs []string) (*ClusterInfos, error)
	// AttachNodeToCluster command use to connect a Node to the cluster
	// the connection will be done on a random node part of the connection pool
	AttachNodeToCluster() error
	// AttachSlaveToMaster attach a slave to a master node
	AttachSlaveToMaster(slave *Node, masterID string) error
	// DetachSlave dettach a slave to its master
	//DetachSlave(slave *Node) error
	//// StartFailover execute the failover of the Redis Master corresponding to the addr
	//StartFailover(addr string) error
	//// ForgetNode execute the Redis command to force the cluster to forgot the the Node
	//ForgetNode(id string) error
	//// ForgetNodeByAddr execute the Redis command to force the cluster to forgot the the Node
	//ForgetNodeByAddr(id string) error
	//// SetSlots exect the redis command to set slots in a pipeline, provide
	//// and empty nodeID if the set slots commands doesn't take a nodeID in parameter
	//SetSlots(addr string, action string, slots []Slot, nodeID string) error
	// AddSlots exect the redis command to add slots in a pipeline
	AddSlots(addr string, slots []Slot) error
	//// DelSlots exec the redis command to del slots in a pipeline
	//DelSlots(addr string, slots []Slot) error
	//// GetKeysInSlot exec the redis command to get the keys in the given slot on the node we are connected to
	//GetKeysInSlot(addr string, slot Slot, batch int, limit bool) ([]string, error)
	//// CountKeysInSlot exec the redis command to count the keys given slot on the node
	//CountKeysInSlot(addr string, slot Slot) (int64, error)
	//// MigrateKeys from addr to destination node. returns number of slot migrated. If replace is true, replace key on busy error
	//MigrateKeys(addr string, dest *Node, slots []Slot, batch, timeout int, replace bool) (int, error)
	//// FlushAndReset reset the cluster configuration of the node, the node is flushed in the same pipe to ensure reset works
	//FlushAndReset(addr string, mode string) error
	//// FlushAll flush all keys in cluster
	//FlushAll()
	//// GetHashMaxSlot get the max slot value
	//GetHashMaxSlot() Slot
	////RebuildConnectionMap rebuild the connection map according to the given addresses
	//RebuildConnectionMap(addrs []string, options *AdminOptions)
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
}

// NewAdmin returns new AdminInterface instance
// at the same time it connects to all Redis Nodes thanks to the addrs list
func NewAdmin(addrs []string, options *AdminOptions) IAdmin {
	a := &Admin{
		hashMaxSlots: DefaultHashMaxSlots,
	}

	// perform initial connections
	a.cnx = NewAdminConnections(addrs, options)

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
			infos.Status = ClusterInfosPartial
			clusterErr.partial = true
			clusterErr.errs[addr] = err
			continue
		}
		if nodeinfos.Node != nil && nodeinfos.Node.IPPort() == addr {
			infos.Infos[addr] = nodeinfos
		} else {
			log.Info("bad node info retrieved from ", addr)
		}
	}

	if len(clusterErr.errs) == 0 {
		clusterErr.inconsistent = !infos.ComputeStatus()
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

	nodeInfos := DecodeNodeInfos(&raw, addr)

	//if log.V(3) {
	//	//Retrieve server info for debugging
	//	resp = c.Cmd("INFO", "SERVER")
	//	if err = a.Connections().ValidateResp(resp, addr, "unable to retrieve Node Info"); err != nil {
	//		return nil, err
	//	}
	//	raw, err = resp.Str()
	//	if err != nil {
	//		return nil, fmt.Errorf("wrong format from INFO SERVER: %v", err)
	//	}
	//
	//	var serverStartTime time.Time
	//	serverStartTime, err = DecodeNodeStartTime(&raw)
	//
	//	if err != nil {
	//		return nil, err
	//	}
	//
	//	nodeInfos.Node.ServerStartTime = serverStartTime
	//}

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
func (a *Admin) AttachNodeToCluster() error {
	all := a.Connections().GetAll()
	if len(all) == 0 {
		return fmt.Errorf("no connection for other redis-node found")
	}
	ip, port, addr := "", "", ""
	var err error
	for cAddr, c := range a.Connections().GetAll() {
		if ip == "" {
			ip, port, err = net.SplitHostPort(cAddr)
			if err != nil {
				return err
			}
			addr = cAddr
			continue
		}
		resp := c.Cmd("CLUSTER", "MEET", ip, port)
		if err = a.Connections().ValidateResp(resp, addr, "cannot attach node to cluster"); err != nil {
			return err
		}
	}

	a.Connections().Add(addr)

	log.Info(fmt.Sprintf("node %s attached properly", addr))
	return nil
}

func (a *Admin) getAllConfig(c IClient, addr string) (map[string]string, error) {
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

// SetConfigIfNeed set redis config
func (a *Admin) SetConfigIfNeed(newConfig map[string]string) error {
	for addr, c := range a.Connections().GetAll() {
		oldConfig, err := a.getAllConfig(c, addr)
		if err != nil {
			return err
		}

		for key, value := range newConfig {
			if value != oldConfig[key] {
				log.V(3).Info("CONFIG SET", key, value)
				resp := c.Cmd("CONFIG", "SET", key, value)
				if err := a.Connections().ValidateResp(resp, addr, "unable to retrieve config"); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
