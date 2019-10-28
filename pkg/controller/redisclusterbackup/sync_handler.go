package redisclusterbackup

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
	"github.com/ucloud/redis-cluster-operator/pkg/event"
	"github.com/ucloud/redis-cluster-operator/pkg/osm"
)

const (
	backupDumpDir  = "/var/data"
	UtilVolumeName = "util-volume"
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

	// Do not process "completed", aka "failed" or "succeeded", backups.
	if backup.Status.Phase == redisv1alpha1.BackupPhaseFailed || backup.Status.Phase == redisv1alpha1.BackupPhaseSucceeded {
		//delete(backup.GetLabels(), redisv1alpha1.LabelBackupStatus)
		//if err := r.crController.UpdateCR(backup); err != nil {
		//	r.recorder.Event(
		//		backup,
		//		corev1.EventTypeWarning,
		//		event.BackupError,
		//		err.Error(),
		//	)
		//	return err
		//}
		return nil
	}

	if err := r.ValidateBackup(backup); err != nil {
		if IsRequestRetryable(err) {
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
		msg := "One Backup is already Running"
		r.markAsFailedBackup(backup, msg)
		r.recorder.Event(
			backup,
			corev1.EventTypeWarning,
			event.BackupFailed,
			msg,
		)
		return nil
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

	secret, err := osm.NewOSMSecret(r.client, OSMSecretName(backup.Name), backup.Namespace, backup.Spec.Backend)
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

	if err := createSecret(r.client, secret); err != nil {
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

	job, err := r.getBackupJob(backup, cluster)
	if err != nil {
		message := fmt.Sprintf("Failed to create Backup Job. Reason: %v", err)
		r.recorder.Event(
			backup,
			corev1.EventTypeWarning,
			event.BackupError,
			message,
		)
		if IsRequestRetryable(err) {
			return err
		}
		r.markAsFailedBackup(backup, message)
		return nil
	}

	backup.Status.Phase = redisv1alpha1.BackupPhaseRunning
	if err := r.crController.UpdateCRStatus(backup); err != nil {
		r.recorder.Event(
			backup,
			corev1.EventTypeWarning,
			event.BackupFailed,
			err.Error(),
		)
		return err
	}

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
			event.BackupFailed,
			err.Error(),
		)
		return err
	}

	return nil
}

func (r *ReconcileRedisClusterBackup) ValidateBackup(backup *redisv1alpha1.RedisClusterBackup) error {
	if err := backup.Validate(); err != nil {
		return err
	}

	if _, err := r.crController.GetDistributedRedisCluster(backup.Namespace, backup.Spec.RedisClusterName); err != nil {
		return err
	}

	return nil
}

func (r *ReconcileRedisClusterBackup) getBackupJob(backup *redisv1alpha1.RedisClusterBackup, cluster *redisv1alpha1.DistributedRedisCluster) (*batchv1.Job, error) {
	jobName := fmt.Sprintf("%s-%s", "redisbacup", backup.Name)
	jobLabel := backup.GetLabels()
	if jobLabel == nil {
		jobLabel = map[string]string{}
	}
	backupSpec := backup.Spec.Backend
	bucket, err := backupSpec.Container()
	if err != nil {
		return nil, err
	}

	dumpArgs := []string{"--all-databases"}

	persistentVolume, err := r.GetVolumeForBackup(backup.Spec.Storage, jobName, backup.Namespace)
	if err != nil {
		return nil, err
	}

	// Folder name inside Cloud bucket where backup will be uploaded
	folderName, err := backup.Location()
	if err != nil {
		return nil, err
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:   jobName,
			Labels: jobLabel,
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
					Containers: []corev1.Container{
						{
							Name:  redisv1alpha1.JobTypeBackup,
							Image: backup.Spec.Image,
							Args: append([]string{
								redisv1alpha1.JobTypeBackup,
								fmt.Sprintf(`--data-dir=%s`, backupDumpDir),
								fmt.Sprintf(`--bucket=%s`, bucket),
								fmt.Sprintf(`--folder=%s`, folderName),
								fmt.Sprintf(`--backup=%s`, backup.Name),
								fmt.Sprintf(`--enable-analytics=%v`, "false"),
								"--",
							}, dumpArgs...),
							Env: upsertEnvVars([]corev1.EnvVar{
								{
									Name: "REDIS_PASSWORD",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: cluster.Spec.PasswordSecret.Name,
											},
											Key: "password",
										},
									},
								},
							}),
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      persistentVolume.Name,
									MountPath: backupDumpDir,
								},
								{
									Name:      "osmconfig",
									ReadOnly:  true,
									MountPath: osm.SecretMountPath,
								},
							},
						},
					},
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
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		},
	}
	if backup.Spec.Backend.Local != nil {
		job.Spec.Template.Spec.Containers[0].VolumeMounts = append(job.Spec.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      "local",
			MountPath: backup.Spec.Backend.Local.MountPath,
			SubPath:   backup.Spec.Backend.Local.SubPath,
		})
		job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes, corev1.Volume{
			Name:         "local",
			VolumeSource: backup.Spec.Backend.Local.VolumeSource,
		})
	}

	return job, nil
}

// GetVolumeForBackup returns pvc or empty directory depending on StorageType.
// In case of PVC, this function will create a PVC then returns the volume.
func (r *ReconcileRedisClusterBackup) GetVolumeForBackup(storage *redisv1alpha1.RedisStorage, jobName, namespace string) (*corev1.Volume, error) {
	if storage == nil || storage.Type == redisv1alpha1.Ephemeral {
		ed := corev1.EmptyDirVolumeSource{}
		return &corev1.Volume{
			Name: UtilVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &ed,
			},
		}, nil
	}

	volume := &corev1.Volume{
		Name: UtilVolumeName,
	}
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
			Namespace: namespace,
		},
		Spec: *pvcSpec,
	}

	if err := r.client.Create(context.TODO(), claim); err != nil {
		return nil, err
	}

	volume.PersistentVolumeClaim = &corev1.PersistentVolumeClaimVolumeSource{
		ClaimName: claim.Name,
	}

	return volume, nil
}
