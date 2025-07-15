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
