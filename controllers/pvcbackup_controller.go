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
	v1 "k8s.io/api/core/v1"
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
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
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

	originalBackupStatus := pvcBackup.Status.BackupStatus
	switch pvcBackup.Status.BackupStatus {
	case "":
		pvcBackup.Status.BackupStatus = foxv1alpha1.BackupNotStarted
	case foxv1alpha1.BackupNotStarted:
		err := r.createDestinationPVC(ctx, &pvcBackup)
		if err != nil {
			logger.Error(err, "createDestinationPVC failed", "name", req.NamespacedName)
			return ctrl.Result{}, err
		}
		err = r.startBackupJob(ctx, &pvcBackup)
		if err != nil {
			logger.Error(err, "startBackupJob failed", "name", req.NamespacedName)
			return ctrl.Result{}, err
		}
		pvcBackup.Status.BackupStatus = foxv1alpha1.BackupInProgress
		logger.Info("Update status to BackupInProgress", "name", req.NamespacedName)
	case foxv1alpha1.BackupInProgress:
		bkJobFinished, err := r.isBackupJobCompleted(ctx, &pvcBackup)
		if err != nil {
			logger.Error(err, "isBackupJobCompleted failed", "name", req.NamespacedName)
			return ctrl.Result{}, err
		} else if bkJobFinished {
			pvcBackup.Status.BackupStatus = foxv1alpha1.BackupFinished
			logger.Info("Update status to BackupFinished", "name", req.NamespacedName)
		} else {
			return ctrl.Result{RequeueAfter: reconcilePeriod}, nil
		}
	case foxv1alpha1.BackupError:
		pvcBackup.Status.BackupStatus = foxv1alpha1.BackupNotStarted
	case foxv1alpha1.BackupFinished:
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
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}

func (r *PVCBackupReconciler) createDestinationPVC(ctx context.Context, pvcBackup *foxv1alpha1.PVCBackup) error {
	// Even if the controller crashes right after creating the destination PVC,
	// the destination PVC must be found in the next reconciliation loop.
	// So its name must be definitely generated from the namespace and the name of PVCBackup CR.
	destinationPVCName := generatePVCName(pvcBackup)
	var pvc v1.PersistentVolumeClaim
	err := r.Get(ctx, client.ObjectKey{Namespace: pvcBackup.Namespace, Name: destinationPVCName}, &pvc)
	if err != nil {
		if errors.IsNotFound(err) {
			srcPVCCapacity, err := r.getSourcePVCCapacity(ctx, pvcBackup)
			if err != nil {
				return err
			}
			volumeMode := corev1.PersistentVolumeFilesystem
			pvc := v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      destinationPVCName,
					Namespace: pvcBackup.Namespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					StorageClassName: &pvcBackup.Spec.DestinationStorageClass,
					VolumeMode:       &volumeMode,
					Resources: corev1.ResourceRequirements{
						Requests: *srcPVCCapacity,
					},
				},
			}
			err = ctrl.SetControllerReference(pvcBackup, &pvc, r.Scheme)
			if err != nil {
				return err
			}
			err = r.Create(ctx, &pvc)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	pvcBackup.Status.DestinationPVC = destinationPVCName
	return nil
}

func (r *PVCBackupReconciler) getSourcePVCCapacity(ctx context.Context, pvcBackup *foxv1alpha1.PVCBackup) (*corev1.ResourceList, error) {
	var pvc v1.PersistentVolumeClaim
	err := r.Get(ctx, client.ObjectKey{Namespace: pvcBackup.Namespace, Name: pvcBackup.Spec.SourcePVC}, &pvc)
	if err != nil {
		return nil, err
	}

	return &pvc.Spec.Resources.Requests, nil
}

func (r *PVCBackupReconciler) startBackupJob(ctx context.Context, pvcBackup *foxv1alpha1.PVCBackup) error {
	// TODO: mount src/dst PVC and start backup.
	backupJobName := generateBackupJobName(pvcBackup)
	var job batchv1.Job
	err := r.Get(ctx, client.ObjectKey{Namespace: pvcBackup.Namespace, Name: backupJobName}, &job)
	if err != nil {
		if errors.IsNotFound(err) {
			job := batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      backupJobName,
					Namespace: pvcBackup.Namespace,
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
									Name:    backupJobName,
									Image:   "ubuntu:20.04",
									Command: []string{"sleep"},
									Args:    []string{"30"},
								},
							},
						},
					},
				},
			}
			err = ctrl.SetControllerReference(pvcBackup, &job, r.Scheme)
			if err != nil {
				return err
			}
			err := r.Create(ctx, &job)
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
	backupJobName := generateBackupJobName(pvcBackup)
	var job batchv1.Job
	err := r.Get(ctx, client.ObjectKey{Namespace: pvcBackup.Namespace, Name: backupJobName}, &job)
	if err != nil {
		return false, err
	}
	if job.Status.CompletionTime.IsZero() {
		return false, nil
	}
	return true, nil
}

func generatePVCName(pvcBackup *foxv1alpha1.PVCBackup) string {
	return "pvc-backup-" + pvcBackup.Namespace + "-" + pvcBackup.Name
}

func generateBackupJobName(pvcBackup *foxv1alpha1.PVCBackup) string {
	return "pvc-backup-job-" + pvcBackup.Namespace + "-" + pvcBackup.Name
}
