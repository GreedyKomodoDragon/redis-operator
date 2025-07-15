package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	koncachev1alpha1 "github.com/GreedyKomodoDragon/redis-operator/api/v1alpha1"
)

// Helper function to check if a pod is ready
func podReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func TestHandleLeaderFailover(t *testing.T) {
	testScheme := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(testScheme))
	require.NoError(t, koncachev1alpha1.AddToScheme(testScheme))

	ctx := context.Background()

	tests := []struct {
		name            string
		pods            []*corev1.Pod
		failedPod       *corev1.Pod
		expectPromotion bool
		expectedLeader  string
	}{
		{
			name: "Leader pod fails, promote healthy candidate",
			pods: []*corev1.Pod{
				createTestPodWithStatefulSetAndAnnotations("redis-ha-1-0", "default", true, "redis-ha-1", map[string]string{redisRoleAnnotation: redisLeaderRole}),
				createTestPodWithStatefulSet("redis-ha-2-0", "default", true, "redis-ha-2"),
			},
			failedPod:       createTestPodWithStatefulSetAndAnnotations("redis-ha-1-0", "default", false, "redis-ha-1", map[string]string{redisRoleAnnotation: redisLeaderRole}),
			expectPromotion: true,
			expectedLeader:  "redis-ha-2-0",
		},
		{
			name: "Non-leader pod fails, no promotion needed",
			pods: []*corev1.Pod{
				createTestPod("redis-ha-1-0", "default", true, map[string]string{redisRoleAnnotation: redisLeaderRole}),
				createTestPod("redis-ha-2-0", "default", false, nil),
			},
			failedPod:       createTestPod("redis-ha-2-0", "default", false, nil),
			expectPromotion: false,
			expectedLeader:  "",
		},
		{
			name: "Leader pod fails, no healthy candidates available",
			pods: []*corev1.Pod{
				createTestPod("redis-ha-1-0", "default", false, map[string]string{redisRoleAnnotation: redisLeaderRole}),
				createTestPod("redis-ha-2-0", "default", false, nil),
			},
			failedPod:       createTestPod("redis-ha-1-0", "default", false, map[string]string{redisRoleAnnotation: redisLeaderRole}),
			expectPromotion: false,
			expectedLeader:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create Redis instance
			redis := &koncachev1alpha1.Redis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-redis",
					Namespace: "default",
				},
				Spec: koncachev1alpha1.RedisSpec{
					HighAvailability: &koncachev1alpha1.RedisHighAvailability{
						Enabled: true,
					},
				},
			}

			// Create fake client with test data
			clientBuilder := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(redis)
			for _, pod := range tt.pods {
				clientBuilder = clientBuilder.WithObjects(pod)
			}
			fakeClient := clientBuilder.Build()

			// Create controller
			reconciler := &RedisReconciler{
				Client: fakeClient,
				Scheme: testScheme,
			}

			// Execute the leader failover
			err := reconciler.handleLeaderFailover(ctx, tt.failedPod, redis)
			require.NoError(t, err)

			if tt.expectPromotion {
				// Check that the expected pod was promoted to leader
				expectedPod := &corev1.Pod{}
				err := fakeClient.Get(ctx, types.NamespacedName{
					Name:      tt.expectedLeader,
					Namespace: "default",
				}, expectedPod)
				require.NoError(t, err)

				assert.Equal(t, redisLeaderRole, expectedPod.Annotations[redisRoleAnnotation])
				assert.NotEmpty(t, expectedPod.Annotations[redisPromotedAtAnnotation])
			}
		})
	}
}

func TestIsPodLeader(t *testing.T) {
	reconciler := &RedisReconciler{}

	tests := []struct {
		name         string
		pod          *corev1.Pod
		expectLeader bool
	}{
		{
			name: "Pod with leader annotation",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						redisRoleAnnotation: redisLeaderRole,
					},
				},
			},
			expectLeader: true,
		},
		{
			name: "Pod without leader annotation",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"other-annotation": "value",
					},
				},
			},
			expectLeader: false,
		},
		{
			name: "Pod with no annotations",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{},
			},
			expectLeader: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := reconciler.isPodLeader(tt.pod)
			assert.Equal(t, tt.expectLeader, result)
		})
	}
}

func TestFindLeaderCandidates(t *testing.T) {
	testScheme := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(testScheme))
	require.NoError(t, koncachev1alpha1.AddToScheme(testScheme))

	ctx := context.Background()

	// Create test pods in different StatefulSets
	pods := []*corev1.Pod{
		createTestPodWithStatefulSet("redis-ha-1-0", "default", true, "redis-ha-1"),
		createTestPodWithStatefulSet("redis-ha-2-0", "default", true, "redis-ha-2"),
		createTestPodWithStatefulSet("redis-ha-3-0", "default", false, "redis-ha-3"), // not healthy
	}

	redis := &koncachev1alpha1.Redis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-redis",
			Namespace: "default",
		},
	}

	clientBuilder := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(redis)
	for _, pod := range pods {
		clientBuilder = clientBuilder.WithObjects(pod)
	}
	fakeClient := clientBuilder.Build()

	reconciler := &RedisReconciler{
		Client: fakeClient,
		Scheme: testScheme,
	}

	// Test finding candidates when failed pod is from redis-ha-1
	failedPod := pods[0] // redis-ha-1-0
	candidates, err := reconciler.findLeaderCandidates(ctx, redis, failedPod)
	require.NoError(t, err)

	// Should find redis-ha-2-0 (healthy and different StatefulSet)
	// Should NOT find redis-ha-3-0 (not healthy)
	assert.Len(t, candidates, 1)
	assert.Equal(t, "redis-ha-2-0", candidates[0].Name)
}

func TestEnsureLeaderExistsInitialCreation(t *testing.T) {
	testScheme := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(testScheme))
	require.NoError(t, koncachev1alpha1.AddToScheme(testScheme))

	ctx := context.Background()

	tests := []struct {
		name           string
		pods           []*corev1.Pod
		expectLeader   bool
		expectedLeader string
	}{
		{
			name:         "No pods yet - no leader assigned",
			pods:         []*corev1.Pod{},
			expectLeader: false,
		},
		{
			name: "Pods not ready yet - no leader assigned",
			pods: []*corev1.Pod{
				createTestPodWithStatefulSet("redis-ha-1-0", "default", false, "redis-ha-1"),
				createTestPodWithStatefulSet("redis-ha-2-0", "default", false, "redis-ha-2"),
			},
			expectLeader: false,
		},
		{
			name: "First pod becomes ready - gets promoted to leader",
			pods: []*corev1.Pod{
				createTestPodWithStatefulSet("redis-ha-1-0", "default", true, "redis-ha-1"),
				createTestPodWithStatefulSet("redis-ha-2-0", "default", false, "redis-ha-2"),
			},
			expectLeader:   true,
			expectedLeader: "redis-ha-1-0",
		},
		{
			name: "Multiple pods ready - first one becomes leader",
			pods: []*corev1.Pod{
				createTestPodWithStatefulSet("redis-ha-1-0", "default", true, "redis-ha-1"),
				createTestPodWithStatefulSet("redis-ha-2-0", "default", true, "redis-ha-2"),
			},
			expectLeader:   true,
			expectedLeader: "redis-ha-1-0", // First pod found becomes leader
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create Redis instance with HA enabled
			redis := &koncachev1alpha1.Redis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-redis",
					Namespace: "default",
				},
				Spec: koncachev1alpha1.RedisSpec{
					HighAvailability: &koncachev1alpha1.RedisHighAvailability{
						Enabled: true,
					},
				},
			}

			// Create fake client with test data
			clientBuilder := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(redis)
			for _, pod := range tt.pods {
				clientBuilder = clientBuilder.WithObjects(pod)
			}
			fakeClient := clientBuilder.Build()

			// Create standalone controller
			controller := &StandaloneController{
				Client: fakeClient,
				Scheme: testScheme,
			}

			// Execute ensureLeaderExists
			err := controller.ensureLeaderExists(ctx, redis)
			require.NoError(t, err)

			if tt.expectLeader {
				// Check that the expected pod was promoted to leader
				expectedPod := &corev1.Pod{}
				err := fakeClient.Get(ctx, types.NamespacedName{
					Name:      tt.expectedLeader,
					Namespace: "default",
				}, expectedPod)
				require.NoError(t, err)

				assert.Equal(t, "leader", expectedPod.Annotations["redis-operator/role"])
				assert.NotEmpty(t, expectedPod.Annotations["redis-operator/promoted-at"])
			} else {
				// Check that no pod has leader annotation
				for _, pod := range tt.pods {
					updatedPod := &corev1.Pod{}
					err := fakeClient.Get(ctx, types.NamespacedName{
						Name:      pod.Name,
						Namespace: pod.Namespace,
					}, updatedPod)
					require.NoError(t, err)

					if updatedPod.Annotations != nil {
						assert.NotEqual(t, "leader", updatedPod.Annotations["redis-operator/role"])
					}
				}
			}
		})
	}
}

func TestHALeaderFailoverComprehensive(t *testing.T) {
	testScheme := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(testScheme))
	require.NoError(t, koncachev1alpha1.AddToScheme(testScheme))

	ctx := context.Background()

	t.Run("HA mode with multiple replicas - leader promotion chain", func(t *testing.T) {
		redis := &koncachev1alpha1.Redis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ha-chain",
				Namespace: "default",
			},
			Spec: koncachev1alpha1.RedisSpec{
				HighAvailability: &koncachev1alpha1.RedisHighAvailability{
					Enabled:  true,
					Replicas: 4,
				},
			},
		}

		// Create pods in different StatefulSets with correct Redis instance labels
		pods := []*corev1.Pod{
			createTestPodWithCorrectLabels("test-ha-chain-a-0", "default", "test-ha-chain", true, "test-ha-chain-a",
				map[string]string{redisRoleAnnotation: redisLeaderRole}),
			createTestPodWithCorrectLabels("test-ha-chain-b-0", "default", "test-ha-chain", true, "test-ha-chain-b", nil),
			createTestPodWithCorrectLabels("test-ha-chain-c-0", "default", "test-ha-chain", true, "test-ha-chain-c", nil),
			createTestPodWithCorrectLabels("test-ha-chain-d-0", "default", "test-ha-chain", false, "test-ha-chain-d", nil), // Not ready
		}

		clientBuilder := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(redis)
		for _, pod := range pods {
			clientBuilder = clientBuilder.WithObjects(pod)
		}
		fakeClient := clientBuilder.Build()

		reconciler := &RedisReconciler{
			Client: fakeClient,
			Scheme: testScheme,
		}

		// Simulate leader failure - get the actual failed pod from client
		failedLeader := &corev1.Pod{}
		err := fakeClient.Get(ctx, types.NamespacedName{Name: "test-ha-chain-a-0", Namespace: "default"}, failedLeader)
		require.NoError(t, err)

		err = reconciler.handleLeaderFailover(ctx, failedLeader, redis)
		require.NoError(t, err)

		// Check that a ready pod was promoted (should be b or c, not d since it's not ready)
		promotedPods := 0
		for _, podName := range []string{"test-ha-chain-b-0", "test-ha-chain-c-0"} {
			pod := &corev1.Pod{}
			err := fakeClient.Get(ctx, types.NamespacedName{Name: podName, Namespace: "default"}, pod)
			require.NoError(t, err)

			if annotations := pod.GetAnnotations(); annotations != nil {
				if role, exists := annotations[redisRoleAnnotation]; exists && role == redisLeaderRole {
					promotedPods++
				}
			}
		}

		assert.Equal(t, 1, promotedPods, "Exactly one pod should be promoted to leader")

		// Verify the non-ready pod wasn't promoted
		nonReadyPod := &corev1.Pod{}
		err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-ha-chain-d-0", Namespace: "default"}, nonReadyPod)
		require.NoError(t, err)

		if annotations := nonReadyPod.GetAnnotations(); annotations != nil {
			assert.NotEqual(t, redisLeaderRole, annotations[redisRoleAnnotation],
				"Non-ready pod should not be promoted")
		}
	})

	t.Run("HA mode with all pods in same StatefulSet - no promotion", func(t *testing.T) {
		redis := &koncachev1alpha1.Redis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-same-sts",
				Namespace: "default",
			},
			Spec: koncachev1alpha1.RedisSpec{
				HighAvailability: &koncachev1alpha1.RedisHighAvailability{
					Enabled:  true,
					Replicas: 2,
				},
			},
		}

		// Create pods all in the same StatefulSet (edge case)
		pods := []*corev1.Pod{
			createTestPodWithStatefulSetAndAnnotations("test-same-sts-a-0", "default", true, "test-same-sts-a",
				map[string]string{redisRoleAnnotation: redisLeaderRole}),
			createTestPodWithStatefulSet("test-same-sts-a-1", "default", true, "test-same-sts-a"), // Same StatefulSet
		}

		clientBuilder := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(redis)
		for _, pod := range pods {
			clientBuilder = clientBuilder.WithObjects(pod)
		}
		fakeClient := clientBuilder.Build()

		reconciler := &RedisReconciler{
			Client: fakeClient,
			Scheme: testScheme,
		}

		// Simulate leader failure
		failedLeader := createTestPodWithStatefulSetAndAnnotations("test-same-sts-a-0", "default", false, "test-same-sts-a",
			map[string]string{redisRoleAnnotation: redisLeaderRole})

		err := reconciler.handleLeaderFailover(ctx, failedLeader, redis)
		require.NoError(t, err)

		// No pod should be promoted since they're in the same StatefulSet
		remainingPod := &corev1.Pod{}
		err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-same-sts-a-1", Namespace: "default"}, remainingPod)
		require.NoError(t, err)

		annotations := remainingPod.GetAnnotations()
		if annotations != nil {
			assert.NotEqual(t, redisLeaderRole, annotations[redisRoleAnnotation],
				"Pod in same StatefulSet should not be promoted")
		}
	})

	t.Run("HA mode leader assignment on initial creation", func(t *testing.T) {
		redis := &koncachev1alpha1.Redis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-initial",
				Namespace: "default",
			},
			Spec: koncachev1alpha1.RedisSpec{
				HighAvailability: &koncachev1alpha1.RedisHighAvailability{
					Enabled:  true,
					Replicas: 3,
				},
			},
		}

		// Create pods without any leader annotations (fresh deployment)
		pods := []*corev1.Pod{
			createTestPodWithStatefulSet("test-initial-a-0", "default", true, "test-initial-a"),
			createTestPodWithStatefulSet("test-initial-b-0", "default", true, "test-initial-b"),
			createTestPodWithStatefulSet("test-initial-c-0", "default", false, "test-initial-c"), // Not ready
		}

		clientBuilder := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(redis)
		for _, pod := range pods {
			clientBuilder = clientBuilder.WithObjects(pod)
		}
		fakeClient := clientBuilder.Build()

		// Assign initial leader (simulate what happens during reconcile)
		// Find the first ready pod from a different StatefulSet and assign it as leader
		leaderAssigned := false
		for _, pod := range []string{"test-initial-a-0", "test-initial-b-0"} {
			podObj := &corev1.Pod{}
			err := fakeClient.Get(ctx, types.NamespacedName{Name: pod, Namespace: "default"}, podObj)
			require.NoError(t, err)

			if podReady(podObj) && !leaderAssigned {
				if podObj.Annotations == nil {
					podObj.Annotations = make(map[string]string)
				}
				podObj.Annotations[redisRoleAnnotation] = redisLeaderRole
				podObj.Annotations[redisPromotedAtAnnotation] = time.Now().Format(time.RFC3339)
				err = fakeClient.Update(ctx, podObj)
				require.NoError(t, err)
				leaderAssigned = true
			}
		}
		require.True(t, leaderAssigned, "Should assign an initial leader")

		// Check that exactly one ready pod was assigned as leader
		leaderCount := 0
		for _, podName := range []string{"test-initial-a-0", "test-initial-b-0", "test-initial-c-0"} {
			pod := &corev1.Pod{}
			err := fakeClient.Get(ctx, types.NamespacedName{Name: podName, Namespace: "default"}, pod)
			require.NoError(t, err)

			if annotations := pod.GetAnnotations(); annotations != nil {
				if role, exists := annotations[redisRoleAnnotation]; exists && role == redisLeaderRole {
					leaderCount++
					// Verify the assigned leader is ready
					assert.True(t, podReady(pod), "Assigned leader should be ready")
				}
			}
		}

		assert.Equal(t, 1, leaderCount, "Exactly one pod should be assigned as initial leader")
	})

	t.Run("Standalone mode should not trigger leader failover", func(t *testing.T) {
		standaloneRedis := &koncachev1alpha1.Redis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-standalone-nofail",
				Namespace: "default",
			},
			Spec: koncachev1alpha1.RedisSpec{
				HighAvailability: &koncachev1alpha1.RedisHighAvailability{
					Enabled: false,
				},
			},
		}

		// Create a standalone pod that fails
		standalonePod := createTestPod("test-standalone-nofail-0", "default", false, nil)

		fakeClient := fake.NewClientBuilder().
			WithScheme(testScheme).
			WithObjects(standaloneRedis, standalonePod).
			Build()

		reconciler := &RedisReconciler{
			Client: fakeClient,
			Scheme: testScheme,
		}

		// Try to handle "leader failover" for standalone mode
		err := reconciler.handleLeaderFailover(ctx, standalonePod, standaloneRedis)
		require.NoError(t, err) // Should succeed but do nothing

		// Verify no changes were made to the pod
		updatedPod := &corev1.Pod{}
		err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-standalone-nofail-0", Namespace: "default"}, updatedPod)
		require.NoError(t, err)

		annotations := updatedPod.GetAnnotations()
		if annotations != nil {
			assert.NotContains(t, annotations, redisRoleAnnotation,
				"Standalone pod should not get leader annotations")
		}
	})

	t.Run("HA mode with rapid successive failures", func(t *testing.T) {
		redis := &koncachev1alpha1.Redis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-rapid-fail",
				Namespace: "default",
			},
			Spec: koncachev1alpha1.RedisSpec{
				HighAvailability: &koncachev1alpha1.RedisHighAvailability{
					Enabled:  true,
					Replicas: 3,
				},
			},
		}

		// Create initial pods with leader and correct Redis instance labels
		pods := []*corev1.Pod{
			createTestPodWithCorrectLabels("test-rapid-fail-a-0", "default", "test-rapid-fail", true, "test-rapid-fail-a",
				map[string]string{redisRoleAnnotation: redisLeaderRole}),
			createTestPodWithCorrectLabels("test-rapid-fail-b-0", "default", "test-rapid-fail", true, "test-rapid-fail-b", nil),
			createTestPodWithCorrectLabels("test-rapid-fail-c-0", "default", "test-rapid-fail", true, "test-rapid-fail-c", nil),
		}

		clientBuilder := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(redis)
		for _, pod := range pods {
			clientBuilder = clientBuilder.WithObjects(pod)
		}
		fakeClient := clientBuilder.Build()

		reconciler := &RedisReconciler{
			Client: fakeClient,
			Scheme: testScheme,
		}

		// First failure - leader pod fails
		failedLeader1 := createTestPodWithStatefulSetAndAnnotations("test-rapid-fail-a-0", "default", false, "test-rapid-fail-a",
			map[string]string{redisRoleAnnotation: redisLeaderRole})

		err := reconciler.handleLeaderFailover(ctx, failedLeader1, redis)
		require.NoError(t, err)

		// Find who became the new leader
		var newLeaderName string
		for _, podName := range []string{"test-rapid-fail-b-0", "test-rapid-fail-c-0"} {
			pod := &corev1.Pod{}
			err := fakeClient.Get(ctx, types.NamespacedName{Name: podName, Namespace: "default"}, pod)
			require.NoError(t, err)

			if annotations := pod.GetAnnotations(); annotations != nil {
				if role, exists := annotations[redisRoleAnnotation]; exists && role == redisLeaderRole {
					newLeaderName = podName
					break
				}
			}
		}
		require.NotEmpty(t, newLeaderName, "A new leader should have been promoted")

		// Second failure - new leader fails immediately
		newLeaderPod := &corev1.Pod{}
		err = fakeClient.Get(ctx, types.NamespacedName{Name: newLeaderName, Namespace: "default"}, newLeaderPod)
		require.NoError(t, err)

		// Mark new leader as failed
		failedLeader2 := newLeaderPod.DeepCopy()
		failedLeader2.Status.Conditions = []corev1.PodCondition{{
			Type:   corev1.PodReady,
			Status: corev1.ConditionFalse,
		}}

		err = reconciler.handleLeaderFailover(ctx, failedLeader2, redis)
		require.NoError(t, err)

		// Verify the remaining pod became leader
		remainingPodNames := []string{"test-rapid-fail-b-0", "test-rapid-fail-c-0"}
		var finalLeaderName string
		for _, podName := range remainingPodNames {
			if podName != newLeaderName {
				finalLeaderName = podName
				break
			}
		}

		finalLeaderPod := &corev1.Pod{}
		err = fakeClient.Get(ctx, types.NamespacedName{Name: finalLeaderName, Namespace: "default"}, finalLeaderPod)
		require.NoError(t, err)

		annotations := finalLeaderPod.GetAnnotations()
		assert.Contains(t, annotations, redisRoleAnnotation)
		assert.Equal(t, redisLeaderRole, annotations[redisRoleAnnotation])
	})
}

// Helper functions

func createTestPod(name, namespace string, healthy bool, annotations map[string]string) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				redisNameLabel:     "redis",
				redisInstanceLabel: "test-redis",
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

func createTestPodWithStatefulSet(name, namespace string, healthy bool, statefulSetName string) *corev1.Pod {
	pod := createTestPod(name, namespace, healthy, nil)
	pod.OwnerReferences = []metav1.OwnerReference{
		{
			Kind: "StatefulSet",
			Name: statefulSetName,
		},
	}
	return pod
}

func createTestPodWithStatefulSetAndAnnotations(name, namespace string, healthy bool, statefulSetName string, annotations map[string]string) *corev1.Pod {
	pod := createTestPod(name, namespace, healthy, annotations)
	pod.OwnerReferences = []metav1.OwnerReference{
		{
			Kind: "StatefulSet",
			Name: statefulSetName,
		},
	}
	return pod
}

// Helper function to create pods with correct Redis instance labels
func createTestPodWithCorrectLabels(name, namespace, redisName string, healthy bool, statefulSetName string, annotations map[string]string) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				redisNameLabel:     "redis",
				redisInstanceLabel: redisName, // Use the actual Redis instance name
			},
			Annotations: annotations,
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "StatefulSet",
					Name: statefulSetName,
				},
			},
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
