package controller

import (
	"context"
	"testing"
	"time"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	koncachev1alpha1 "github.com/GreedyKomodoDragon/redis-operator/api/v1alpha1"
)

// TestModeIsolation ensures that HA and standalone features don't interfere with each other
func TestModeIsolation(t *testing.T) {
	testScheme := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(testScheme))
	require.NoError(t, koncachev1alpha1.AddToScheme(testScheme))
	require.NoError(t, monitoringv1.AddToScheme(testScheme))

	ctx := context.Background()

	t.Run("Standalone mode should not have leader annotations", func(t *testing.T) {
		// Create a standalone Redis instance
		redis := &koncachev1alpha1.Redis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-standalone",
				Namespace: "default",
			},
			Spec: koncachev1alpha1.RedisSpec{
				Image: "redis:7.2-alpine",
				HighAvailability: &koncachev1alpha1.RedisHighAvailability{
					Enabled: false, // Explicitly disabled
				},
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(testScheme).
			WithObjects(redis).
			Build()

		reconciler := &RedisReconciler{
			Client: fakeClient,
			Scheme: testScheme,
		}

		// Reconcile the Redis instance
		_, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "test-standalone",
				Namespace: "default",
			},
		})
		require.NoError(t, err)

		// Check that only one StatefulSet was created (not multiple HA StatefulSets)
		statefulSets := &appsv1.StatefulSetList{}
		err = fakeClient.List(ctx, statefulSets)
		require.NoError(t, err)
		assert.Len(t, statefulSets.Items, 1, "Standalone mode should create exactly one StatefulSet")

		// Check the StatefulSet name doesn't have HA suffix pattern
		sts := statefulSets.Items[0]
		assert.Equal(t, "test-standalone", sts.Name, "Standalone StatefulSet should not have HA naming pattern")

		// Wait for pods to be created and check they don't have leader annotations
		pods := &corev1.PodList{}
		err = fakeClient.List(ctx, pods)
		require.NoError(t, err)

		for _, pod := range pods.Items {
			annotations := pod.GetAnnotations()
			if annotations != nil {
				assert.NotContains(t, annotations, redisRoleAnnotation,
					"Standalone pods should not have leader role annotations")
			}
		}
	})

	t.Run("HA mode should create multiple StatefulSets with leader election", func(t *testing.T) {
		// Create an HA Redis instance
		redis := &koncachev1alpha1.Redis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ha",
				Namespace: "default",
			},
			Spec: koncachev1alpha1.RedisSpec{
				Image: "redis:7.2-alpine",
				HighAvailability: &koncachev1alpha1.RedisHighAvailability{
					Enabled:  true,
					Replicas: 3,
				},
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(testScheme).
			WithObjects(redis).
			Build()

		reconciler := &RedisReconciler{
			Client: fakeClient,
			Scheme: testScheme,
		}

		// Reconcile the Redis instance
		_, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "test-ha",
				Namespace: "default",
			},
		})
		require.NoError(t, err)

		// Check that multiple StatefulSets were created for HA
		statefulSets := &appsv1.StatefulSetList{}
		err = fakeClient.List(ctx, statefulSets)
		require.NoError(t, err)
		assert.Equal(t, 3, len(statefulSets.Items), "HA mode should create multiple StatefulSets")

		// Check StatefulSet naming pattern follows HA convention
		expectedNames := []string{"test-ha-ha-1", "test-ha-ha-2", "test-ha-ha-3"}
		actualNames := make([]string, len(statefulSets.Items))
		for i, sts := range statefulSets.Items {
			actualNames[i] = sts.Name
		}
		assert.ElementsMatch(t, expectedNames, actualNames, "HA StatefulSet should follow naming pattern")
	})

	t.Run("Switching from standalone to HA should clean up properly", func(t *testing.T) {
		// Start with standalone
		redis := &koncachev1alpha1.Redis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-switch",
				Namespace: "default",
			},
			Spec: koncachev1alpha1.RedisSpec{
				Image: "redis:7.2-alpine",
				HighAvailability: &koncachev1alpha1.RedisHighAvailability{
					Enabled: false,
				},
			},
		}

		// Create standalone StatefulSet manually to simulate existing state
		standaloneStatefulSet := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-switch",
				Namespace: "default",
				Labels: map[string]string{
					"app.kubernetes.io/name":      "redis",
					"app.kubernetes.io/instance":  "test-switch",
					"app.kubernetes.io/component": "database",
				},
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas: int32Ptr(1),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app.kubernetes.io/name":     "redis",
						"app.kubernetes.io/instance": "test-switch",
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app.kubernetes.io/name":     "redis",
							"app.kubernetes.io/instance": "test-switch",
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name:  "redis",
							Image: "redis:7.2-alpine",
						}},
					},
				},
			},
		}

		// Add OwnerReference to the StatefulSet
		standaloneStatefulSet.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: "koncache.greedykomodo/v1alpha1",
				Kind:       "Redis",
				Name:       "test-switch",
				UID:        "test-uid",
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(testScheme).
			WithObjects(redis, standaloneStatefulSet).
			Build()

		// Test the cleanup logic directly instead of full reconciliation
		standaloneController := NewStandaloneController(fakeClient, testScheme)

		// Update Redis to HA mode
		redis.Spec.HighAvailability.Enabled = true
		redis.Spec.HighAvailability.Replicas = 2

		// Generate expected StatefulSets for HA mode
		expectedStatefulSets := standaloneController.statefulsSetForRedis(redis, "", "")
		assert.Len(t, expectedStatefulSets, 2, "Should generate 2 HA StatefulSets")

		// Test cleanup of orphaned standalone StatefulSet
		err := standaloneController.cleanupOrphanedStatefulSets(ctx, redis, expectedStatefulSets)
		require.NoError(t, err)

		// Verify standalone StatefulSet was cleaned up
		standaloneExists := &appsv1.StatefulSet{}
		err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-switch", Namespace: "default"}, standaloneExists)
		assert.Error(t, err, "Standalone StatefulSet should be cleaned up when switching to HA")
	})

	t.Run("Switching from HA to standalone should clean up HA StatefulSets", func(t *testing.T) {
		// Create a Redis instance in standalone mode
		redis := &koncachev1alpha1.Redis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-switch-back",
				Namespace: "default",
			},
			Spec: koncachev1alpha1.RedisSpec{
				Image: "redis:7.2-alpine",
				HighAvailability: &koncachev1alpha1.RedisHighAvailability{
					Enabled: false, // Now in standalone mode
				},
			},
		}

		// Create pre-existing HA StatefulSets (simulating prior HA state)
		haStatefulSets := []*appsv1.StatefulSet{
			createHAStatefulSetForRedis("test-switch-back-ha-1", "default", "test-switch-back"),
			createHAStatefulSetForRedis("test-switch-back-ha-2", "default", "test-switch-back"),
		}

		clientBuilder := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(redis)
		for _, sts := range haStatefulSets {
			clientBuilder = clientBuilder.WithObjects(sts)
		}
		fakeClient := clientBuilder.Build()

		standaloneController := NewStandaloneController(fakeClient, testScheme)

		// Generate expected StatefulSets for standalone mode (should be 1)
		expectedStatefulSets := standaloneController.statefulsSetForRedis(redis, "", "")
		assert.Len(t, expectedStatefulSets, 1, "Should generate 1 standalone StatefulSet")

		// Test cleanup of orphaned HA StatefulSets
		err := standaloneController.cleanupOrphanedStatefulSets(ctx, redis, expectedStatefulSets)
		require.NoError(t, err)

		// Verify HA StatefulSets were cleaned up
		haStatefulSet1 := &appsv1.StatefulSet{}
		err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-switch-back-ha-1", Namespace: "default"}, haStatefulSet1)
		assert.Error(t, err, "HA StatefulSet 1 should be cleaned up when switching to standalone")

		haStatefulSet2 := &appsv1.StatefulSet{}
		err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-switch-back-ha-2", Namespace: "default"}, haStatefulSet2)
		assert.Error(t, err, "HA StatefulSet 2 should be cleaned up when switching to standalone")
	})

	t.Run("Leader failover should only affect HA mode", func(t *testing.T) {
		// Create both standalone and HA Redis instances
		standaloneRedis := &koncachev1alpha1.Redis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "standalone-redis",
				Namespace: "default",
			},
			Spec: koncachev1alpha1.RedisSpec{
				Image: "redis:7.2-alpine",
				HighAvailability: &koncachev1alpha1.RedisHighAvailability{
					Enabled: false,
				},
			},
		}

		haRedis := &koncachev1alpha1.Redis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ha-redis",
				Namespace: "default",
			},
			Spec: koncachev1alpha1.RedisSpec{
				Image: "redis:7.2-alpine",
				HighAvailability: &koncachev1alpha1.RedisHighAvailability{
					Enabled:  true,
					Replicas: 2,
				},
			},
		}

		// Create test pods for both modes
		standalonePod := createTestPodForRedis("standalone-redis-0", "default", "standalone-redis", false, nil)
		haPods := []*corev1.Pod{
			createTestPodWithStatefulSetForRedis("ha-redis-ha-1-0", "default", "ha-redis", true, "ha-redis-ha-1", map[string]string{redisRoleAnnotation: redisLeaderRole}),
			createTestPodWithStatefulSetForRedis("ha-redis-ha-2-0", "default", "ha-redis", true, "ha-redis-ha-2", nil),
		}

		clientBuilder := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(standaloneRedis, haRedis, standalonePod)
		for _, pod := range haPods {
			clientBuilder = clientBuilder.WithObjects(pod)
		}
		fakeClient := clientBuilder.Build()

		reconciler := &RedisReconciler{
			Client: fakeClient,
			Scheme: testScheme,
		}

		// Simulate leader pod failure in HA mode
		// Get the current leader pod from the client
		failedHAPod := &corev1.Pod{}
		err := fakeClient.Get(ctx, types.NamespacedName{Name: "ha-redis-ha-1-0", Namespace: "default"}, failedHAPod)
		require.NoError(t, err)

		// Handle leader failover
		err = reconciler.handleLeaderFailover(ctx, failedHAPod, haRedis)
		require.NoError(t, err)

		// Verify that standalone pod is unaffected
		updatedStandalonePod := &corev1.Pod{}
		err = fakeClient.Get(ctx, types.NamespacedName{Name: "standalone-redis-0", Namespace: "default"}, updatedStandalonePod)
		require.NoError(t, err)

		annotations := updatedStandalonePod.GetAnnotations()
		if annotations != nil {
			assert.NotContains(t, annotations, redisRoleAnnotation,
				"Standalone pod should not get leader annotations during HA failover")
		}

		// Verify that a new leader was promoted in HA mode
		updatedHAPod := &corev1.Pod{}
		err = fakeClient.Get(ctx, types.NamespacedName{Name: "ha-redis-ha-2-0", Namespace: "default"}, updatedHAPod)
		require.NoError(t, err)

		annotations = updatedHAPod.GetAnnotations()
		assert.Contains(t, annotations, redisRoleAnnotation, "HA pod should be promoted to leader")
		assert.Equal(t, redisLeaderRole, annotations[redisRoleAnnotation])
	})

	t.Run("Orphaned StatefulSet cleanup should only affect HA mode", func(t *testing.T) {
		// Create both standalone and HA Redis instances
		standaloneRedis := &koncachev1alpha1.Redis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "standalone-cleanup",
				Namespace: "default",
			},
			Spec: koncachev1alpha1.RedisSpec{
				Image: "redis:7.2-alpine",
				HighAvailability: &koncachev1alpha1.RedisHighAvailability{
					Enabled: false,
				},
			},
		}

		haRedis := &koncachev1alpha1.Redis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ha-cleanup",
				Namespace: "default",
			},
			Spec: koncachev1alpha1.RedisSpec{
				Image: "redis:7.2-alpine",
				HighAvailability: &koncachev1alpha1.RedisHighAvailability{
					Enabled:  true,
					Replicas: 2, // Reducing from 3 to 2
				},
			},
		}

		// Create StatefulSets
		standaloneStatefulSet := createStandaloneStatefulSet("standalone-cleanup", "default")
		haStatefulSets := []*appsv1.StatefulSet{
			createHAStatefulSet("ha-cleanup-ha-1", "default"),
			createHAStatefulSet("ha-cleanup-ha-2", "default"),
			createHAStatefulSet("ha-cleanup-ha-3", "default"), // This should be cleaned up
		}

		clientBuilder := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(standaloneRedis, haRedis, standaloneStatefulSet)
		for _, sts := range haStatefulSets {
			clientBuilder = clientBuilder.WithObjects(sts)
		}
		fakeClient := clientBuilder.Build()

		standaloneController := NewStandaloneController(fakeClient, testScheme)

		// Create expected StatefulSets for HA mode (should be 2, not 3)
		expectedStatefulSets := standaloneController.statefulsSetForRedis(haRedis, "", "")

		// Debug: print expected StatefulSet names
		expectedNames := make([]string, len(expectedStatefulSets))
		for i, sts := range expectedStatefulSets {
			expectedNames[i] = sts.Name
		}
		t.Logf("Expected StatefulSets for HA Redis: %v", expectedNames)

		// Test cleanup for HA Redis instance
		err := standaloneController.cleanupOrphanedStatefulSets(ctx, haRedis, expectedStatefulSets)
		require.NoError(t, err)

		// Verify standalone StatefulSet is untouched
		standaloneExists := &appsv1.StatefulSet{}
		err = fakeClient.Get(ctx, types.NamespacedName{Name: "standalone-cleanup", Namespace: "default"}, standaloneExists)
		require.NoError(t, err, "Standalone StatefulSet should not be affected by HA cleanup")

		// Verify orphaned HA StatefulSet was cleaned up
		orphanedSts := &appsv1.StatefulSet{}
		err = fakeClient.Get(ctx, types.NamespacedName{Name: "ha-cleanup-ha-3", Namespace: "default"}, orphanedSts)
		assert.Error(t, err, "Orphaned HA StatefulSet should be cleaned up")

		// Verify required HA StatefulSets still exist
		requiredSts := &appsv1.StatefulSet{}
		err = fakeClient.Get(ctx, types.NamespacedName{Name: "ha-cleanup-ha-1", Namespace: "default"}, requiredSts)
		require.NoError(t, err, "Required HA StatefulSet should still exist")

		err = fakeClient.Get(ctx, types.NamespacedName{Name: "ha-cleanup-ha-2", Namespace: "default"}, requiredSts)
		require.NoError(t, err, "Required HA StatefulSet should still exist")
	})
}

// Helper functions
func createHAStatefulSet(name, namespace string) *appsv1.StatefulSet {
	return createHAStatefulSetForRedis(name, namespace, "ha-cleanup")
}

func createHAStatefulSetForRedis(name, namespace, redisName string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "redis",
				"app.kubernetes.io/instance":  redisName,
				"app.kubernetes.io/component": "database",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "koncache.greedykomodo/v1alpha1",
					Kind:       "Redis",
					Name:       redisName,
					UID:        "test-uid", // This would normally be set by the controller
				},
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name":     "redis",
					"app.kubernetes.io/instance": redisName,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name":     "redis",
						"app.kubernetes.io/instance": redisName,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "redis",
						Image: "redis:7.2-alpine",
					}},
				},
			},
		},
	}
}

func createStandaloneStatefulSet(name, namespace string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "redis",
				"app.kubernetes.io/instance":  name,
				"app.kubernetes.io/component": "database",
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name":     "redis",
					"app.kubernetes.io/instance": name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name":     "redis",
						"app.kubernetes.io/instance": name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "redis",
						Image: "redis:7.2-alpine",
					}},
				},
			},
		},
	}
}

func int32Ptr(i int32) *int32 {
	return &i
}

// Helper function to create pods with correct labels for a specific Redis instance
func createTestPodForRedis(name, namespace, redisName string, healthy bool, annotations map[string]string) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":     "redis",
				"app.kubernetes.io/instance": redisName,
			},
			Annotations: annotations,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "redis",
					Image: "redis:7.2-alpine",
				},
			},
		},
	}

	if healthy {
		pod.Status = corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  "redis",
					Ready: true,
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{
							StartedAt: metav1.NewTime(time.Now()),
						},
					},
				},
			},
		}
	} else {
		pod.Status = corev1.PodStatus{
			Phase: corev1.PodFailed,
		}
	}

	return pod
}

func createTestPodWithStatefulSetForRedis(name, namespace, redisName string, healthy bool, statefulSetName string, annotations map[string]string) *corev1.Pod {
	pod := createTestPodForRedis(name, namespace, redisName, healthy, annotations)
	pod.OwnerReferences = []metav1.OwnerReference{
		{
			Kind: "StatefulSet",
			Name: statefulSetName,
		},
	}
	return pod
}
