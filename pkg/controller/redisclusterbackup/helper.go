package redisclusterbackup

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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

	if len(backupList.Items) > 0 {
		return true, nil
	}

	return false, nil
}

func OSMSecretName(name string) string {
	return fmt.Sprintf("osm-%v", name)
}

func createSecret(client client.Client, secret *corev1.Secret) error {
	ctx := context.TODO()
	s := &corev1.Secret{}
	err := client.Get(ctx, types.NamespacedName{
		Namespace: secret.Namespace,
		Name:      secret.Name,
	}, s)
	if err != nil {
		if errors.IsNotFound(err) {
			return client.Create(ctx, secret)
		}
	}
	return err
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

func IsRequestRetryable(err error) bool {
	return kerr.IsServiceUnavailable(err) ||
		kerr.IsTimeout(err) ||
		kerr.IsServerTimeout(err) ||
		kerr.IsTooManyRequests(err)
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
