package redisclusterbackup

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
	"github.com/ucloud/redis-cluster-operator/pkg/event"
	"github.com/ucloud/redis-cluster-operator/pkg/k8sutil"
	"github.com/ucloud/redis-cluster-operator/pkg/osm"
	"github.com/ucloud/redis-cluster-operator/pkg/utils"
)

func (r *ReconcileRedisClusterBackup) create(reqLogger logr.Logger, backup *redisv1alpha1.RedisClusterBackup) error {
	if backup.Status.StartTime == nil {
		t := metav1.Now()
		backup.Status.StartTime = &t
		if err := r.crController.UpdateCRStatus(backup); err != nil {
			r.recorder.Event(
				backup,
				corev1.EventTypeWarning,
				event.BackupError,
				err.Error(),
			)
			return err
		}
	}

	// Do not process "completed", aka "failed" or "succeeded" or "ignored", backups.
	if backup.Status.Phase == redisv1alpha1.BackupPhaseFailed ||
		backup.Status.Phase == redisv1alpha1.BackupPhaseSucceeded ||
		backup.Status.Phase == redisv1alpha1.BackupPhaseIgnored {
		delete(backup.GetLabels(), redisv1alpha1.LabelBackupStatus)
		if err := r.crController.UpdateCR(backup); err != nil {
			r.recorder.Event(
				backup,
				corev1.EventTypeWarning,
				event.BackupError,
				err.Error(),
			)
			return err
		}
		return nil
	}

	//if backup.Labels == nil {
	//	backup.Labels = make(map[string]string)
	//}
	//backup.Labels[redisv1alpha1.LabelClusterName] = backup.Spec.RedisClusterName
	//if err := r.crController.UpdateCR(backup); err != nil {
	//	r.recorder.Event(
	//		backup,
	//		corev1.EventTypeWarning,
	//		event.BackupError,
	//		err.Error(),
	//	)
	//	return err
	//}

	if err := r.ValidateBackup(backup); err != nil {
		if k8sutil.IsRequestRetryable(err) {
			return err
		}
		r.markAsFailedBackup(backup, err.Error())
		r.recorder.Event(
			backup,
			corev1.EventTypeWarning,
			event.BackupFailed,
			err.Error(),
		)
		return nil // stop retry
	}

	running, err := r.isBackupRunning(backup)
	if err != nil {
		r.recorder.Event(
			backup,
			corev1.EventTypeWarning,
			event.BackupError,
			err.Error(),
		)
		return err
	}
	if running {
		return r.handleBackupJob(reqLogger, backup)
	}

	cluster, err := r.crController.GetDistributedRedisCluster(backup.Namespace, backup.Spec.RedisClusterName)
	if err != nil {
		r.recorder.Event(
			backup,
			corev1.EventTypeWarning,
			event.BackupError,
			err.Error(),
		)
		return err
	}

	secret, err := osm.NewCephSecret(r.client, backup.OSMSecretName(), backup.Namespace, backup.Spec.Backend)
	if err != nil {
		msg := fmt.Sprintf("Failed to generate osm secret. Reason: %v", err)
		r.markAsFailedBackup(backup, msg)
		r.recorder.Event(
			backup,
			corev1.EventTypeWarning,
			event.BackupFailed,
			msg,
		)
		return nil // don't retry
	}

	if err := k8sutil.CreateSecret(r.client, secret, reqLogger); err != nil {
		r.recorder.Event(
			backup,
			corev1.EventTypeWarning,
			event.BackupError,
			err.Error(),
		)
		return err
	}

	if backup.Spec.Local == nil {
		if err := osm.CheckBucketAccess(r.client, backup.Spec.Backend, backup.Namespace); err != nil {
			r.markAsFailedBackup(backup, err.Error())
			r.recorder.Event(
				backup,
				corev1.EventTypeWarning,
				event.BackupFailed,
				err.Error(),
			)
			return nil
		}
	}

	job, err := r.getBackupJob(reqLogger, backup, cluster)
	if err != nil {
		message := fmt.Sprintf("Failed to create Backup Job. Reason: %v", err)
		r.recorder.Event(
			backup,
			corev1.EventTypeWarning,
			event.BackupError,
			message,
		)
		if k8sutil.IsRequestRetryable(err) {
			return err
		}
		return r.markAsFailedBackup(backup, message)
	}

	backup.Status.Phase = redisv1alpha1.BackupPhaseRunning
	backup.Status.MasterSize = cluster.Spec.MasterSize
	backup.Status.ClusterReplicas = cluster.Spec.ClusterReplicas
	backup.Status.ClusterImage = cluster.Spec.Image
	if err := r.crController.UpdateCRStatus(backup); err != nil {
		r.recorder.Event(
			backup,
			corev1.EventTypeWarning,
			event.BackupFailed,
			err.Error(),
		)
		return err
	}

	backup.Labels[redisv1alpha1.LabelClusterName] = backup.Spec.RedisClusterName
	backup.Labels[redisv1alpha1.LabelBackupStatus] = string(redisv1alpha1.BackupPhaseRunning)
	if err := r.crController.UpdateCR(backup); err != nil {
		r.recorder.Event(
			backup,
			corev1.EventTypeWarning,
			event.BackupError,
			err.Error(),
		)
		return err
	}

	reqLogger.Info("Backup running")
	r.recorder.Event(
		backup,
		corev1.EventTypeNormal,
		event.Starting,
		"Backup running",
	)

	if err := r.client.Create(context.TODO(), job); err != nil {
		r.recorder.Event(
			backup,
			corev1.EventTypeWarning,
			event.BackupError,
			err.Error(),
		)
		return err
	}

	return nil
}

func (r *ReconcileRedisClusterBackup) ValidateBackup(backup *redisv1alpha1.RedisClusterBackup) error {
	if backup.Labels == nil {
		backup.Labels = make(map[string]string)
	}
	if err := backup.Validate(); err != nil {
		return err
	}

	if _, err := r.crController.GetDistributedRedisCluster(backup.Namespace, backup.Spec.RedisClusterName); err != nil {
		return err
	}

	return nil
}

func (r *ReconcileRedisClusterBackup) getBackupJob(reqLogger logr.Logger, backup *redisv1alpha1.RedisClusterBackup, cluster *redisv1alpha1.DistributedRedisCluster) (*batchv1.Job, error) {
	jobName := backup.JobName()
	jobLabel := map[string]string{
		redisv1alpha1.LabelClusterName:  backup.Spec.RedisClusterName,
		redisv1alpha1.AnnotationJobType: redisv1alpha1.JobTypeBackup,
	}

	persistentVolume, err := r.GetVolumeForBackup(backup, jobName)
	if err != nil {
		return nil, err
	}

	containers, err := r.backupContainers(backup, cluster)
	if err != nil {
		return nil, err
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: backup.Namespace,
			Labels:    jobLabel,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: redisv1alpha1.SchemeGroupVersion.String(),
					Kind:       redisv1alpha1.RedisClusterBackupKind,
					Name:       backup.Name,
					UID:        backup.UID,
				},
			},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: containers,
					Volumes: []corev1.Volume{
						{
							Name:         persistentVolume.Name,
							VolumeSource: persistentVolume.VolumeSource,
						},
						{
							Name: "osmconfig",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: backup.OSMSecretName(),
								},
							},
						},
					},
					RestartPolicy:     corev1.RestartPolicyNever,
					NodeSelector:      backup.Spec.PodSpec.NodeSelector,
					Affinity:          backup.Spec.PodSpec.Affinity,
					SchedulerName:     backup.Spec.PodSpec.SchedulerName,
					Tolerations:       backup.Spec.PodSpec.Tolerations,
					PriorityClassName: backup.Spec.PodSpec.PriorityClassName,
					Priority:          backup.Spec.PodSpec.Priority,
					SecurityContext:   backup.Spec.PodSpec.SecurityContext,
					ImagePullSecrets:  backup.Spec.PodSpec.ImagePullSecrets,
				},
			},
		},
	}
	if backup.Spec.Backend.Local != nil {
		job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes, corev1.Volume{
			Name:         "local",
			VolumeSource: backup.Spec.Backend.Local.VolumeSource,
		})
	}

	return job, nil
}

func (r *ReconcileRedisClusterBackup) backupContainers(backup *redisv1alpha1.RedisClusterBackup, cluster *redisv1alpha1.DistributedRedisCluster) ([]corev1.Container, error) {
	backupSpec := backup.Spec.Backend
	bucket, err := backupSpec.Container()
	if err != nil {
		return nil, err
	}
	masterNum := int(cluster.Spec.MasterSize)
	containers := make([]corev1.Container, masterNum)
	i := 0
	for _, node := range cluster.Status.Nodes {
		if node.Role == redisv1alpha1.RedisClusterNodeRoleMaster {
			if i == masterNum {
				break
			}
			folderName, err := backup.Location()
			if err != nil {
				r.recorder.Event(
					backup,
					corev1.EventTypeWarning,
					event.BackupError,
					err.Error(),
				)
				return nil, err
			}
			container := corev1.Container{
				Name:            fmt.Sprintf("%s-%d", redisv1alpha1.JobTypeBackup, i),
				Image:           backup.Spec.Image,
				ImagePullPolicy: "Always",
				Args: []string{
					redisv1alpha1.JobTypeBackup,
					fmt.Sprintf(`--data-dir=%s`, redisv1alpha1.BackupDumpDir),
					fmt.Sprintf(`--bucket=%s`, bucket),
					fmt.Sprintf(`--enable-analytics=%v`, "false"),
					fmt.Sprintf(`--host=%s`, node.IP),
					fmt.Sprintf(`--folder=%s`, folderName),
					fmt.Sprintf(`--snapshot=%s-%d`, backup.Name, i),
					"--",
				},
				Resources:      backup.Spec.PodSpec.Resources,
				LivenessProbe:  backup.Spec.PodSpec.LivenessProbe,
				ReadinessProbe: backup.Spec.PodSpec.ReadinessProbe,
				Lifecycle:      backup.Spec.PodSpec.Lifecycle,
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      redisv1alpha1.UtilVolumeName,
						MountPath: redisv1alpha1.BackupDumpDir,
					},
					{
						Name:      "osmconfig",
						ReadOnly:  true,
						MountPath: osm.SecretMountPath,
					},
				},
			}
			if cluster.Spec.PasswordSecret != nil {
				container.Env = append(container.Env, redisPassword(cluster))
			}
			if backup.Spec.Backend.Local != nil {
				container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
					Name:      "local",
					MountPath: backup.Spec.Backend.Local.MountPath,
					SubPath:   backup.Spec.Backend.Local.SubPath,
				})
			}
			containers[i] = container
			i++
		}
	}
	return containers, nil
}

// GetVolumeForBackup returns pvc or empty directory depending on StorageType.
// In case of PVC, this function will create a PVC then returns the volume.
func (r *ReconcileRedisClusterBackup) GetVolumeForBackup(backup *redisv1alpha1.RedisClusterBackup, jobName string) (*corev1.Volume, error) {
	storage := backup.Spec.Storage
	if storage == nil || storage.Type == redisv1alpha1.Ephemeral {
		ed := corev1.EmptyDirVolumeSource{}
		return &corev1.Volume{
			Name: redisv1alpha1.UtilVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &ed,
			},
		}, nil
	}

	volume := &corev1.Volume{
		Name: redisv1alpha1.UtilVolumeName,
	}

	if err := r.createPVCForBackup(backup, jobName); err != nil {
		return nil, err
	}

	volume.PersistentVolumeClaim = &corev1.PersistentVolumeClaimVolumeSource{
		ClaimName: jobName,
	}

	return volume, nil
}

func (r *ReconcileRedisClusterBackup) createPVCForBackup(backup *redisv1alpha1.RedisClusterBackup, jobName string) error {
	getClaim := &corev1.PersistentVolumeClaim{}
	err := r.client.Get(context.TODO(), types.NamespacedName{
		Namespace: backup.Namespace,
		Name:      jobName,
	}, getClaim)
	if err != nil {
		if errors.IsNotFound(err) {
			storage := backup.Spec.Storage
			mode := corev1.PersistentVolumeFilesystem
			pvcSpec := &corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: storage.Size,
					},
				},
				StorageClassName: &storage.Class,
				VolumeMode:       &mode,
			}

			claim := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      jobName,
					Namespace: backup.Namespace,
				},
				Spec: *pvcSpec,
			}
			if storage.DeleteClaim {
				claim.OwnerReferences = []metav1.OwnerReference{
					{
						APIVersion: redisv1alpha1.SchemeGroupVersion.String(),
						Kind:       redisv1alpha1.RedisClusterBackupKind,
						Name:       backup.Name,
						UID:        backup.UID,
					},
				}
			}

			return r.client.Create(context.TODO(), claim)
		}
		return err
	}
	return nil
}

func (r *ReconcileRedisClusterBackup) handleBackupJob(reqLogger logr.Logger, backup *redisv1alpha1.RedisClusterBackup) error {
	reqLogger.Info("Handle Backup Job")
	job, err := r.jobController.GetJob(backup.Namespace, backup.JobName())
	if err != nil {
		// TODO: Sometimes the job is created successfully, but it cannot be obtained immediately.
		if errors.IsNotFound(err) {
			msg := "One Backup is already Running"
			reqLogger.Info(msg, "err", err)
			r.markAsIgnoredBackup(backup, msg)
			r.recorder.Event(
				backup,
				corev1.EventTypeWarning,
				event.BackupFailed,
				msg,
			)
			return nil
		}
		return err
	}
	if job.Status.Succeeded == 0 && job.Status.Failed < utils.Int32(job.Spec.BackoffLimit) {
		return fmt.Errorf("wait for job Succeeded or Failed")
	}

	cluster, err := r.crController.GetDistributedRedisCluster(backup.Namespace, backup.Spec.RedisClusterName)
	if err != nil {
		r.recorder.Event(
			backup,
			corev1.EventTypeWarning,
			event.BackupError,
			err.Error(),
		)
		return err
	}
	for _, o := range job.OwnerReferences {
		if o.Kind == redisv1alpha1.RedisClusterBackupKind {
			if o.Name == backup.Name {
				jobSucceeded := job.Status.Succeeded > 0
				if jobSucceeded {
					backup.Status.Phase = redisv1alpha1.BackupPhaseSucceeded
				} else {
					backup.Status.Phase = redisv1alpha1.BackupPhaseFailed
					backup.Status.Reason = "run batch job failed"
				}
				t := metav1.Now()
				backup.Status.CompletionTime = &t
				if err := r.crController.UpdateCRStatus(backup); err != nil {
					r.recorder.Event(
						backup,
						corev1.EventTypeWarning,
						event.BackupError,
						err.Error(),
					)
					return err
				}

				delete(backup.GetLabels(), redisv1alpha1.LabelBackupStatus)
				if err := r.crController.UpdateCR(backup); err != nil {
					r.recorder.Event(
						backup,
						corev1.EventTypeWarning,
						event.BackupError,
						err.Error(),
					)
					return err
				}

				if jobSucceeded {
					msg := "Successfully completed backup"
					reqLogger.Info(msg)
					r.recorder.Event(
						backup,
						corev1.EventTypeNormal,
						event.BackupSuccessful,
						msg,
					)
					r.recorder.Event(
						cluster,
						corev1.EventTypeNormal,
						event.BackupSuccessful,
						msg,
					)
				} else {
					msg := "Failed to complete backup"
					reqLogger.Info(msg)
					r.recorder.Event(
						backup,
						corev1.EventTypeWarning,
						event.BackupFailed,
						msg,
					)
					r.recorder.Event(
						cluster,
						corev1.EventTypeWarning,
						event.BackupFailed,
						msg,
					)
				}
			} else {
				msg := "One Backup is already Running"
				reqLogger.Info(msg, o.Name, backup.Name)
				r.markAsIgnoredBackup(backup, msg)
				r.recorder.Event(
					backup,
					corev1.EventTypeWarning,
					event.BackupFailed,
					msg,
				)
			}
		}
	}

	return nil
}
