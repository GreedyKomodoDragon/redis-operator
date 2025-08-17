package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	koncachev1alpha1 "github.com/GreedyKomodoDragon/redis-operator/api/v1alpha1"
)

const (
	testRedisName       = "test-redis"
	testRedisImageValue = "redis:7.2-alpine"
)

func TestGetSidecarAnnotationsDisabled(t *testing.T) {
	// Create Istio manager
	istioManager := &IstioManager{}

	// Create Redis without Istio configuration
	redis := &koncachev1alpha1.Redis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testRedisName,
			Namespace: "default",
		},
		Spec: koncachev1alpha1.RedisSpec{
			Mode:    koncachev1alpha1.RedisModeStandalone,
			Version: "7.2",
			Image:   testRedisImageValue,
			Port:    6379,
		},
	}

	annotations := istioManager.GetSidecarAnnotations(redis)

	// Should return empty annotations when Istio is not configured
	assert.Empty(t, annotations, "Expected empty annotations when Istio is not configured")
}

func TestGetSidecarAnnotationsEnabled(t *testing.T) {
	// Create Istio manager
	istioManager := &IstioManager{}

	// Create Redis with Istio configuration
	enabledTrue := true
	redis := &koncachev1alpha1.Redis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testRedisName,
			Namespace: "default",
		},
		Spec: koncachev1alpha1.RedisSpec{
			Mode:    koncachev1alpha1.RedisModeStandalone,
			Version: "7.2",
			Image:   testRedisImageValue,
			Port:    6379,
			Networking: &koncachev1alpha1.RedisNetworking{
				Istio: &koncachev1alpha1.RedisIstio{
					Enabled: true,
					SidecarInjection: &koncachev1alpha1.RedisIstioSidecarInjection{
						Enabled:  &enabledTrue,
						Template: "custom-template",
						ProxyMetadata: map[string]string{
							"PILOT_ENABLE_WORKLOAD_ENTRY_AUTOREGISTRATION": "true",
							"PROXY_CONFIG_CPU": "100m",
						},
					},
				},
			},
		},
	}

	annotations := istioManager.GetSidecarAnnotations(redis)

	// Should have sidecar injection annotations
	expectedAnnotations := map[string]string{
		SidecarInjectAnnotation:   "true",
		SidecarTemplateAnnotation: "custom-template",
		"sidecar.istio.io/PILOT_ENABLE_WORKLOAD_ENTRY_AUTOREGISTRATION": "true",
		"sidecar.istio.io/PROXY_CONFIG_CPU":                             "100m",
	}

	assert.Len(t, annotations, len(expectedAnnotations), "Annotation count should match expected")

	for expectedKey, expectedValue := range expectedAnnotations {
		assert.Contains(t, annotations, expectedKey, "Expected annotation %s should be present", expectedKey)
		assert.Equal(t, expectedValue, annotations[expectedKey], "Annotation %s should have correct value", expectedKey)
	}
}

func TestGetSidecarAnnotationsDisabledExplicitly(t *testing.T) {
	// Create Istio manager
	istioManager := &IstioManager{}

	// Create Redis with Istio enabled but sidecar injection disabled
	enabledFalse := false
	redis := &koncachev1alpha1.Redis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testRedisName,
			Namespace: "default",
		},
		Spec: koncachev1alpha1.RedisSpec{
			Mode:    koncachev1alpha1.RedisModeStandalone,
			Version: "7.2",
			Image:   testRedisImageValue,
			Port:    6379,
			Networking: &koncachev1alpha1.RedisNetworking{
				Istio: &koncachev1alpha1.RedisIstio{
					Enabled: true,
					SidecarInjection: &koncachev1alpha1.RedisIstioSidecarInjection{
						Enabled: &enabledFalse,
					},
				},
			},
		},
	}

	annotations := istioManager.GetSidecarAnnotations(redis)

	// Should have sidecar injection disabled
	assert.Contains(t, annotations, SidecarInjectAnnotation, "Expected sidecar injection annotation")
	assert.Equal(t, "false", annotations[SidecarInjectAnnotation], "Expected sidecar injection to be disabled")
}

func TestBuildTrafficPolicy(t *testing.T) {
	// Test with nil policy
	result := buildTrafficPolicy(nil)
	assert.Empty(t, result, "Expected empty result for nil policy")

	// Test with simple load balancer policy
	policy := &koncachev1alpha1.RedisIstioTrafficPolicy{
		LoadBalancer: &koncachev1alpha1.RedisIstioLoadBalancer{
			Simple: "ROUND_ROBIN",
		},
	}

	result = buildTrafficPolicy(policy)

	// Check load balancer
	require.Contains(t, result, "loadBalancer", "Expected loadBalancer in result")

	lbMap, ok := result["loadBalancer"].(map[string]interface{})
	require.True(t, ok, "Expected loadBalancer to be a map")

	require.Contains(t, lbMap, "simple", "Expected simple in loadBalancer")
	assert.Equal(t, "ROUND_ROBIN", lbMap["simple"], "Expected simple=ROUND_ROBIN")
}

func TestBuildTrafficPolicyWithConnectionPool(t *testing.T) {
	timeout := &metav1.Duration{Duration: 30 * time.Second}
	keepalive := &metav1.Duration{Duration: 2 * time.Hour}

	policy := &koncachev1alpha1.RedisIstioTrafficPolicy{
		ConnectionPool: &koncachev1alpha1.RedisIstioConnectionPool{
			TCP: &koncachev1alpha1.RedisIstioTCPSettings{
				MaxConnections: 100,
				ConnectTimeout: timeout,
				TCPNoDelay:     true,
				KeepaliveTime:  keepalive,
			},
		},
	}

	result := buildTrafficPolicy(policy)

	// Check connection pool
	require.Contains(t, result, "connectionPool", "Expected connectionPool in result")

	cpMap, ok := result["connectionPool"].(map[string]interface{})
	require.True(t, ok, "Expected connectionPool to be a map")

	require.Contains(t, cpMap, "tcp", "Expected tcp in connectionPool")

	tcpMap, ok := cpMap["tcp"].(map[string]interface{})
	require.True(t, ok, "Expected tcp to be a map")

	require.Contains(t, tcpMap, "maxConnections", "Expected maxConnections in tcp")
	assert.Equal(t, int64(100), tcpMap["maxConnections"], "Expected maxConnections=100")

	require.Contains(t, tcpMap, "tcpNoDelay", "Expected tcpNoDelay in tcp")
	assert.Equal(t, true, tcpMap["tcpNoDelay"], "Expected tcpNoDelay=true")
}

func TestVirtualServiceStructuredConversion(t *testing.T) {
	// Test our new structured approach to VirtualService conversion
	port := uint32(6379)
	tcpRoute := TCPRoute{
		Match: []L4MatchAttributes{
			{
				Port: &port,
			},
		},
		Route: []RouteDestination{
			{
				Destination: &Destination{
					Host: "redis-service",
					Port: &PortSelector{
						Number: &port,
					},
				},
			},
		},
	}

	timeout := "30s"
	unstructured := tcpRoute.ToUnstructured(&timeout)

	t.Run("match conversion", func(t *testing.T) {
		require.Len(t, unstructured.Match, 1, "Expected 1 match")
		require.NotNil(t, unstructured.Match[0].Port, "Expected port to be set")
		assert.Equal(t, int64(port), *unstructured.Match[0].Port, "Expected correct port")
	})

	t.Run("route conversion", func(t *testing.T) {
		require.Len(t, unstructured.Route, 1, "Expected 1 route")
		assert.Equal(t, "redis-service", unstructured.Route[0].Destination.Host, "Expected correct host")
	})

	t.Run("timeout conversion", func(t *testing.T) {
		require.NotNil(t, unstructured.Timeout, "Expected timeout to be set")
		assert.Equal(t, timeout, *unstructured.Timeout, "Expected correct timeout")
	})

	t.Run("map conversion", func(t *testing.T) {
		resultMap := unstructured.ToMap()

		// Verify map structure
		require.Contains(t, resultMap, "match", "Expected match in result map")
		matchSlice, ok := resultMap["match"].([]interface{})
		require.True(t, ok, "Expected match to be slice")
		assert.Len(t, matchSlice, 1, "Expected 1 match in slice")

		require.Contains(t, resultMap, "route", "Expected route in result map")
		routeSlice, ok := resultMap["route"].([]interface{})
		require.True(t, ok, "Expected route to be slice")
		assert.Len(t, routeSlice, 1, "Expected 1 route in slice")

		require.Contains(t, resultMap, "timeout", "Expected timeout in result map")
		timeoutStr, ok := resultMap["timeout"].(string)
		require.True(t, ok, "Expected timeout to be string")
		assert.Equal(t, timeout, timeoutStr, "Expected correct timeout string")
	})
}
