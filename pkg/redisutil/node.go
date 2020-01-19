package redisutil

import (
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
	"github.com/ucloud/redis-cluster-operator/pkg/utils"
)

const (
	// DefaultRedisPort define the default Redis Port
	DefaultRedisPort = "6379"
	// RedisMasterRole redis role master
	RedisMasterRole = "master"
	// RedisSlaveRole redis role slave
	RedisSlaveRole = "slave"
)

const (
	// RedisLinkStateConnected redis connection status connected
	RedisLinkStateConnected = "connected"
	// RedisLinkStateDisconnected redis connection status disconnected
	RedisLinkStateDisconnected = "disconnected"
)

const (
	// NodeStatusPFail Node is in PFAIL state. Not reachable for the node you are contacting, but still logically reachable
	NodeStatusPFail = "fail?"
	// NodeStatusFail Node is in FAIL state. It was not reachable for multiple nodes that promoted the PFAIL state to FAIL
	NodeStatusFail = "fail"
	// NodeStatusHandshake Untrusted node, we are handshaking.
	NodeStatusHandshake = "handshake"
	// NodeStatusNoAddr No address known for this node
	NodeStatusNoAddr = "noaddr"
	// NodeStatusNoFlags no flags at all
	NodeStatusNoFlags = "noflags"
)

// Node Represent a Redis Node
type Node struct {
	ID              string
	IP              string
	Port            string
	Role            string
	LinkState       string
	MasterReferent  string
	FailStatus      []string
	PingSent        int64
	PongRecv        int64
	ConfigEpoch     int64
	Slots           []Slot
	balance         int
	MigratingSlots  map[Slot]string
	ImportingSlots  map[Slot]string
	ServerStartTime time.Time

	NodeName    string
	PodName     string
	StatefulSet string
}

// Nodes represent a Node slice
type Nodes []*Node

func (n Nodes) String() string {
	stringer := []utils.Stringer{}
	for _, node := range n {
		stringer = append(stringer, node)
	}

	return utils.SliceJoin(stringer, ",")
}

// NewDefaultNode builds and returns new defaultNode instance
func NewDefaultNode() *Node {
	return &Node{
		Port:           DefaultRedisPort,
		Slots:          []Slot{},
		MigratingSlots: map[Slot]string{},
		ImportingSlots: map[Slot]string{},
	}
}

// NewNode builds and returns new Node instance
func NewNode(id, ip string, pod *corev1.Pod) *Node {
	node := NewDefaultNode()
	node.ID = id
	node.IP = ip
	node.PodName = pod.Name
	node.NodeName = pod.Spec.NodeName

	return node
}

// SetRole from a flags string list set the Node's role
func (n *Node) SetRole(flags string) error {
	n.Role = "" // reset value before setting the new one
	vals := strings.Split(flags, ",")
	for _, val := range vals {
		switch val {
		case RedisMasterRole:
			n.Role = RedisMasterRole
		case RedisSlaveRole:
			n.Role = RedisSlaveRole
		}
	}

	if n.Role == "" {
		return errors.New("node setRole failed")
	}

	return nil
}

// GetRole return the Redis Cluster Node GetRole
func (n *Node) GetRole() redisv1alpha1.RedisRole {
	switch n.Role {
	case RedisMasterRole:
		return redisv1alpha1.RedisClusterNodeRoleMaster
	case RedisSlaveRole:
		return redisv1alpha1.RedisClusterNodeRoleSlave
	default:
		if n.MasterReferent != "" {
			return redisv1alpha1.RedisClusterNodeRoleSlave
		}
		if len(n.Slots) > 0 {
			return redisv1alpha1.RedisClusterNodeRoleMaster
		}
	}

	return redisv1alpha1.RedisClusterNodeRoleNone
}

// String string representation of a Instance
func (n *Node) String() string {
	if n.ServerStartTime.IsZero() {
		return fmt.Sprintf("{Redis ID: %s, role: %s, master: %s, link: %s, status: %s, addr: %s, slots: %s, len(migratingSlots): %d, len(importingSlots): %d}", n.ID, n.GetRole(), n.MasterReferent, n.LinkState, n.FailStatus, n.IPPort(), SlotSlice(n.Slots), len(n.MigratingSlots), len(n.ImportingSlots))
	}
	return fmt.Sprintf("{Redis ID: %s, role: %s, master: %s, link: %s, status: %s, addr: %s, slots: %s, len(migratingSlots): %d, len(importingSlots): %d, ServerStartTime: %s}", n.ID, n.GetRole(), n.MasterReferent, n.LinkState, n.FailStatus, n.IPPort(), SlotSlice(n.Slots), len(n.MigratingSlots), len(n.ImportingSlots), n.ServerStartTime.Format("2006-01-02 15:04:05"))
}

// IPPort returns join Ip Port string
func (n *Node) IPPort() string {
	return net.JoinHostPort(n.IP, n.Port)
}

// GetNodesByFunc returns first node found by the FindNodeFunc
func (n Nodes) GetNodesByFunc(f FindNodeFunc) (Nodes, error) {
	nodes := Nodes{}
	for _, node := range n {
		if f(node) {
			nodes = append(nodes, node)
		}
	}
	if len(nodes) == 0 {
		return nodes, nodeNotFoundedError
	}
	return nodes, nil
}

// ToAPINode used to convert the current Node to an API redisv1alpha1.RedisClusterNode
func (n *Node) ToAPINode() redisv1alpha1.RedisClusterNode {
	apiNode := redisv1alpha1.RedisClusterNode{
		ID:      n.ID,
		IP:      n.IP,
		PodName: n.PodName,
		Role:    n.GetRole(),
		Slots:   []string{},
	}

	return apiNode
}

// Clear used to clear possible ressources attach to the current Node
func (n *Node) Clear() {

}

func (n *Node) Balance() int {
	return n.balance
}

func (n *Node) SetBalance(balance int) {
	n.balance = balance
}

// SetLinkStatus set the Node link status
func (n *Node) SetLinkStatus(status string) error {
	n.LinkState = "" // reset value before setting the new one
	switch status {
	case RedisLinkStateConnected:
		n.LinkState = RedisLinkStateConnected
	case RedisLinkStateDisconnected:
		n.LinkState = RedisLinkStateDisconnected
	}

	if n.LinkState == "" {
		return errors.New("Node SetLinkStatus failed")
	}

	return nil
}

// SetFailureStatus set from inputs flags the possible failure status
func (n *Node) SetFailureStatus(flags string) {
	n.FailStatus = []string{} // reset value before setting the new one
	vals := strings.Split(flags, ",")
	for _, val := range vals {
		switch val {
		case NodeStatusFail:
			n.FailStatus = append(n.FailStatus, NodeStatusFail)
		case NodeStatusPFail:
			n.FailStatus = append(n.FailStatus, NodeStatusPFail)
		case NodeStatusHandshake:
			n.FailStatus = append(n.FailStatus, NodeStatusHandshake)
		case NodeStatusNoAddr:
			n.FailStatus = append(n.FailStatus, NodeStatusNoAddr)
		case NodeStatusNoFlags:
			n.FailStatus = append(n.FailStatus, NodeStatusNoFlags)
		}
	}
}

// SetReferentMaster set the redis node parent referent
func (n *Node) SetReferentMaster(ref string) {
	n.MasterReferent = ""
	if ref == "-" {
		return
	}
	n.MasterReferent = ref
}

// TotalSlots return the total number of slot
func (n *Node) TotalSlots() int {
	return len(n.Slots)
}

// HasStatus returns true if the node has the provided fail status flag
func (n *Node) HasStatus(flag string) bool {
	for _, status := range n.FailStatus {
		if status == flag {
			return true
		}
	}
	return false
}

// IsMasterWithNoSlot anonymous function for searching Master Node with no slot
var IsMasterWithNoSlot = func(n *Node) bool {
	if (n.GetRole() == redisv1alpha1.RedisClusterNodeRoleMaster) && (n.TotalSlots() == 0) {
		return true
	}
	return false
}

// IsMasterWithSlot anonymous function for searching Master Node withslot
var IsMasterWithSlot = func(n *Node) bool {
	if (n.GetRole() == redisv1alpha1.RedisClusterNodeRoleMaster) && (n.TotalSlots() > 0) {
		return true
	}
	return false
}

// IsSlave anonymous function for searching Slave Node
var IsSlave = func(n *Node) bool {
	return n.GetRole() == redisv1alpha1.RedisClusterNodeRoleSlave
}

// SortNodes sort Nodes and return the sorted Nodes
func (n Nodes) SortNodes() Nodes {
	sort.Sort(n)
	return n
}

// GetNodeByID returns a Redis Node by its ID
// if not present in the Nodes slice return an error
func (n Nodes) GetNodeByID(id string) (*Node, error) {
	for _, node := range n {
		if node.ID == id {
			return node, nil
		}
	}

	return nil, nodeNotFoundedError
}

// CountByFunc gives the number elements of NodeSlice that return true for the passed func.
func (n Nodes) CountByFunc(fn func(*Node) bool) (result int) {
	for _, v := range n {
		if fn(v) {
			result++
		}
	}
	return
}

// FilterByFunc remove a node from a slice by node ID and returns the slice. If not found, fail silently. Value must be unique
func (n Nodes) FilterByFunc(fn func(*Node) bool) Nodes {
	newSlice := Nodes{}
	for _, node := range n {
		if fn(node) {
			newSlice = append(newSlice, node)
		}
	}
	return newSlice
}

// SortByFunc returns a new ordered NodeSlice, determined by a func defining ‘less’.
func (n Nodes) SortByFunc(less func(*Node, *Node) bool) Nodes {
	//result := make(Nodes, len(n))
	//copy(result, n)
	by(less).Sort(n)
	return n
}

// Len is the number of elements in the collection.
func (n Nodes) Len() int {
	return len(n)
}

// Less reports whether the element with
// index i should sort before the element with index j.
func (n Nodes) Less(i, j int) bool {
	return n[i].ID < n[j].ID
}

// Swap swaps the elements with indexes i and j.
func (n Nodes) Swap(i, j int) {
	n[i], n[j] = n[j], n[i]
}

// By is the type of a "less" function that defines the ordering of its Node arguments.
type by func(p1, p2 *Node) bool

// Sort is a method on the function type, By, that sorts the argument slice according to the function.
func (b by) Sort(nodes Nodes) {
	ps := &nodeSorter{
		nodes: nodes,
		by:    b, // The Sort method's receiver is the function (closure) that defines the sort order.
	}
	sort.Sort(ps)
}

// nodeSorter joins a By function and a slice of Nodes to be sorted.
type nodeSorter struct {
	nodes Nodes
	by    func(p1, p2 *Node) bool // Closure used in the Less method.
}

// Len is part of sort.Interface.
func (s *nodeSorter) Len() int {
	return len(s.nodes)
}

// Swap is part of sort.Interface.
func (s *nodeSorter) Swap(i, j int) {
	s.nodes[i], s.nodes[j] = s.nodes[j], s.nodes[i]
}

// Less is part of sort.Interface. It is implemented by calling the "by" closure in the sorter.
func (s *nodeSorter) Less(i, j int) bool {
	return s.by(s.nodes[i], s.nodes[j])
}

// LessByID compare 2 Nodes with there ID
func LessByID(n1, n2 *Node) bool {
	return n1.ID < n2.ID
}

// MoreByID compare 2 Nodes with there ID
func MoreByID(n1, n2 *Node) bool {
	return n1.ID > n2.ID
}
