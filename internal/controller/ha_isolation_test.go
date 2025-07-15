package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	koncachev1alpha1 "github.com/GreedyKomodoDragon/redis-operator/api/v1alpha1"
)

// TestHAStandaloneIsolation focuses on the actual controller logic without requiring full reconciliation
func TestHAStandaloneIsolation(t *testing.T) {
	t.Run("Standalone mode creates single StatefulSet with correct naming", func(t *testing.T) {
		standaloneRedis := &koncachev1alpha1.Redis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-standalone",
				Namespace: "default",
			},
			Spec: koncachev1alpha1.RedisSpec{
				Image: testRedisImage,
				HighAvailability: &koncachev1alpha1.RedisHighAvailability{
					Enabled: false,
				},
			},
		}

		controller := &StandaloneController{}
		statefulSets := controller.statefulsSetForRedis(standaloneRedis, "", "")

		require.Len(t, statefulSets, 1, "Standalone should create exactly one StatefulSet")
		sts := statefulSets[0]

		assert.Equal(t, "test-standalone", sts.Name, "StatefulSet name should match Redis name")
		assert.Equal(t, int32(1), *sts.Spec.Replicas, "Standalone should have 1 replica")
		assert.NotContains(t, sts.Name, "-ha-", "Standalone should not have HA naming pattern")

		// Verify pod template doesn't have HA-specific annotations
		podTemplate := sts.Spec.Template
		annotations := podTemplate.GetAnnotations()
		if annotations != nil {
			assert.NotContains(t, annotations, redisRoleAnnotation, "Standalone pods should not have role annotations")
		}
	})

	t.Run("HA mode creates multiple StatefulSets with correct naming", func(t *testing.T) {
		haRedis := &koncachev1alpha1.Redis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ha",
				Namespace: "default",
			},
			Spec: koncachev1alpha1.RedisSpec{
				Image: testRedisImage,
				HighAvailability: &koncachev1alpha1.RedisHighAvailability{
					Enabled:  true,
					Replicas: 3,
				},
			},
		}

		controller := &StandaloneController{}
		statefulSets := controller.statefulsSetForRedis(haRedis, "", "")

		require.Len(t, statefulSets, 3, "HA should create multiple StatefulSets")

		for i, sts := range statefulSets {
			expectedName := "test-ha-ha-" + string(rune('1'+i))
			assert.Equal(t, expectedName, sts.Name, "HA StatefulSet should follow naming pattern")
			assert.Equal(t, int32(1), *sts.Spec.Replicas, "Each HA StatefulSet should have 1 replica")
		}
	})

	t.Run("HA nil spec is treated as standalone", func(t *testing.T) {
		redisWithNilHA := &koncachev1alpha1.Redis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-nil-ha",
				Namespace: "default",
			},
			Spec: koncachev1alpha1.RedisSpec{
				Image:            testRedisImage,
				HighAvailability: nil, // nil HA spec
			},
		}

		controller := &StandaloneController{}
		statefulSets := controller.statefulsSetForRedis(redisWithNilHA, "", "")

		require.Len(t, statefulSets, 1, "Nil HA spec should be treated as standalone")
		sts := statefulSets[0]
		assert.Equal(t, "test-nil-ha", sts.Name, "Should use standalone naming")
	})

	t.Run("Leader annotations only apply to HA mode", func(t *testing.T) {
		// Test that isPodLeader works correctly
		reconciler := &RedisReconciler{}

		// Pod with leader annotation
		leaderPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					redisRoleAnnotation: redisLeaderRole,
				},
			},
		}
		assert.True(t, reconciler.isPodLeader(leaderPod), "Pod with leader annotation should be identified as leader")

		// Pod without annotations
		plainPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{},
		}
		assert.False(t, reconciler.isPodLeader(plainPod), "Pod without annotations should not be leader")

		// Pod with different annotation
		otherPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"other-annotation": "value",
				},
			},
		}
		assert.False(t, reconciler.isPodLeader(otherPod), "Pod with other annotations should not be leader")
	})

	t.Run("StatefulSet Service configurations differ between modes", func(t *testing.T) {
		controller := &StandaloneController{}

		// Standalone service
		standaloneRedis := &koncachev1alpha1.Redis{
			ObjectMeta: metav1.ObjectMeta{Name: "standalone-svc", Namespace: "default"},
			Spec: koncachev1alpha1.RedisSpec{
				HighAvailability: &koncachev1alpha1.RedisHighAvailability{Enabled: false},
			},
		}
		standaloneService := controller.serviceForRedis(standaloneRedis)
		assert.Equal(t, "standalone-svc", standaloneService.Name, "Standalone service should match Redis name")

		// Verify service selector doesn't include HA-specific selectors
		selector := standaloneService.Spec.Selector
		assert.NotContains(t, selector, "redis.io/role", "Standalone service should not select by role")
	})

	t.Run("Backup configuration isolation", func(t *testing.T) {
		controller := &StandaloneController{}

		// Standalone with backup
		redisWithBackup := &koncachev1alpha1.Redis{
			ObjectMeta: metav1.ObjectMeta{Name: "backup-test", Namespace: "default"},
			Spec: koncachev1alpha1.RedisSpec{
				HighAvailability: &koncachev1alpha1.RedisHighAvailability{Enabled: false},
				Backup: koncachev1alpha1.RedisBackup{
					BackUpInitConfig: koncachev1alpha1.BackupInitConfig{
						Enabled: true,
					},
				},
			},
		}

		statefulSets := controller.statefulsSetForRedis(redisWithBackup, "", "")
		require.Len(t, statefulSets, 1, "Should create one StatefulSet")

		sts := statefulSets[0]
		initContainers := sts.Spec.Template.Spec.InitContainers

		if len(initContainers) > 0 {
			// If backup init containers are added, verify they don't interfere with HA features
			podAnnotations := sts.Spec.Template.GetAnnotations()
			if podAnnotations != nil {
				assert.NotContains(t, podAnnotations, redisRoleAnnotation,
					"Backup-enabled standalone should not have role annotations")
			}
		}
	})
}

// TestStatefulSetNamingPatterns ensures naming conventions are followed
func TestStatefulSetNamingPatterns(t *testing.T) {
	controller := &StandaloneController{}

	tests := []struct {
		name          string
		redisName     string
		haEnabled     bool
		haReplicas    int32
		expectedNames []string
	}{
		{
			name:          "standalone simple name",
			redisName:     "redis",
			haEnabled:     false,
			expectedNames: []string{"redis"},
		},
		{
			name:          "standalone with hyphens",
			redisName:     "my-redis-instance",
			haEnabled:     false,
			expectedNames: []string{"my-redis-instance"},
		},
		{
			name:          "HA with 2 replicas",
			redisName:     "ha-redis",
			haEnabled:     true,
			haReplicas:    2,
			expectedNames: []string{"ha-redis-ha-1", "ha-redis-ha-2"},
		},
		{
			name:          "HA with 3 replicas",
			redisName:     "cluster",
			haEnabled:     true,
			haReplicas:    3,
			expectedNames: []string{"cluster-ha-1", "cluster-ha-2", "cluster-ha-3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redis := &koncachev1alpha1.Redis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tt.redisName,
					Namespace: "default",
				},
				Spec: koncachev1alpha1.RedisSpec{
					Image: testRedisImage,
					HighAvailability: &koncachev1alpha1.RedisHighAvailability{
						Enabled:  tt.haEnabled,
						Replicas: tt.haReplicas,
					},
				},
			}

			if !tt.haEnabled {
				redis.Spec.HighAvailability.Enabled = false
			}

			statefulSets := controller.statefulsSetForRedis(redis, "", "")
			require.Len(t, statefulSets, len(tt.expectedNames), "Should create expected number of StatefulSets")

			for i, expectedName := range tt.expectedNames {
				assert.Equal(t, expectedName, statefulSets[i].Name, "StatefulSet name should match expected pattern")
			}
		})
	}
}
