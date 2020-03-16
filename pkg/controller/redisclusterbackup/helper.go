package redisclusterbackup

import (
	"context"

	batch "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
)

func (r *ReconcileRedisClusterBackup) markAsFailedBackup(backup *redisv1alpha1.RedisClusterBackup,
	reason string) error {
	t := metav1.Now()
	backup.Status.CompletionTime = &t
	backup.Status.Phase = redisv1alpha1.BackupPhaseFailed
	backup.Status.Reason = reason
	return r.crController.UpdateCRStatus(backup)
}

func (r *ReconcileRedisClusterBackup) markAsIgnoredBackup(backup *redisv1alpha1.RedisClusterBackup,
	reason string) error {
	t := metav1.Now()
	backup.Status.CompletionTime = &t
	backup.Status.Phase = redisv1alpha1.BackupPhaseIgnored
	backup.Status.Reason = reason
	return r.crController.UpdateCRStatus(backup)
}

func (r *ReconcileRedisClusterBackup) isBackupRunning(backup *redisv1alpha1.RedisClusterBackup) (bool, error) {
	labMap := client.MatchingLabels{
		redisv1alpha1.LabelBackupStatus: string(redisv1alpha1.BackupPhaseRunning),
		redisv1alpha1.LabelClusterName:  backup.Spec.RedisClusterName,
	}
	backupList := &redisv1alpha1.RedisClusterBackupList{}
	opts := []client.ListOption{
		client.InNamespace(backup.Namespace),
		labMap,
	}
	err := r.client.List(context.TODO(), backupList, opts...)
	if err != nil {
		return false, err
	}

	jobLabMap := client.MatchingLabels{
		redisv1alpha1.LabelClusterName:  backup.Spec.RedisClusterName,
		redisv1alpha1.AnnotationJobType: redisv1alpha1.JobTypeBackup,
	}
	backupJobList, err := r.jobController.ListJobByLabels(backup.Namespace, jobLabMap)
	if err != nil {
		return false, err
	}

	if len(backupList.Items) > 0 && len(backupJobList.Items) > 0 {
		return true, nil
	}

	return false, nil
}

func upsertEnvVars(vars []corev1.EnvVar, nv ...corev1.EnvVar) []corev1.EnvVar {
	upsert := func(env corev1.EnvVar) {
		for i, v := range vars {
			if v.Name == env.Name {
				vars[i] = env
				return
			}
		}
		vars = append(vars, env)
	}

	for _, env := range nv {
		upsert(env)
	}
	return vars
}

// Returns the REDIS_PASSWORD environment variable.
func redisPassword(cluster *redisv1alpha1.DistributedRedisCluster) corev1.EnvVar {
	secretName := cluster.Spec.PasswordSecret.Name
	return corev1.EnvVar{
		Name: "REDIS_PASSWORD",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
				},
				Key: "password",
			},
		},
	}
}

func newDirectClient(config *rest.Config) client.Client {
	c, err := client.New(config, client.Options{})
	if err != nil {
		panic(err)
	}
	return c
}

func isJobFinished(j *batch.Job) bool {
	for _, c := range j.Status.Conditions {
		if (c.Type == batch.JobComplete || c.Type == batch.JobFailed) && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
