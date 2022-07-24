/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	foxv1alpha1 "github.com/peng225/fox/api/v1alpha1"
)

// PVCBackupReconciler reconciles a PVCBackup object
type PVCBackupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const (
	reconcilePeriod = 10
)

//+kubebuilder:rbac:groups=fox.peng225.github.io,resources=pvcbackups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=fox.peng225.github.io,resources=pvcbackups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=fox.peng225.github.io,resources=pvcbackups/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the PVCBackup object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.1/pkg/reconcile
func (r *PVCBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	logger := log.FromContext(ctx)

	var pvcBackup foxv1alpha1.PVCBackup
	err := r.Get(ctx, req.NamespacedName, &pvcBackup)
	if errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}
	if err != nil {
		logger.Error(err, "Failed to get PVCBackup", "name", req.NamespacedName)
		return ctrl.Result{}, err
	}

	if !pvcBackup.ObjectMeta.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	// TODO: check the all log message
	originalBackupStatus := pvcBackup.Status.BackupStatus
	switch pvcBackup.Status.BackupStatus {
	case "":
		pvcBackup.Status.BackupStatus = foxv1alpha1.BackupNotStarted
	case foxv1alpha1.BackupNotStarted:
		err = r.startBackupJob(ctx, &pvcBackup)
		if err != nil {
			logger.Error(err, "startBackupJob failed", "name", req.NamespacedName)
			return ctrl.Result{}, err
		}
		pvcBackup.Status.BackupStatus = foxv1alpha1.BackupInProgress
		logger.Info("Update status to BackupInProgress", "name", req.NamespacedName)
	case foxv1alpha1.BackupInProgress:
		// TODO: Consider the case where a job vanishes due to the repeated failures.
		bkJobCompleted, err := r.isBackupJobCompleted(ctx, &pvcBackup)
		if err != nil {
			logger.Error(err, "isBackupJobCompleted failed", "name", req.NamespacedName)
			return ctrl.Result{}, err
		} else if bkJobCompleted {
			pvcBackup.Status.BackupStatus = foxv1alpha1.BackupCompleted
			logger.Info("Update status to BackupCompleted", "name", req.NamespacedName)
		} else {
			return ctrl.Result{RequeueAfter: reconcilePeriod}, nil
		}
	case foxv1alpha1.BackupError:
		pvcBackup.Status.BackupStatus = foxv1alpha1.BackupNotStarted
	case foxv1alpha1.BackupCompleted:
		// Nothing to do
	default:
		pvcBackup.Status.BackupStatus = foxv1alpha1.BackupNotStarted
		logger.Error(fmt.Errorf("internal logic error"), "Unknown backup status", "name", req.NamespacedName, "backupStatus", pvcBackup.Status.BackupStatus)
	}

	if pvcBackup.Status.BackupStatus != originalBackupStatus {
		err = r.Status().Update(ctx, &pvcBackup)
		if err != nil {
			logger.Error(err, "Status update failed", "name", req.NamespacedName)
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PVCBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&foxv1alpha1.PVCBackup{}).
		Owns(&corev1.Pod{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}

func (r *PVCBackupReconciler) startBackupJob(ctx context.Context, pvcBackup *foxv1alpha1.PVCBackup) error {
	// Create a source Pod
	sourcePodName := generateSourcePodName(pvcBackup)
	var srcPod corev1.Pod
	err := r.Get(ctx, client.ObjectKey{Namespace: pvcBackup.Spec.SourceNamespace, Name: sourcePodName}, &srcPod)
	if err != nil {
		if errors.IsNotFound(err) {
			srcPod = corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      sourcePodName,
					Namespace: pvcBackup.Spec.DestinationNamespace,
					Labels:    map[string]string{"app": "fox"},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					Containers: []corev1.Container{
						{
							Name:            sourcePodName,
							Image:           "rdiff-backup:latest",
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{"/bin/sh", "-c"},
							Args:            []string{"service ssh start && sleep infinity"},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "data",
									MountPath: "/data",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "data",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: pvcBackup.Spec.SourcePVC,
								},
							},
						},
					},
				},
			}
			err = ctrl.SetControllerReference(pvcBackup, &srcPod, r.Scheme)
			if err != nil {
				return err
			}
			err := r.Create(ctx, &srcPod)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	// Create a destination Job
	backupJobName := generateBackupJobName(pvcBackup)
	var dstJob batchv1.Job
	err = r.Get(ctx, client.ObjectKey{Namespace: pvcBackup.Spec.DestinationNamespace, Name: backupJobName}, &dstJob)
	if err != nil {
		if errors.IsNotFound(err) {
			if srcPod.Status.PodIP == "" {
				return fmt.Errorf("no IP address is assigned to the source pod")
			}
			dstJob = batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      backupJobName,
					Namespace: pvcBackup.Spec.SourceNamespace,
					Labels:    map[string]string{"app": "fox"},
				},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "fox"},
						},
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyOnFailure,
							Containers: []corev1.Container{
								{
									Name:            backupJobName,
									Image:           "rdiff-backup:latest",
									ImagePullPolicy: corev1.PullIfNotPresent,
									Command:         []string{"/bin/sh", "-c"},
									Args: []string{
										"rdiff-backup --remote-schema 'ssh -i /root/.ssh/id_rsa -o StrictHostKeyChecking=no -C %s rdiff-backup --server' --print " + srcPod.Status.PodIP + "::/data /backup",
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "backup",
											MountPath: "/backup",
										},
									},
								},
							},
							Volumes: []corev1.Volume{
								{
									Name: "backup",
									VolumeSource: corev1.VolumeSource{
										PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
											ClaimName: pvcBackup.Spec.DestinationPVC,
										},
									},
								},
							},
						},
					},
				},
			}
			err = ctrl.SetControllerReference(pvcBackup, &dstJob, r.Scheme)
			if err != nil {
				return err
			}
			err := r.Create(ctx, &dstJob)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	return nil
}

func (r *PVCBackupReconciler) isBackupJobCompleted(ctx context.Context, pvcBackup *foxv1alpha1.PVCBackup) (bool, error) {
	// TODO: handle the case where a job completes with an error status.
	backupJobName := generateBackupJobName(pvcBackup)
	var dstJob batchv1.Job
	err := r.Get(ctx, client.ObjectKey{Namespace: pvcBackup.Spec.DestinationNamespace, Name: backupJobName}, &dstJob)
	if err != nil {
		return false, err
	}
	if dstJob.Status.CompletionTime.IsZero() {
		return false, nil
	}
	return true, nil
}

func generateBackupJobName(pvcBackup *foxv1alpha1.PVCBackup) string {
	return "pvc-backup-job-" + pvcBackup.Spec.DestinationNamespace + "-" + pvcBackup.Name
}

func generateSourcePodName(pvcBackup *foxv1alpha1.PVCBackup) string {
	return "pvc-backup-pod-" + pvcBackup.Spec.SourceNamespace + "-" + pvcBackup.Name
}
