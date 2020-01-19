package clustering

import (
	"fmt"

	"github.com/go-logr/logr"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
	"github.com/ucloud/redis-cluster-operator/pkg/redisutil"
	"github.com/ucloud/redis-cluster-operator/pkg/resources/statefulsets"
)

type Ctx struct {
	log               logr.Logger
	expectedMasterNum int
	clusterName       string
	cluster           *redisutil.Cluster
	nodes             map[string]redisutil.Nodes
	currentMasters    redisutil.Nodes
	newMastersBySts   map[string]*redisutil.Node
	slavesByMaster    map[string]redisutil.Nodes
	bestEffort        bool
}

func NewCtx(cluster *redisutil.Cluster, nodes redisutil.Nodes, masterNum int32, clusterName string, log logr.Logger) *Ctx {
	ctx := &Ctx{
		log:               log,
		expectedMasterNum: int(masterNum),
		clusterName:       clusterName,
		cluster:           cluster,
		slavesByMaster:    make(map[string]redisutil.Nodes),
		newMastersBySts:   make(map[string]*redisutil.Node),
	}
	ctx.nodes = ctx.sortRedisNodeByStatefulSet(nodes)
	return ctx
}

func (c *Ctx) sortRedisNodeByStatefulSet(nodes redisutil.Nodes) map[string]redisutil.Nodes {
	nodesByStatefulSet := make(map[string]redisutil.Nodes)

	for _, rNode := range nodes {
		cNode, err := c.cluster.GetNodeByID(rNode.ID)
		if err != nil {
			c.log.Error(err, "[sortRedisNodeByStatefulSet] unable fo found the Cluster.Node with redis", "ID", rNode.ID)
			continue // if not then next line with cNode.Pod will cause a panic since cNode is nil
		}
		ssName := unknownVMName
		if cNode.StatefulSet != "" {
			ssName = cNode.StatefulSet
		}
		if _, ok := nodesByStatefulSet[ssName]; !ok {
			nodesByStatefulSet[ssName] = redisutil.Nodes{}
		}
		nodesByStatefulSet[ssName] = append(nodesByStatefulSet[ssName], rNode)
		if (rNode.GetRole() == redisv1alpha1.RedisClusterNodeRoleMaster) && rNode.TotalSlots() > 0 {
			c.currentMasters = append(c.currentMasters, rNode)
		}
	}

	return nodesByStatefulSet
}

func (c *Ctx) DispatchMasters() error {
	for i := 0; i < c.expectedMasterNum; i++ {
		stsName := statefulsets.ClusterStatefulSetName(c.clusterName, i)
		nodes, ok := c.nodes[stsName]
		if !ok {
			return fmt.Errorf("missing statefulset %s", stsName)
		}
		currentMasterNodes := nodes.FilterByFunc(redisutil.IsMasterWithSlot)
		if len(currentMasterNodes) == 0 {
			master := c.PlaceMasters(stsName)
			c.newMastersBySts[stsName] = master
		} else if len(currentMasterNodes) == 1 {
			c.newMastersBySts[stsName] = currentMasterNodes[0]
		} else if len(currentMasterNodes) > 1 {
			c.log.Error(fmt.Errorf("split brain"), "fix manually", "statefulSet", stsName, "masters", currentMasterNodes)
			return fmt.Errorf("split brain: %s", stsName)
		}
	}

	return nil
}

func (c *Ctx) PlaceMasters(ssName string) *redisutil.Node {
	var allMasters redisutil.Nodes
	allMasters = append(allMasters, c.currentMasters...)
	for _, master := range c.newMastersBySts {
		allMasters = append(allMasters, master)
	}
	nodes := c.nodes[ssName]
	for _, cNode := range nodes {
		_, err := allMasters.GetNodesByFunc(func(node *redisutil.Node) bool {
			if node.NodeName == cNode.NodeName {
				return true
			}
			return false
		})
		if err != nil {
			return cNode
		}
	}
	c.bestEffort = true
	c.log.Info("the pod are not spread enough on VMs to have only one master by VM", "select", nodes[0].IP)
	return nodes[0]
}

func (c *Ctx) PlaceSlaves() error {
	c.bestEffort = true
	for ssName, nodes := range c.nodes {
		master := c.newMastersBySts[ssName]
		for _, node := range nodes {
			if node.IP == master.IP {
				continue
			}
			if node.NodeName != master.NodeName {
				c.bestEffort = false
			}
			if node.GetRole() == redisv1alpha1.RedisClusterNodeRoleSlave {
				if node.MasterReferent != master.ID {
					c.log.Error(nil, "master referent conflict", "node ip", node.IP,
						"current masterID", node.MasterReferent, "expect masterID", master.ID, "master IP", master.IP)
					c.slavesByMaster[master.ID] = append(c.slavesByMaster[master.ID], node)
				}
				continue
			}
			c.slavesByMaster[master.ID] = append(c.slavesByMaster[master.ID], node)
		}
	}
	return nil
}

func (c *Ctx) GetCurrentMasters() redisutil.Nodes {
	return c.currentMasters
}

func (c *Ctx) GetNewMasters() redisutil.Nodes {
	var nodes redisutil.Nodes
	for _, node := range c.newMastersBySts {
		nodes = append(nodes, node)
	}
	return nodes
}

func (c *Ctx) GetSlaves() map[string]redisutil.Nodes {
	return c.slavesByMaster
}

func (c *Ctx) GetStatefulsetNodes() map[string]redisutil.Nodes {
	return c.nodes
}
