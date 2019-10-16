package clustering

import (
	"fmt"

	"github.com/ucloud/redis-cluster-operator/pkg/redisutil"
)

// AttachingSlavesToMaster used to attach slaves to there masters
func AttachingSlavesToMaster(cluster *redisutil.Cluster, admin redisutil.IAdmin, slavesByMaster map[string]redisutil.Nodes) error {
	var globalErr error
	for masterID, slaves := range slavesByMaster {
		masterNode, err := cluster.GetNodeByID(masterID)
		if err != nil {
			log.Error(err, fmt.Sprintf("unable fo found the Cluster.Node with redis ID:%s", masterID))
			continue
		}
		for _, slave := range slaves {
			log.V(2).Info(fmt.Sprintf("attaching node %s to master %s", slave.ID, masterID))

			err := admin.AttachSlaveToMaster(slave, masterNode.ID)
			if err != nil {
				log.Error(err, fmt.Sprintf("attaching node %s to master %s", slave.ID, masterID))
				globalErr = err
			}
		}
	}
	return globalErr
}
