/*
Copyright 2025.

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

package controller

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	depappsv1 "github.com/shalinibani/deployment-freezer/api/v1"
)

// DeploymentFreezerReconciler reconciles a DeploymentFreezer object
type DeploymentFreezerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=apps.mydomain.dev,resources=deploymentfreezers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.mydomain.dev,resources=deploymentfreezers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.mydomain.dev,resources=deploymentfreezers/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch

func (r *DeploymentFreezerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	var deploymentFreezer depappsv1.DeploymentFreezer
	if err := r.Get(ctx, req.NamespacedName, &deploymentFreezer); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if deploymentFreezer.Status.CompletionTime != nil {
		logger.Info("Reconciliation already completed for this DeploymentFreezer", "name", deploymentFreezer.Name)

		return ctrl.Result{}, nil
	}

	var targetDeployment appsv1.Deployment
	if err := r.Get(ctx, client.ObjectKey{Namespace: req.Namespace, Name: deploymentFreezer.Spec.DeploymentName}, &targetDeployment); err != nil {
		logger.Error(err, "Failed to get target Deployment", "deploymentName", deploymentFreezer.Spec.DeploymentName)

		return ctrl.Result{}, err
	}
	// Store the original replica count.
	if deploymentFreezer.Status.PrevReplicaCount == nil {
		return r.freezeDeployment(ctx, logger, targetDeployment, deploymentFreezer)
	}

	// Scale back up the deployment to its original replica count.
	if deploymentFreezer.Status.FreezeEndTime != nil {
		return r.unfreezeDeployment(ctx, logger, targetDeployment, deploymentFreezer)
	}

	return ctrl.Result{}, nil
}

func (r *DeploymentFreezerReconciler) freezeDeployment(ctx context.Context, logger logr.Logger, targetDeployment appsv1.Deployment,
	deploymentFreezer depappsv1.DeploymentFreezer) (ctrl.Result, error) {
	logger.Info("Storing original replica count and scaling down Deployment", "deploymentName", targetDeployment.Name)

	replicas := *targetDeployment.Spec.Replicas
	deploymentFreezer.Status.PrevReplicaCount = &replicas
	deploymentFreezer.Status.FreezeEndTime = &metav1.Time{Time: time.Now().Add(time.Duration(deploymentFreezer.Spec.FreezeDurationInSec) * time.Second)}

	// Scale down deployment replica count to 0 and annotate.
	targetDeployment.Spec.Replicas = int32Ptr(0)
	annotations := targetDeployment.Annotations
	if annotations == nil {
		annotations = make(map[string]string)
	}

	annotations["frozen-by"] = deploymentFreezer.Name
	targetDeployment.Annotations = annotations

	if err := r.Update(ctx, &targetDeployment); err != nil {
		logger.Error(err, "Failed to scale down Deployment", "deploymentName", targetDeployment.Name)

		return ctrl.Result{}, err
	}

	if err := r.Status().Update(ctx, &deploymentFreezer); err != nil {
		logger.Error(err, "Failed to update DeploymentFreezer status with previous replica count", "name", deploymentFreezer.Name)

		return ctrl.Result{}, err
	}

	logger.Info("Scaled down Deployment and updated DeploymentFreezer status", "deploymentName", targetDeployment.Name, "prevReplicaCount", replicas)

	return ctrl.Result{RequeueAfter: time.Duration(deploymentFreezer.Spec.FreezeDurationInSec) * time.Second}, nil
}

func (r *DeploymentFreezerReconciler) unfreezeDeployment(ctx context.Context, logger logr.Logger, targetDeployment appsv1.Deployment,
	deploymentFreezer depappsv1.DeploymentFreezer) (ctrl.Result, error) {
	now := time.Now()

	if now.Before(deploymentFreezer.Status.FreezeEndTime.Time) {
		requeueAfter := deploymentFreezer.Status.FreezeEndTime.Time.Sub(now)
		logger.Info("Freeze duration not yet elapsed, requeuing", "requeueAfter", requeueAfter)

		// Requeue it again for better relaibility in case of controller restart.
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	targetDeployment.Spec.Replicas = deploymentFreezer.Status.PrevReplicaCount
	annotations := targetDeployment.Annotations
	if annotations != nil {
		delete(annotations, "frozen-by")
		targetDeployment.Annotations = annotations
	}

	if err := r.Update(ctx, &targetDeployment); err != nil {
		logger.Error(err, "Failed to scale up Deployment to original replica count", "deploymentName", targetDeployment.Name)

		return ctrl.Result{}, err
	}

	logger.Info("Setting DeploymentFreezer status to completed", "name", deploymentFreezer.Name)

	currentTime := metav1.Now()
	deploymentFreezer.Status.CompletionTime = &currentTime

	if err := r.Status().Update(ctx, &deploymentFreezer); err != nil {
		logger.Error(err, "Failed to update DeploymentFreezer status as completed", "name", deploymentFreezer.Name)

		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func int32Ptr(i int32) *int32 {
	return &i
}

// SetupWithManager sets up the controller with the Manager.
func (r *DeploymentFreezerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&depappsv1.DeploymentFreezer{}).
		Named("deploymentfreezer").
		Complete(r)
}
