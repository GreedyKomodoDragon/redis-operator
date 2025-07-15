package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	koncachev1alpha1 "github.com/GreedyKomodoDragon/redis-operator/api/v1alpha1"
)

func TestCleanupOrphanedStatefulSets(t *testing.T) {
	testScheme := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(testScheme))
	require.NoError(t, koncachev1alpha1.AddToScheme(testScheme))

	ctx := context.Background()

	// Create a Redis instance
	redis := &koncachev1alpha1.Redis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-redis",
			Namespace: "default",
			UID:       "test-uid",
		},
		Spec: koncachev1alpha1.RedisSpec{
			HighAvailability: &koncachev1alpha1.RedisHighAvailability{
				Enabled:  true,
				Replicas: 2, // Only want 2 replicas now
			},
		},
	}

	// Create existing StatefulSets (simulate having 3 replicas before, now reducing to 2)
	existingStatefulSets := []*appsv1.StatefulSet{
		createTestStatefulSet("test-redis-ha-1", "default", redis),
		createTestStatefulSet("test-redis-ha-2", "default", redis),
		createTestStatefulSet("test-redis-ha-3", "default", redis), // This should be deleted
	}

	// Create expected StatefulSets (only 2 now)
	expectedStatefulSets := []*appsv1.StatefulSet{
		createTestStatefulSet("test-redis-ha-1", "default", redis),
		createTestStatefulSet("test-redis-ha-2", "default", redis),
	}

	// Create fake client with existing data
	clientBuilder := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(redis)
	for _, ss := range existingStatefulSets {
		clientBuilder = clientBuilder.WithObjects(ss)
	}
	fakeClient := clientBuilder.Build()

	// Create controller
	controller := &StandaloneController{
		Client: fakeClient,
		Scheme: testScheme,
	}

	// Execute cleanup
	err := controller.cleanupOrphanedStatefulSets(ctx, redis, expectedStatefulSets)
	require.NoError(t, err)

	// Verify that test-redis-ha-3 was deleted
	deletedSS := &appsv1.StatefulSet{}
	err = fakeClient.Get(ctx, types.NamespacedName{
		Name:      "test-redis-ha-3",
		Namespace: "default",
	}, deletedSS)
	assert.True(t, err != nil, "StatefulSet test-redis-ha-3 should have been deleted")

	// Verify that test-redis-ha-1 and test-redis-ha-2 still exist
	for _, expectedName := range []string{"test-redis-ha-1", "test-redis-ha-2"} {
		existingSS := &appsv1.StatefulSet{}
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      expectedName,
			Namespace: "default",
		}, existingSS)
		assert.NoError(t, err, "StatefulSet %s should still exist", expectedName)
	}
}

func TestIsOwnedByRedis(t *testing.T) {
	controller := &StandaloneController{}

	redis := &koncachev1alpha1.Redis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-redis",
			Namespace: "default",
			UID:       "test-uid",
		},
	}

	tests := []struct {
		name        string
		statefulSet *appsv1.StatefulSet
		expectOwned bool
	}{
		{
			name: "StatefulSet owned by Redis",
			statefulSet: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-redis-ha-1",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind:       "Redis",
							Name:       "test-redis",
							APIVersion: "koncache.greedykomodo/v1alpha1",
							UID:        "test-uid",
						},
					},
				},
			},
			expectOwned: true,
		},
		{
			name: "StatefulSet not owned by Redis",
			statefulSet: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "other-statefulset",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind:       "Redis",
							Name:       "other-redis",
							APIVersion: "koncache.greedykomodo/v1alpha1",
							UID:        "other-uid",
						},
					},
				},
			},
			expectOwned: false,
		},
		{
			name: "StatefulSet with no owner references",
			statefulSet: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "orphan-statefulset",
					Namespace: "default",
				},
			},
			expectOwned: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := controller.isOwnedByRedis(tt.statefulSet, redis)
			assert.Equal(t, tt.expectOwned, result)
		})
	}
}

// Helper function to create a test StatefulSet
func createTestStatefulSet(name, namespace string, redis *koncachev1alpha1.Redis) *appsv1.StatefulSet {
	labels := map[string]string{
		"app.kubernetes.io/name":     "redis",
		"app.kubernetes.io/instance": redis.Name,
	}

	ss := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "Redis",
					Name:       redis.Name,
					APIVersion: "koncache.greedykomodo/v1alpha1",
					UID:        redis.UID,
				},
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
		},
	}

	return ss
}
