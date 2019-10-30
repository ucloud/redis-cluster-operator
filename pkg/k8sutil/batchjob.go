package k8sutil

import (
	"context"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IJobControl definej the interface that usej to create, update, and delete Jobs.
type IJobControl interface {
	CreateJob(*batchv1.Job) error
	UpdateJob(*batchv1.Job) error
	DeleteJob(*batchv1.Job) error
	GetJob(namespace, name string) (*batchv1.Job, error)
	ListJobByLabels(namespace string, labs client.MatchingLabels) (*batchv1.JobList, error)
}

type JobController struct {
	client client.Client
}

// NewRealJobControl creates a concrete implementation of the
// IJobControl.
func NewJobController(client client.Client) IJobControl {
	return &JobController{client: client}
}

// CreateJob implement the IJobControl.Interface.
func (j *JobController) CreateJob(job *batchv1.Job) error {
	return j.client.Create(context.TODO(), job)
}

// UpdateJob implement the IJobControl.Interface.
func (j *JobController) UpdateJob(job *batchv1.Job) error {
	return j.client.Update(context.TODO(), job)
}

// DeleteJob implement the IJobControl.Interface.
func (j *JobController) DeleteJob(job *batchv1.Job) error {
	return j.client.Delete(context.TODO(), job)
}

// GetJob implement the IJobControl.Interface.
func (j *JobController) GetJob(namespace, name string) (*batchv1.Job, error) {
	job := &batchv1.Job{}
	err := j.client.Get(context.TODO(), types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, job)
	return job, err
}

func (j *JobController) ListJobByLabels(namespace string, labs client.MatchingLabels) (*batchv1.JobList, error) {
	jobList := &batchv1.JobList{}
	opts := []client.ListOption{
		client.InNamespace(namespace),
		labs,
	}
	err := j.client.List(context.TODO(), jobList, opts...)
	if err != nil {
		return nil, err
	}

	return jobList, nil
}
