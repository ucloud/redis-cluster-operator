package configmaps

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
)

const (
	RestoreSucceeded = "succeeded"

	RedisConfKey = "redis.conf"
)

// NewConfigMapForCR creates a new ConfigMap for the given Cluster
func NewConfigMapForCR(cluster *redisv1alpha1.DistributedRedisCluster, labels map[string]string) *corev1.ConfigMap {
	// Do CLUSTER FAILOVER when master down
	shutdownContent := `#!/bin/sh
CLUSTER_CONFIG="/data/nodes.conf"
failover() {
	echo "Do CLUSTER FAILOVER"
	masterID=$(cat ${CLUSTER_CONFIG} | grep "myself" | awk '{print $1}')
	echo "Master: ${masterID}"
	slave=$(cat ${CLUSTER_CONFIG} | grep ${masterID} | grep "slave" | awk 'NR==1{print $2}' | sed 's/:6379@16379//')
	echo "Slave: ${slave}"
	password=$(cat /data/redis_password)
	if [[ -z "${password}" ]]; then
		redis-cli -h ${slave} CLUSTER FAILOVER
	else
		redis-cli -h ${slave} -a "${password}" CLUSTER FAILOVER
	fi
	echo "Wait for MASTER <-> SLAVE syncFinished"
	sleep 20
}
if [ -f ${CLUSTER_CONFIG} ]; then
	cat ${CLUSTER_CONFIG} | grep "myself" | grep "master" && \
	failover
fi`

	// Fixed Nodes.conf does not update IP address of a node when IP changes after restart,
	// see more https://github.com/antirez/redis/issues/4645.
	fixIPContent := `#!/bin/sh
CLUSTER_CONFIG="/data/nodes.conf"
if [ -f ${CLUSTER_CONFIG} ]; then
    if [ -z "${POD_IP}" ]; then
    echo "Unable to determine Pod IP address!"
    exit 1
    fi
    echo "Updating my IP to ${POD_IP} in ${CLUSTER_CONFIG}"
    sed -i.bak -e "/myself/ s/ .*:6379@16379/ ${POD_IP}:6379@16379/" ${CLUSTER_CONFIG}
fi
exec "$@"`

	redisConfContent := generateRedisConfContent(cluster.Spec.Config)

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            RedisConfigMapName(cluster.Name),
			Namespace:       cluster.Namespace,
			Labels:          labels,
			OwnerReferences: redisv1alpha1.DefaultOwnerReferences(cluster),
		},
		Data: map[string]string{
			"shutdown.sh": shutdownContent,
			"fix-ip.sh":   fixIPContent,
			RedisConfKey:  redisConfContent,
		},
	}
}

func generateRedisConfContent(configMap map[string]string) string {
	if configMap == nil {
		return ""
	}

	var buffer bytes.Buffer

	keys := make([]string, 0, len(configMap))
	for k := range configMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		v := configMap[k]
		if len(v) == 0 {
			continue
		}
		buffer.WriteString(fmt.Sprintf("%s %s", k, v))
		buffer.WriteString("\n")
	}

	return buffer.String()
}

func RedisConfigMapName(clusterName string) string {
	return fmt.Sprintf("%s-%s", "redis-cluster", clusterName)
}

func NewConfigMapForRestore(cluster *redisv1alpha1.DistributedRedisCluster, labels map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            RestoreConfigMapName(cluster.Name),
			Namespace:       cluster.Namespace,
			Labels:          labels,
			OwnerReferences: redisv1alpha1.DefaultOwnerReferences(cluster),
		},
		Data: map[string]string{
			RestoreSucceeded: strconv.Itoa(0),
		},
	}
}

func RestoreConfigMapName(clusterName string) string {
	return fmt.Sprintf("%s-%s", "rediscluster-restore", clusterName)
}
