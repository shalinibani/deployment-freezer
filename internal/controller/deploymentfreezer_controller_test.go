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
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	depappsv1 "github.com/shalinibani/deployment-freezer/api/v1"
)

var (
	testScheme *runtime.Scheme
)

func TestMain(m *testing.M) {
	testScheme = runtime.NewScheme()
	_ = depappsv1.AddToScheme(testScheme)
	_ = appsv1.AddToScheme(testScheme)

	code := m.Run()
	os.Exit(code)
}

func setupTestReconciler() (*DeploymentFreezerReconciler, client.Client) {
	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithStatusSubresource(&depappsv1.DeploymentFreezer{}).
		Build()

	reconciler := &DeploymentFreezerReconciler{
		Client: fakeClient,
		Scheme: testScheme,
	}

	return reconciler, fakeClient
}

func TestValidateNoConflicts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		existingObject *depappsv1.DeploymentFreezer
		testFreezer    *depappsv1.DeploymentFreezer
		expectError    bool
	}{
		{
			name:           "noConflicts",
			existingObject: nil,
			testFreezer: &depappsv1.DeploymentFreezer{
				ObjectMeta: metav1.ObjectMeta{Name: "test-freezer", Namespace: "default"},
				Spec:       depappsv1.DeploymentFreezerSpec{DeploymentName: "test-deployment"},
			},
			expectError: false,
		},
		{
			name: "conflictDetected",
			existingObject: &depappsv1.DeploymentFreezer{
				ObjectMeta: metav1.ObjectMeta{Name: "existing-freezer", Namespace: "default"},
				Spec:       depappsv1.DeploymentFreezerSpec{DeploymentName: "test-deployment"},
				Status: depappsv1.DeploymentFreezerStatus{
					CreationTime: &metav1.Time{Time: time.Now()},
				},
			},
			testFreezer: &depappsv1.DeploymentFreezer{
				ObjectMeta: metav1.ObjectMeta{Name: "new-freezer", Namespace: "default"},
				Spec:       depappsv1.DeploymentFreezerSpec{DeploymentName: "test-deployment"},
			},
			expectError: true,
		},
		{
			name: "completedFreezerNoConflict",
			existingObject: &depappsv1.DeploymentFreezer{
				ObjectMeta: metav1.ObjectMeta{Name: "completed-freezer", Namespace: "default"},
				Spec:       depappsv1.DeploymentFreezerSpec{DeploymentName: "test-deployment"},
				Status: depappsv1.DeploymentFreezerStatus{
					CreationTime:   &metav1.Time{Time: time.Now()},
					CompletionTime: &metav1.Time{Time: time.Now()},
				},
			},
			testFreezer: &depappsv1.DeploymentFreezer{
				ObjectMeta: metav1.ObjectMeta{Name: "new-freezer", Namespace: "default"},
				Spec:       depappsv1.DeploymentFreezerSpec{DeploymentName: "test-deployment"},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := require.New(t)
			reconciler, fakeClient := setupTestReconciler()

			// Setup existing object if provided
			if tt.existingObject != nil {
				req.NoError(fakeClient.Create(context.Background(), tt.existingObject))
			}

			err := reconciler.validateNoConflicts(context.Background(), tt.testFreezer)

			if tt.expectError {
				req.Error(err)
				req.Contains(err.Error(), "conflict detected")
			} else {
				req.NoError(err)
			}
		})
	}
}

// TestFreezeDeployment tests the freezeDeployment method directly (unit test).
// Verifies deployment scaling to 0, annotation setting, and status updates.
func TestFreezeDeployment(t *testing.T) {
	t.Parallel()
	req := require.New(t)
	reconciler, fakeClient := setupTestReconciler()

	replicas := int32(3)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: "default"},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
	}

	freezer := depappsv1.DeploymentFreezer{
		ObjectMeta: metav1.ObjectMeta{Name: "test-freezer", Namespace: "default"},
		Spec: depappsv1.DeploymentFreezerSpec{
			DeploymentName:      "test-deployment",
			FreezeDurationInSec: 300,
		},
	}

	req.NoError(fakeClient.Create(context.Background(), deployment))
	req.NoError(fakeClient.Create(context.Background(), &freezer))

	result, err := reconciler.freezeDeployment(context.Background(), ctrl.Log, *deployment, freezer)

	req.NoError(err)
	req.Equal(time.Duration(300)*time.Second, result.RequeueAfter)

	// Verify deployment was scaled down
	var updatedDeployment appsv1.Deployment
	req.NoError(fakeClient.Get(context.Background(), client.ObjectKey{Name: "test-deployment", Namespace: "default"}, &updatedDeployment))
	req.Equal(int32(0), *updatedDeployment.Spec.Replicas)
	req.Equal("test-freezer", updatedDeployment.Annotations["frozen-by"])

	// Verify freezer status was updated
	var updatedFreezer depappsv1.DeploymentFreezer
	req.NoError(fakeClient.Get(context.Background(), client.ObjectKey{Name: "test-freezer", Namespace: "default"}, &updatedFreezer))
	req.NotNil(updatedFreezer.Status.CreationTime)
	req.NotNil(updatedFreezer.Status.PrevReplicaCount)
	req.Equal(int32(3), *updatedFreezer.Status.PrevReplicaCount)
	req.NotNil(updatedFreezer.Status.FreezeEndTime)
}

func TestUnfreezeDeployment(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		currentReplicas  int32
		freezeEndTime    time.Time
		expectRequeue    bool
		expectedReplicas int32
		expectCompletion bool
	}{
		{
			name:             "successfulUnfreeze",
			currentReplicas:  0,
			freezeEndTime:    time.Now().Add(-5 * time.Minute), // Past time
			expectRequeue:    false,
			expectedReplicas: 3,
			expectCompletion: true,
		},
		{
			name:             "freezeNotElapsed",
			currentReplicas:  0,
			freezeEndTime:    time.Now().Add(5 * time.Minute), // Future time
			expectRequeue:    true,
			expectedReplicas: 0,
			expectCompletion: false,
		},
		{
			name:             "manualChangesDetected",
			currentReplicas:  2,                                // Manually changed from 0
			freezeEndTime:    time.Now().Add(-5 * time.Minute), // Past time
			expectRequeue:    false,
			expectedReplicas: 3, // Should restore to original
			expectCompletion: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := require.New(t)
			reconciler, fakeClient := setupTestReconciler()

			prevReplicas := int32(3)
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "default",
					Annotations: map[string]string{
						"frozen-by": "test-freezer",
					},
				},
				Spec: appsv1.DeploymentSpec{Replicas: &tt.currentReplicas},
			}

			freezer := depappsv1.DeploymentFreezer{
				ObjectMeta: metav1.ObjectMeta{Name: "test-freezer", Namespace: "default"},
				Spec: depappsv1.DeploymentFreezerSpec{
					DeploymentName:      "test-deployment",
					FreezeDurationInSec: 300,
				},
				Status: depappsv1.DeploymentFreezerStatus{
					PrevReplicaCount: &prevReplicas,
					FreezeEndTime:    &metav1.Time{Time: tt.freezeEndTime},
				},
			}

			req.NoError(fakeClient.Create(context.Background(), deployment))
			req.NoError(fakeClient.Create(context.Background(), &freezer))

			result, err := reconciler.unfreezeDeployment(context.Background(), ctrl.Log, *deployment, freezer)

			req.NoError(err)

			if tt.expectRequeue {
				req.True(result.RequeueAfter > 0)
			} else {
				req.Equal(ctrl.Result{}, result)

				// Verify deployment was updated
				var updatedDeployment appsv1.Deployment
				req.NoError(fakeClient.Get(context.Background(), client.ObjectKey{Name: "test-deployment", Namespace: "default"}, &updatedDeployment))
				req.Equal(tt.expectedReplicas, *updatedDeployment.Spec.Replicas)

				if tt.expectCompletion {
					// Verify freezer was marked as completed
					var updatedFreezer depappsv1.DeploymentFreezer
					req.NoError(fakeClient.Get(context.Background(), client.ObjectKey{Name: "test-freezer", Namespace: "default"}, &updatedFreezer))
					req.NotNil(updatedFreezer.Status.CompletionTime)
				}
			}
		})
	}
}

func TestReconcileUnfreezeAfterDuration(t *testing.T) {
	t.Parallel()
	req := require.New(t)
	reconciler, fakeClient := setupTestReconciler()

	replicas := int32(0)
	prevReplicas := int32(3)
	pastTime := metav1.NewTime(time.Now().Add(-5 * time.Minute))

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "default",
			Annotations: map[string]string{
				"frozen-by": "test-freezer",
			},
		},
		Spec: appsv1.DeploymentSpec{Replicas: &replicas},
	}

	freezer := &depappsv1.DeploymentFreezer{
		ObjectMeta: metav1.ObjectMeta{Name: "test-freezer", Namespace: "default"},
		Spec: depappsv1.DeploymentFreezerSpec{
			DeploymentName:      "test-deployment",
			FreezeDurationInSec: 300,
		},
		Status: depappsv1.DeploymentFreezerStatus{
			CreationTime:     &metav1.Time{Time: time.Now().Add(-10 * time.Minute)},
			PrevReplicaCount: &prevReplicas,
			FreezeEndTime:    &pastTime,
		},
	}

	req.NoError(fakeClient.Create(context.Background(), deployment))
	req.NoError(fakeClient.Create(context.Background(), freezer))

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "test-freezer", Namespace: "default"},
	}

	result, err := reconciler.Reconcile(context.Background(), request)

	req.NoError(err)
	req.Equal(ctrl.Result{}, result)

	// Verify deployment was scaled back up
	var updatedDeployment appsv1.Deployment
	req.NoError(fakeClient.Get(context.Background(), client.ObjectKey{Name: "test-deployment", Namespace: "default"}, &updatedDeployment))
	req.Equal(int32(3), *updatedDeployment.Spec.Replicas)
	_, exists := updatedDeployment.Annotations["frozen-by"]
	req.False(exists)

	// Verify freezer was marked as completed
	var updatedFreezer depappsv1.DeploymentFreezer
	req.NoError(fakeClient.Get(context.Background(), client.ObjectKey{Name: "test-freezer", Namespace: "default"}, &updatedFreezer))
	req.NotNil(updatedFreezer.Status.CompletionTime)
}

func TestReconcileEdgeCases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		setupObject    client.Object
		request        reconcile.Request
		expectedResult ctrl.Result
		expectError    bool
		errorCheck     func(error) bool
	}{
		{
			name:        "deploymentFreezerNotFound",
			setupObject: nil,
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: "default"},
			},
			expectedResult: ctrl.Result{},
			expectError:    false,
		},
		{
			name: "alreadyCompleted",
			setupObject: &depappsv1.DeploymentFreezer{
				ObjectMeta: metav1.ObjectMeta{Name: "completed-freezer", Namespace: "default"},
				Status: depappsv1.DeploymentFreezerStatus{
					CompletionTime: &metav1.Time{Time: time.Now()},
				},
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "completed-freezer", Namespace: "default"},
			},
			expectedResult: ctrl.Result{},
			expectError:    false,
		},
		{
			name: "targetDeploymentNotFound",
			setupObject: &depappsv1.DeploymentFreezer{
				ObjectMeta: metav1.ObjectMeta{Name: "test-freezer", Namespace: "default"},
				Spec: depappsv1.DeploymentFreezerSpec{
					DeploymentName:      "nonexistent-deployment",
					FreezeDurationInSec: 300,
				},
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "test-freezer", Namespace: "default"},
			},
			expectedResult: ctrl.Result{},
			expectError:    true,
			errorCheck:     errors.IsNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := require.New(t)
			reconciler, fakeClient := setupTestReconciler()

			// Setup test object if provided
			if tt.setupObject != nil {
				req.NoError(fakeClient.Create(context.Background(), tt.setupObject))
			}

			result, err := reconciler.Reconcile(context.Background(), tt.request)

			if tt.expectError {
				req.Error(err)
				if tt.errorCheck != nil {
					req.True(tt.errorCheck(err))
				}
			} else {
				req.NoError(err)
			}

			req.Equal(tt.expectedResult, result)
		})
	}
}

func TestInt32Ptr(t *testing.T) {
	t.Parallel()
	req := require.New(t)
	value := int32(42)
	ptr := int32Ptr(value)

	req.NotNil(ptr)
	req.Equal(value, *ptr)
}
