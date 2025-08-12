package controller

import (
	"testing"
	"time"

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
	if len(annotations) != 0 {
		t.Errorf("Expected empty annotations, got %d annotations", len(annotations))
	}
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

	if len(annotations) != len(expectedAnnotations) {
		t.Errorf("Expected %d annotations, got %d", len(expectedAnnotations), len(annotations))
	}

	for expectedKey, expectedValue := range expectedAnnotations {
		if actualValue, exists := annotations[expectedKey]; !exists {
			t.Errorf("Expected annotation %s not found", expectedKey)
		} else if actualValue != expectedValue {
			t.Errorf("Expected annotation %s=%s, got %s=%s", expectedKey, expectedValue, expectedKey, actualValue)
		}
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
	expectedValue := "false"
	if actualValue, exists := annotations[SidecarInjectAnnotation]; !exists {
		t.Errorf("Expected annotation %s not found", SidecarInjectAnnotation)
	} else if actualValue != expectedValue {
		t.Errorf("Expected %s=%s, got %s", SidecarInjectAnnotation, expectedValue, actualValue)
	}
}

func TestBuildTrafficPolicy(t *testing.T) {
	// Test with nil policy
	result := buildTrafficPolicy(nil)
	if len(result) != 0 {
		t.Errorf("Expected empty result for nil policy, got %d items", len(result))
	}

	// Test with simple load balancer policy
	policy := &koncachev1alpha1.RedisIstioTrafficPolicy{
		LoadBalancer: &koncachev1alpha1.RedisIstioLoadBalancer{
			Simple: "ROUND_ROBIN",
		},
	}

	result = buildTrafficPolicy(policy)

	// Check load balancer
	if lb, exists := result["loadBalancer"]; !exists {
		t.Error("Expected loadBalancer in result")
	} else {
		if lbMap, ok := lb.(map[string]interface{}); !ok {
			t.Error("Expected loadBalancer to be a map")
		} else {
			if simple, exists := lbMap["simple"]; !exists {
				t.Error("Expected simple in loadBalancer")
			} else if simple != "ROUND_ROBIN" {
				t.Errorf("Expected simple=ROUND_ROBIN, got %s", simple)
			}
		}
	}
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
	if cp, exists := result["connectionPool"]; !exists {
		t.Error("Expected connectionPool in result")
	} else if cpMap, ok := cp.(map[string]interface{}); !ok {
		t.Error("Expected connectionPool to be a map")
	} else if tcp, exists := cpMap["tcp"]; !exists {
		t.Error("Expected tcp in connectionPool")
	} else if tcpMap, ok := tcp.(map[string]interface{}); !ok {
		t.Error("Expected tcp to be a map")
	} else {
		if maxConn, exists := tcpMap["maxConnections"]; !exists {
			t.Error("Expected maxConnections in tcp")
		} else if maxConn != int32(100) {
			t.Errorf("Expected maxConnections=100, got %v", maxConn)
		}
		if tcpNoDelay, exists := tcpMap["tcpNoDelay"]; !exists {
			t.Error("Expected tcpNoDelay in tcp")
		} else if tcpNoDelay != true {
			t.Errorf("Expected tcpNoDelay=true, got %v", tcpNoDelay)
		}
	}
}
