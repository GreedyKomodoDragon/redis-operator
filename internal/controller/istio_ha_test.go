package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	koncachev1alpha1 "github.com/GreedyKomodoDragon/redis-operator/api/v1alpha1"
)

const (
	testHARedisName      = "test-ha-redis"
	testHARedisNamespace = "default"
	testRedisImageHA     = "redis:7.2-alpine"
	testStatefulSet1     = "test-ha-redis-ha-1"
	testStatefulSet2     = "test-ha-redis-ha-2"
	testStatefulSet3     = "test-ha-redis-ha-3"
	testPod1             = "test-ha-redis-ha-1-0"
	testPod2             = "test-ha-redis-ha-2-0"
	testPod3             = "test-ha-redis-ha-3-0"
)

// TestIstioHAIntegration tests Istio service mesh integration with HA Redis deployments
func TestIstioHAIntegration(t *testing.T) {
	testScheme := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(testScheme))
	require.NoError(t, koncachev1alpha1.AddToScheme(testScheme))

	ctx := context.Background()

	t.Run("HA Redis with Istio sidecar injection", func(t *testing.T) {
		// Create HA Redis with Istio sidecar injection enabled
		redis := createHARedisWithIstio(&koncachev1alpha1.RedisIstioSidecarInjection{
			Enabled: &[]bool{true}[0],
			ProxyMetadata: map[string]string{
				"PILOT_ENABLE_WORKLOAD_ENTRY_AUTO_REGISTRATION": "true",
				"ISTIO_META_DNS_CAPTURE":                        "true",
			},
		})

		fakeClient := fake.NewClientBuilder().
			WithScheme(testScheme).
			WithObjects(redis).
			Build()

		istioManager := NewIstioManager(fakeClient, testScheme)

		// Test sidecar annotations for HA deployment
		annotations := istioManager.GetSidecarAnnotations(redis)

		expectedAnnotations := map[string]string{
			SidecarInjectAnnotation: "true",
			"sidecar.istio.io/PILOT_ENABLE_WORKLOAD_ENTRY_AUTO_REGISTRATION": "true",
			"sidecar.istio.io/ISTIO_META_DNS_CAPTURE":                        "true",
		}

		assert.Len(t, annotations, len(expectedAnnotations), "Should have correct number of annotations")
		for key, expectedValue := range expectedAnnotations {
			assert.Equal(t, expectedValue, annotations[key], "Annotation %s should have correct value", key)
		}
	})

	t.Run("HA Redis with Istio VirtualService", func(t *testing.T) {
		// Create HA Redis with VirtualService configuration
		redis := createHARedisWithIstio(&koncachev1alpha1.RedisIstioSidecarInjection{
			Enabled: &[]bool{true}[0],
		})
		redis.Spec.Networking.Istio.VirtualService = &koncachev1alpha1.RedisIstioVirtualService{
			Enabled:  &[]bool{true}[0],
			Hosts:    []string{"redis.example.com"},
			Gateways: []string{"istio-system/gateway"},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(testScheme).
			WithObjects(redis).
			Build()

		istioManager := NewIstioManager(fakeClient, testScheme)

		// Reconcile Istio resources
		err := istioManager.ReconcileIstioResources(ctx, redis)
		require.NoError(t, err)

		// Verify VirtualService was created
		virtualService := &unstructured.Unstructured{}
		virtualService.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   VirtualServiceGVR.Group,
			Version: VirtualServiceGVR.Version,
			Kind:    "VirtualService",
		})

		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      redis.Name + "-vs",
			Namespace: redis.Namespace,
		}, virtualService)
		require.NoError(t, err, "VirtualService should be created")

		// Verify VirtualService spec contains HA service configuration
		spec, found, err := unstructured.NestedMap(virtualService.Object, "spec")
		require.NoError(t, err)
		require.True(t, found, "VirtualService should have spec")

		hosts, found, err := unstructured.NestedStringSlice(spec, "hosts")
		require.NoError(t, err)
		require.True(t, found, "VirtualService should have hosts")
		assert.Contains(t, hosts, "redis.example.com", "VirtualService should contain custom host")

		// Verify TCP route configuration for Redis port
		tcpRoutes, found, err := unstructured.NestedSlice(spec, "tcp")
		require.NoError(t, err)
		require.True(t, found, "VirtualService should have TCP routes")
		assert.Len(t, tcpRoutes, 1, "Should have one TCP route")
	})

	t.Run("HA Redis with Istio DestinationRule and traffic policy", func(t *testing.T) {
		// Create HA Redis with DestinationRule and traffic policy
		redis := createHARedisWithIstio(&koncachev1alpha1.RedisIstioSidecarInjection{
			Enabled: &[]bool{true}[0],
		})
		redis.Spec.Networking.Istio.DestinationRule = &koncachev1alpha1.RedisIstioDestinationRule{
			Enabled: &[]bool{true}[0],
			TrafficPolicy: &koncachev1alpha1.RedisIstioTrafficPolicy{
				LoadBalancer: &koncachev1alpha1.RedisIstioLoadBalancer{
					Simple: "ROUND_ROBIN",
				},
				ConnectionPool: &koncachev1alpha1.RedisIstioConnectionPool{
					TCP: &koncachev1alpha1.RedisIstioTCPSettings{
						MaxConnections: 100,
						ConnectTimeout: &metav1.Duration{Duration: 30 * time.Second},
						TCPNoDelay:     true,
					},
				},
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(testScheme).
			WithObjects(redis).
			Build()

		istioManager := NewIstioManager(fakeClient, testScheme)

		// Reconcile Istio resources
		err := istioManager.ReconcileIstioResources(ctx, redis)
		require.NoError(t, err)

		// Verify DestinationRule was created
		destinationRule := &unstructured.Unstructured{}
		destinationRule.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   DestinationRuleGVR.Group,
			Version: DestinationRuleGVR.Version,
			Kind:    "DestinationRule",
		})

		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      redis.Name + "-dr",
			Namespace: redis.Namespace,
		}, destinationRule)
		require.NoError(t, err, "DestinationRule should be created")

		// Verify DestinationRule spec contains traffic policy
		spec, found, err := unstructured.NestedMap(destinationRule.Object, "spec")
		require.NoError(t, err)
		require.True(t, found, "DestinationRule should have spec")

		trafficPolicy, found, err := unstructured.NestedMap(spec, "trafficPolicy")
		require.NoError(t, err)
		require.True(t, found, "DestinationRule should have traffic policy")

		// Verify load balancer settings
		loadBalancer, found, err := unstructured.NestedMap(trafficPolicy, "loadBalancer")
		require.NoError(t, err)
		require.True(t, found, "Traffic policy should have load balancer")

		simple, found, err := unstructured.NestedString(loadBalancer, "simple")
		require.NoError(t, err)
		require.True(t, found, "Load balancer should have simple setting")
		assert.Equal(t, "ROUND_ROBIN", simple, "Load balancer should use ROUND_ROBIN")

		// Verify connection pool settings
		connectionPool, found, err := unstructured.NestedMap(trafficPolicy, "connectionPool")
		require.NoError(t, err)
		require.True(t, found, "Traffic policy should have connection pool")

		tcp, found, err := unstructured.NestedMap(connectionPool, "tcp")
		require.NoError(t, err)
		require.True(t, found, "Connection pool should have TCP settings")

		maxConnections, found, err := unstructured.NestedInt64(tcp, "maxConnections")
		require.NoError(t, err)
		require.True(t, found, "TCP settings should have max connections")
		assert.Equal(t, int64(100), maxConnections, "Max connections should be 100")
	})

	t.Run("HA Redis with Istio PeerAuthentication for mTLS", func(t *testing.T) {
		// Create HA Redis with PeerAuthentication for mutual TLS
		redis := createHARedisWithIstio(&koncachev1alpha1.RedisIstioSidecarInjection{
			Enabled: &[]bool{true}[0],
		})
		redis.Spec.Networking.Istio.PeerAuthentication = &koncachev1alpha1.RedisIstioPeerAuthentication{
			Enabled: &[]bool{true}[0],
			MutualTLS: &koncachev1alpha1.RedisIstioMutualTLS{
				Mode: "STRICT",
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(testScheme).
			WithObjects(redis).
			Build()

		istioManager := NewIstioManager(fakeClient, testScheme)

		// Reconcile Istio resources
		err := istioManager.ReconcileIstioResources(ctx, redis)
		require.NoError(t, err)

		// Verify PeerAuthentication was created
		peerAuth := &unstructured.Unstructured{}
		peerAuth.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   PeerAuthenticationGVR.Group,
			Version: PeerAuthenticationGVR.Version,
			Kind:    "PeerAuthentication",
		})

		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      redis.Name + "-pa",
			Namespace: redis.Namespace,
		}, peerAuth)
		require.NoError(t, err, "PeerAuthentication should be created")

		// Verify PeerAuthentication spec contains mTLS configuration
		spec, found, err := unstructured.NestedMap(peerAuth.Object, "spec")
		require.NoError(t, err)
		require.True(t, found, "PeerAuthentication should have spec")

		mtls, found, err := unstructured.NestedMap(spec, "mtls")
		require.NoError(t, err)
		require.True(t, found, "PeerAuthentication should have mTLS configuration")

		mode, found, err := unstructured.NestedString(mtls, "mode")
		require.NoError(t, err)
		require.True(t, found, "mTLS should have mode")
		assert.Equal(t, "STRICT", mode, "mTLS mode should be STRICT")
	})

	t.Run("HA Redis StatefulSets receive Istio annotations", func(t *testing.T) {
		// Create HA Redis with Istio configuration
		redis := createHARedisWithIstio(&koncachev1alpha1.RedisIstioSidecarInjection{
			Enabled:  &[]bool{true}[0],
			Template: "custom-template",
		})

		fakeClient := fake.NewClientBuilder().
			WithScheme(testScheme).
			WithObjects(redis).
			Build()

		standaloneController := NewStandaloneController(fakeClient, testScheme)
		standaloneController.IstioManager = NewIstioManager(fakeClient, testScheme)

		// Generate StatefulSets for HA mode
		statefulSets := standaloneController.statefulsSetForRedis(redis, "", "testhash")

		// Verify that we get multiple StatefulSets for HA
		assert.Len(t, statefulSets, 3, "Should create 3 StatefulSets for HA with 3 replicas")

		// Verify each StatefulSet has Istio annotations
		for i, sts := range statefulSets {
			expectedName := redis.Name + "-ha-" + string(rune('1'+i))
			assert.Equal(t, expectedName, sts.Name, "StatefulSet should have correct HA naming pattern")

			// Check pod template annotations
			podAnnotations := sts.Spec.Template.GetAnnotations()
			require.NotNil(t, podAnnotations, "Pod template should have annotations")

			assert.Equal(t, "true", podAnnotations[SidecarInjectAnnotation], "Pod should have sidecar injection enabled")
			assert.Equal(t, "custom-template", podAnnotations[SidecarTemplateAnnotation], "Pod should have custom sidecar template")
		}
	})

	t.Run("HA Redis leader failover with Istio service mesh", func(t *testing.T) {
		// Create HA Redis with Istio and leader election
		redis := createHARedisWithIstio(&koncachev1alpha1.RedisIstioSidecarInjection{
			Enabled: &[]bool{true}[0],
		})

		// Create pods representing the HA deployment
		leaderPod := createHAPodWithIstio(testPod1, testHARedisNamespace, testHARedisName, true, testStatefulSet1, map[string]string{
			redisRoleAnnotation:     redisLeaderRole,
			SidecarInjectAnnotation: "true",
		})
		followerPod1 := createHAPodWithIstio(testPod2, testHARedisNamespace, testHARedisName, true, testStatefulSet2, map[string]string{
			SidecarInjectAnnotation: "true",
		})
		followerPod2 := createHAPodWithIstio(testPod3, testHARedisNamespace, testHARedisName, true, testStatefulSet3, map[string]string{
			SidecarInjectAnnotation: "true",
		})

		fakeClient := fake.NewClientBuilder().
			WithScheme(testScheme).
			WithObjects(redis, leaderPod, followerPod1, followerPod2).
			Build()

		reconciler := &RedisReconciler{
			Client: fakeClient,
			Scheme: testScheme,
		}

		// Simulate leader pod failure
		err := reconciler.handleLeaderFailover(ctx, leaderPod, redis)
		require.NoError(t, err)

		// Verify new leader was promoted and still has Istio annotations
		updatedFollowerPod1 := &corev1.Pod{}
		err = fakeClient.Get(ctx, types.NamespacedName{Name: testPod2, Namespace: testHARedisNamespace}, updatedFollowerPod1)
		require.NoError(t, err)

		annotations := updatedFollowerPod1.GetAnnotations()
		require.NotNil(t, annotations, "Pod should have annotations")

		// Verify both leader promotion and Istio annotations are preserved
		assert.Equal(t, redisLeaderRole, annotations[redisRoleAnnotation], "Pod should be promoted to leader")
		assert.Equal(t, "true", annotations[SidecarInjectAnnotation], "Pod should retain Istio sidecar injection")
	})

	t.Run("HA Redis scaling with Istio resources", func(t *testing.T) {
		// Create HA Redis initially with 3 replicas
		redis := createHARedisWithIstio(&koncachev1alpha1.RedisIstioSidecarInjection{
			Enabled: &[]bool{true}[0],
		})

		// Create StatefulSets for initial state
		initialStatefulSets := []*appsv1.StatefulSet{
			createHAStatefulSetWithIstio(testStatefulSet1, testHARedisNamespace, testHARedisName),
			createHAStatefulSetWithIstio(testStatefulSet2, testHARedisNamespace, testHARedisName),
			createHAStatefulSetWithIstio(testStatefulSet3, testHARedisNamespace, testHARedisName),
		}

		clientBuilder := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(redis)
		for _, sts := range initialStatefulSets {
			clientBuilder = clientBuilder.WithObjects(sts)
		}
		fakeClient := clientBuilder.Build()

		istioManager := NewIstioManager(fakeClient, testScheme)
		standaloneController := NewStandaloneController(fakeClient, testScheme)
		standaloneController.IstioManager = istioManager

		// Scale down to 2 replicas
		redis.Spec.HighAvailability.Replicas = 2

		// Generate new expected StatefulSets
		expectedStatefulSets := standaloneController.statefulsSetForRedis(redis, "", "")
		assert.Len(t, expectedStatefulSets, 2, "Should generate 2 StatefulSets after scaling down")

		// Test cleanup of orphaned StatefulSets
		err := standaloneController.cleanupOrphanedStatefulSets(ctx, redis, expectedStatefulSets)
		require.NoError(t, err)

		// Verify third StatefulSet was cleaned up
		orphanedSts := &appsv1.StatefulSet{}
		err = fakeClient.Get(ctx, types.NamespacedName{Name: testStatefulSet3, Namespace: testHARedisNamespace}, orphanedSts)
		assert.Error(t, err, "Orphaned StatefulSet should be cleaned up")

		// Verify remaining StatefulSets still exist and have Istio annotations
		for i := 1; i <= 2; i++ {
			sts := &appsv1.StatefulSet{}
			name := testHARedisName + "-ha-" + string(rune('0'+i))
			err = fakeClient.Get(ctx, types.NamespacedName{Name: name, Namespace: testHARedisNamespace}, sts)
			require.NoError(t, err, "Required StatefulSet should still exist")

			podAnnotations := sts.Spec.Template.GetAnnotations()
			assert.Equal(t, "true", podAnnotations[SidecarInjectAnnotation], "Remaining StatefulSets should retain Istio annotations")
		}
	})

	t.Run("HA Redis with Istio disabled should not have sidecar annotations", func(t *testing.T) {
		// Create HA Redis with Istio disabled
		redis := createHARedisWithIstio(&koncachev1alpha1.RedisIstioSidecarInjection{
			Enabled: &[]bool{false}[0],
		})

		fakeClient := fake.NewClientBuilder().
			WithScheme(testScheme).
			WithObjects(redis).
			Build()

		istioManager := NewIstioManager(fakeClient, testScheme)

		// Test sidecar annotations
		annotations := istioManager.GetSidecarAnnotations(redis)

		// Should only have the injection disabled annotation
		expectedAnnotations := map[string]string{
			SidecarInjectAnnotation: "false",
		}

		assert.Len(t, annotations, len(expectedAnnotations), "Should have correct number of annotations")
		for key, expectedValue := range expectedAnnotations {
			assert.Equal(t, expectedValue, annotations[key], "Annotation %s should have correct value", key)
		}
	})
}

// Helper functions for creating test objects

func createHARedisWithIstio(sidecarConfig *koncachev1alpha1.RedisIstioSidecarInjection) *koncachev1alpha1.Redis {
	return &koncachev1alpha1.Redis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testHARedisName,
			Namespace: testHARedisNamespace,
		},
		Spec: koncachev1alpha1.RedisSpec{
			Mode:    koncachev1alpha1.RedisModeStandalone,
			Version: "7.2",
			Image:   testRedisImageHA,
			Port:    6379,
			HighAvailability: &koncachev1alpha1.RedisHighAvailability{
				Enabled:  true,
				Replicas: 3,
			},
			Networking: &koncachev1alpha1.RedisNetworking{
				Istio: &koncachev1alpha1.RedisIstio{
					Enabled:          true,
					SidecarInjection: sidecarConfig,
				},
			},
		},
	}
}

func createHAPodWithIstio(name, namespace, redisName string, healthy bool, statefulSetName string, annotations map[string]string) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":     "redis",
				"app.kubernetes.io/instance": redisName,
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
					Image: testRedisImageHA,
				},
				{
					Name:  "istio-proxy",
					Image: "istio/proxyv2:1.20.0",
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
				{
					Name:  "istio-proxy",
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

func createHAStatefulSetWithIstio(name, namespace, redisName string) *appsv1.StatefulSet {
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
					UID:        "test-uid",
				},
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &[]int32{1}[0],
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
					Annotations: map[string]string{
						SidecarInjectAnnotation: "true",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "redis",
							Image: testRedisImageHA,
						},
					},
				},
			},
		},
	}
}
