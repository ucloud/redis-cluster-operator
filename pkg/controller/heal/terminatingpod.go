package heal

import (
	"time"

	"k8s.io/apimachinery/pkg/util/errors"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
)

// FixTerminatingPods used to for the deletion of pod blocked in terminating status.
// in it append the this method will for the deletion of the Pod.
func (c *CheckAndHeal) FixTerminatingPods(cluster *redisv1alpha1.DistributedRedisCluster, maxDuration time.Duration) (bool, error) {
	var errs []error
	var actionDone bool

	if maxDuration == time.Duration(0) {
		return actionDone, nil
	}

	now := time.Now()
	for _, pod := range c.Pods {
		if pod.DeletionTimestamp == nil {
			// ignore pod without deletion timestamp
			continue
		}
		maxTime := pod.DeletionTimestamp.Add(maxDuration) // adding MaxDuration for configuration
		if maxTime.Before(now) {
			c.Logger.Info("[FixTerminatingPods] found deletion pod", "podName", pod.Name)
			actionDone = true
			// it means that this pod should already been deleted since a wild
			if !c.DryRun {
				c.Logger.Info("[FixTerminatingPods] try to delete pod", "podName", pod.Name)
				if err := c.PodControl.DeletePodByName(cluster.Namespace, pod.Name); err != nil {
					errs = append(errs, err)
				}
			}
		}
	}

	return actionDone, errors.NewAggregate(errs)
}
