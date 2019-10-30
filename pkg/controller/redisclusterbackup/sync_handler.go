package redisclusterbackup

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
	"github.com/ucloud/redis-cluster-operator/pkg/event"
	"github.com/ucloud/redis-cluster-operator/pkg/osm"
	"github.com/ucloud/redis-cluster-operator/pkg/utils"
)

const (
	backupDumpDir  = "/data"
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
		reqLogger.Info(msg)
		//r.markAsFailedBackup(backup, msg)
		//r.recorder.Event(
		//	backup,
		//	corev1.EventTypeWarning,
		//	event.BackupFailed,
		//	msg,
		//)
		return r.handleBackupJob(reqLogger, backup)
	}

	if backup.Labels == nil {
		backup.Labels = make(map[string]string)
	}
	backup.Labels[redisv1alpha1.LabelClusterName] = backup.Spec.RedisClusterName
	if err := r.crController.UpdateCR(backup); err != nil {
		r.recorder.Event(
			backup,
			corev1.EventTypeWarning,
			event.BackupError,
			err.Error(),
		)
		return err
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

	r.recorder.Event(
		backup,
		corev1.EventTypeNormal,
		event.Starting,
		"Backup running",
	)

	i := 0
	for _, node := range cluster.Status.Nodes {
		if node.Role == redisv1alpha1.RedisClusterNodeRoleMaster {
			createJob := job.DeepCopy()
			name := fmt.Sprintf("redisbacup-%s-%d", backup.Name, i)
			// Folder name inside Cloud bucket where backup will be uploaded
			folderName, err := backup.Location()
			if err != nil {
				r.recorder.Event(
					backup,
					corev1.EventTypeWarning,
					event.BackupError,
					err.Error(),
				)
				return err
			}
			createJob.Name = name
			createJob.Spec.Template.Spec.Containers[0].Args = append(createJob.Spec.Template.Spec.Containers[0].Args,
				fmt.Sprintf(`--host=%s`, node.IP),
				fmt.Sprintf(`--folder=%s`, folderName),
				fmt.Sprintf(`--snapshot=%s-%d`, backup.Name, i),
				"--")
			i++
			if err := r.client.Create(context.TODO(), createJob); err != nil {
				r.recorder.Event(
					backup,
					corev1.EventTypeWarning,
					event.BackupError,
					err.Error(),
				)
				return err
			}
		}
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
	jobLabel := map[string]string{
		redisv1alpha1.LabelClusterName:  backup.Spec.RedisClusterName,
		redisv1alpha1.AnnotationJobType: redisv1alpha1.JobTypeBackup,
	}
	backupSpec := backup.Spec.Backend
	bucket, err := backupSpec.Container()
	if err != nil {
		return nil, err
	}

	persistentVolume, err := r.GetVolumeForBackup(backup.Spec.Storage, jobName, backup.Namespace)
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
					Containers: []corev1.Container{
						{
							Name:            redisv1alpha1.JobTypeBackup,
							Image:           backup.Spec.Image,
							ImagePullPolicy: "Always",
							Args: []string{
								redisv1alpha1.JobTypeBackup,
								fmt.Sprintf(`--data-dir=%s`, backupDumpDir),
								fmt.Sprintf(`--bucket=%s`, bucket),
								fmt.Sprintf(`--enable-analytics=%v`, "false"),
							},
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
	if cluster.Spec.PasswordSecret != nil {
		job.Spec.Template.Spec.Containers[0].Env = append(job.Spec.Template.Spec.Containers[0].Env, redisPassword(cluster))
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

func (r *ReconcileRedisClusterBackup) handleBackupJob(reqLogger logr.Logger, backup *redisv1alpha1.RedisClusterBackup) error {
	match := client.MatchingLabels{
		redisv1alpha1.LabelClusterName:  backup.Spec.RedisClusterName,
		redisv1alpha1.AnnotationJobType: redisv1alpha1.JobTypeBackup,
	}
	jobs, err := r.jobController.ListJobByLabels(backup.Namespace, match)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
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

	masterSize := int(cluster.Spec.MasterSize)
	jobNum := len(jobs.Items)
	reqLogger.V(3).Info("Handle Backup Job", "jobNum", jobNum, "masterNum", masterSize)
	// wait all job start
	if jobNum != masterSize {
		return fmt.Errorf("wait for all job start")
	}
	jobSucceededNum := 0
	for _, job := range jobs.Items {
		// wait all job succeeded or failed
		if job.Status.Succeeded == 0 && job.Status.Failed <= utils.Int32(job.Spec.BackoffLimit) {
			return fmt.Errorf("wait for jobs succeeded or failed")
		}
		if job.Status.Succeeded > 0 {
			jobSucceededNum++
		}
	}

	if jobNum == jobSucceededNum {
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

	if jobNum == jobSucceededNum {
		msg := "Successfully completed backup"
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
