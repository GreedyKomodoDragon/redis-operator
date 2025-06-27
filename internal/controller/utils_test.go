package controller_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	koncachev1alpha1 "github.com/GreedyKomodoDragon/redis-operator/api/v1alpha1"
	"github.com/GreedyKomodoDragon/redis-operator/internal/controller"
)

// Test constants to avoid duplication
const (
	testRedisExporterImage = "oliver006/redis_exporter:v1.45.0"
	testRedisName          = "my-redis"
	testRedisClusterName   = "redis-cluster-prod"
	testRedisInstance      = "test-redis"
	testTLSCertsSecret     = "tls-certs"

	// Label constants
	testAppNameLabel     = "app.kubernetes.io/name"
	testAppInstanceLabel = "app.kubernetes.io/instance"
	testAppPartOfLabel   = "app.kubernetes.io/part-of"
	testRedisValue       = "redis"
	testOperatorValue    = "redis-operator"

	// Volume constants
	testDataVolume   = "redis-data"
	testConfigVolume = "redis-config"

	// Test case names
	testAuthEnabledCase = "auth enabled"
	testTLSEnabledCase  = "TLS enabled"
)

func TestLabelsForRedis(t *testing.T) {
	tests := []struct {
		name      string
		redisName string
		expected  map[string]string
	}{
		{
			name:      "basic redis name",
			redisName: testRedisName,
			expected: map[string]string{
				testAppNameLabel:     testRedisValue,
				testAppInstanceLabel: testRedisName,
				testAppPartOfLabel:   testOperatorValue,
			},
		},
		{
			name:      "redis name with hyphens",
			redisName: testRedisClusterName,
			expected: map[string]string{
				testAppNameLabel:     testRedisValue,
				testAppInstanceLabel: testRedisClusterName,
				testAppPartOfLabel:   testOperatorValue,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := controller.LabelsForRedis(tt.redisName)

			assert.Equal(t, tt.expected, result, "Labels should match expected values")
		})
	}
}

func TestLabelsForRedisCluster(t *testing.T) {
	tests := []struct {
		name      string
		redisName string
		expected  map[string]string
	}{
		{
			name:      "cluster labels",
			redisName: "my-cluster",
			expected: map[string]string{
				testAppNameLabel:              testRedisValue,
				testAppInstanceLabel:          "my-cluster",
				testAppPartOfLabel:            testOperatorValue,
				"app.kubernetes.io/component": "cluster",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := controller.LabelsForRedisCluster(tt.redisName)

			assert.Equal(t, tt.expected, result, "Cluster labels should match expected values")
		})
	}
}

func TestLabelsForRedisSentinel(t *testing.T) {
	tests := []struct {
		name      string
		redisName string
		expected  map[string]string
	}{
		{
			name:      "sentinel labels",
			redisName: "my-sentinel",
			expected: map[string]string{
				testAppNameLabel:              testRedisValue,
				testAppInstanceLabel:          "my-sentinel",
				testAppPartOfLabel:            testOperatorValue,
				"app.kubernetes.io/component": "sentinel",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := controller.LabelsForRedisSentinel(tt.redisName)

			assert.Equal(t, tt.expected, result, "Sentinel labels should match expected values")
		})
	}
}

func TestBuildRedisConfig(t *testing.T) {
	tests := []struct {
		name        string
		redis       *koncachev1alpha1.Redis
		contains    []string
		notContains []string
	}{
		{
			name: "empty config with zero values",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Config:   koncachev1alpha1.RedisConfig{},
					Security: koncachev1alpha1.RedisSecurity{},
				},
			},
			contains: []string{
				"timeout 0",
				"tcp-keepalive 0",
				"databases 0",
			},
		},
		{
			name: "custom config with memory settings",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Config: koncachev1alpha1.RedisConfig{
						MaxMemory:       "256mb",
						MaxMemoryPolicy: "allkeys-lfu",
						LogLevel:        "debug",
						Timeout:         300,
						TCPKeepAlive:    60,
						Databases:       8,
					},
					Security: koncachev1alpha1.RedisSecurity{},
				},
			},
			contains: []string{
				"maxmemory 256mb",
				"maxmemory-policy allkeys-lfu",
				"loglevel debug",
				"timeout 300",
				"tcp-keepalive 60",
				"databases 8",
			},
		},
		{
			name: "persistence configuration",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Config: koncachev1alpha1.RedisConfig{
						Save:        []string{"900 1", "300 10"},
						AppendOnly:  func() *bool { b := true; return &b }(),
						AppendFsync: "always",
					},
					Security: koncachev1alpha1.RedisSecurity{},
				},
			},
			contains: []string{
				"save 900 1",
				"save 300 10",
				"appendonly yes",
				"appendfsync always",
			},
		},
		{
			name: "security configuration",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Config: koncachev1alpha1.RedisConfig{},
					Security: koncachev1alpha1.RedisSecurity{
						RequireAuth: func() *bool { b := true; return &b }(),
					},
				},
			},
			contains: []string{
				"protected-mode yes",
			},
		},
		{
			name: "additional custom config",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Config: koncachev1alpha1.RedisConfig{
						AdditionalConfig: map[string]string{
							"custom-setting":  "custom-value",
							"another-setting": "another-value",
						},
					},
					Security: koncachev1alpha1.RedisSecurity{},
				},
			},
			contains: []string{
				"custom-setting custom-value",
				"another-setting another-value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := controller.BuildRedisConfig(tt.redis)

			for _, expected := range tt.contains {
				assert.Contains(t, result, expected, "Config should contain expected string: %s", expected)
			}

			for _, notExpected := range tt.notContains {
				assert.NotContains(t, result, notExpected, "Config should not contain unexpected string: %s", notExpected)
			}
		})
	}
}

func TestBuildRedisContainer(t *testing.T) {
	tests := []struct {
		name  string
		redis *koncachev1alpha1.Redis
		port  int32
	}{
		{
			name: "basic container configuration",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Image:           "redis:7.2-alpine",
					ImagePullPolicy: corev1.PullIfNotPresent,
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("512Mi"),
						},
					},
				},
			},
			port: 6379,
		},
		{
			name: "custom port configuration",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Image:           "redis:6.2",
					ImagePullPolicy: corev1.PullAlways,
				},
			},
			port: 7000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := controller.BuildRedisContainer(tt.redis, tt.port)

			// Test basic properties
			assert.Equal(t, "redis", result.Name)
			assert.Equal(t, tt.redis.Spec.Image, result.Image)
			assert.Equal(t, tt.redis.Spec.ImagePullPolicy, result.ImagePullPolicy)

			// Test port configuration
			require.Len(t, result.Ports, 1, "Container should have exactly 1 port")
			// For basic Redis without TLS, should have the configured port
			assert.Equal(t, tt.port, result.Ports[0].ContainerPort)
			assert.Equal(t, "redis", result.Ports[0].Name)

			// Test volume mounts
			expectedVolumeMountNames := []string{testDataVolume, testConfigVolume}
			assert.Len(t, result.VolumeMounts, len(expectedVolumeMountNames), "Container should have expected number of volume mounts")

			// Verify mount names exist
			mountNames := make([]string, len(result.VolumeMounts))
			for i, mount := range result.VolumeMounts {
				mountNames[i] = mount.Name
			}
			for _, expectedName := range expectedVolumeMountNames {
				assert.Contains(t, mountNames, expectedName, "Volume mount should contain %s", expectedName)
			}

			// Test probes are present
			assert.NotNil(t, result.LivenessProbe, "Container should have liveness probe")
			assert.NotNil(t, result.ReadinessProbe, "Container should have readiness probe")

			// Test command and args
			expectedCommand := []string{"redis-server"}
			assert.Equal(t, expectedCommand, result.Command)

			expectedArgs := []string{"/usr/local/etc/redis/redis.conf"}
			assert.Equal(t, expectedArgs, result.Args)
		})
	}
}

func TestBuildVolumeClaimTemplate(t *testing.T) {
	tests := []struct {
		name     string
		redis    *koncachev1alpha1.Redis
		expected corev1.PersistentVolumeClaim
	}{
		{
			name: "default storage configuration",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Storage: koncachev1alpha1.RedisStorage{},
				},
			},
			expected: corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: testDataVolume,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
				},
			},
		},
		{
			name: "custom storage configuration",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Storage: koncachev1alpha1.RedisStorage{
						Size:             resource.MustParse("10Gi"),
						AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
						StorageClassName: func() *string { s := "fast-ssd"; return &s }(),
					},
				},
			},
			expected: corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: testDataVolume,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
					StorageClassName: func() *string { s := "fast-ssd"; return &s }(),
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("10Gi"),
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := controller.BuildVolumeClaimTemplate(tt.redis)

			assert.Equal(t, tt.expected.ObjectMeta.Name, result.ObjectMeta.Name)
			assert.Equal(t, tt.expected.Spec.AccessModes, result.Spec.AccessModes)
			assert.True(t, controller.EqualResourceLists(result.Spec.Resources.Requests, tt.expected.Spec.Resources.Requests))

			// Test storage class name (handling nil pointers)
			if tt.expected.Spec.StorageClassName == nil {
				assert.Nil(t, result.Spec.StorageClassName)
			} else {
				require.NotNil(t, result.Spec.StorageClassName)
				assert.Equal(t, *tt.expected.Spec.StorageClassName, *result.Spec.StorageClassName)
			}
		})
	}
}

func TestBuildConfigMapVolume(t *testing.T) {
	tests := []struct {
		name      string
		redisName string
		expected  corev1.Volume
	}{
		{
			name:      "basic configmap volume",
			redisName: testRedisName,
			expected: corev1.Volume{
				Name: testConfigVolume,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "my-redis-config",
						},
					},
				},
			},
		},
		{
			name:      "configmap volume with complex name",
			redisName: testRedisClusterName,
			expected: corev1.Volume{
				Name: testConfigVolume,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "redis-cluster-prod-config",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := controller.BuildConfigMapVolume(tt.redisName)

			assert.Equal(t, tt.expected.Name, result.Name)
			require.NotNil(t, result.VolumeSource.ConfigMap, "Volume should have ConfigMap volume source")
			assert.Equal(t, tt.expected.VolumeSource.ConfigMap.Name, result.VolumeSource.ConfigMap.Name)
		})
	}
}

func TestEqualResourceLists(t *testing.T) {
	tests := []struct {
		name     string
		a        corev1.ResourceList
		b        corev1.ResourceList
		expected bool
	}{
		{
			name:     "equal empty lists",
			a:        corev1.ResourceList{},
			b:        corev1.ResourceList{},
			expected: true,
		},
		{
			name: "equal lists with same resources",
			a: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
			b: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
			expected: true,
		},
		{
			name: "different resource values",
			a: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("100m"),
			},
			b: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("200m"),
			},
			expected: false,
		},
		{
			name: "different number of resources",
			a: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("100m"),
			},
			b: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
			expected: false,
		},
		{
			name: "missing resource in second list",
			a: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
			b: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("100m"),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := controller.EqualResourceLists(tt.a, tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEqualStringMaps(t *testing.T) {
	tests := []struct {
		name     string
		a        map[string]string
		b        map[string]string
		expected bool
	}{
		{
			name:     "equal empty maps",
			a:        map[string]string{},
			b:        map[string]string{},
			expected: true,
		},
		{
			name: "equal maps with same key-value pairs",
			a: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			b: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			expected: true,
		},
		{
			name: "different values for same keys",
			a: map[string]string{
				"key1": "value1",
			},
			b: map[string]string{
				"key1": "value2",
			},
			expected: false,
		},
		{
			name: "different number of keys",
			a: map[string]string{
				"key1": "value1",
			},
			b: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			expected: false,
		},
		{
			name:     "nil maps",
			a:        nil,
			b:        nil,
			expected: true,
		},
		{
			name:     "one nil map",
			a:        nil,
			b:        map[string]string{},
			expected: true, // Both are considered empty
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := controller.EqualStringMaps(tt.a, tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEqualTolerations(t *testing.T) {
	tests := []struct {
		name     string
		a        []corev1.Toleration
		b        []corev1.Toleration
		expected bool
	}{
		{
			name:     "equal empty slices",
			a:        []corev1.Toleration{},
			b:        []corev1.Toleration{},
			expected: true,
		},
		{
			name: "equal tolerations",
			a: []corev1.Toleration{
				{
					Key:      "redis",
					Operator: corev1.TolerationOpEqual,
					Value:    "true",
					Effect:   corev1.TaintEffectNoSchedule,
				},
			},
			b: []corev1.Toleration{
				{
					Key:      "redis",
					Operator: corev1.TolerationOpEqual,
					Value:    "true",
					Effect:   corev1.TaintEffectNoSchedule,
				},
			},
			expected: true,
		},
		{
			name: "different toleration values",
			a: []corev1.Toleration{
				{
					Key:      "redis",
					Operator: corev1.TolerationOpEqual,
					Value:    "true",
					Effect:   corev1.TaintEffectNoSchedule,
				},
			},
			b: []corev1.Toleration{
				{
					Key:      "redis",
					Operator: corev1.TolerationOpEqual,
					Value:    "false",
					Effect:   corev1.TaintEffectNoSchedule,
				},
			},
			expected: false,
		},
		{
			name: "different number of tolerations",
			a: []corev1.Toleration{
				{
					Key:      "redis",
					Operator: corev1.TolerationOpEqual,
					Value:    "true",
					Effect:   corev1.TaintEffectNoSchedule,
				},
			},
			b:        []corev1.Toleration{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := controller.EqualTolerations(tt.a, tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTolerationsEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        corev1.Toleration
		b        corev1.Toleration
		expected bool
	}{
		{
			name: "equal tolerations",
			a: corev1.Toleration{
				Key:      "redis",
				Operator: corev1.TolerationOpEqual,
				Value:    "true",
				Effect:   corev1.TaintEffectNoSchedule,
			},
			b: corev1.Toleration{
				Key:      "redis",
				Operator: corev1.TolerationOpEqual,
				Value:    "true",
				Effect:   corev1.TaintEffectNoSchedule,
			},
			expected: true,
		},
		{
			name: "different keys",
			a: corev1.Toleration{
				Key:      "redis",
				Operator: corev1.TolerationOpEqual,
				Value:    "true",
				Effect:   corev1.TaintEffectNoSchedule,
			},
			b: corev1.Toleration{
				Key:      "memcached",
				Operator: corev1.TolerationOpEqual,
				Value:    "true",
				Effect:   corev1.TaintEffectNoSchedule,
			},
			expected: false,
		},
		{
			name: "equal tolerations with TolerationSeconds",
			a: corev1.Toleration{
				Key:               "redis",
				Operator:          corev1.TolerationOpEqual,
				Value:             "true",
				Effect:            corev1.TaintEffectNoExecute,
				TolerationSeconds: func() *int64 { i := int64(300); return &i }(),
			},
			b: corev1.Toleration{
				Key:               "redis",
				Operator:          corev1.TolerationOpEqual,
				Value:             "true",
				Effect:            corev1.TaintEffectNoExecute,
				TolerationSeconds: func() *int64 { i := int64(300); return &i }(),
			},
			expected: true,
		},
		{
			name: "different TolerationSeconds",
			a: corev1.Toleration{
				Key:               "redis",
				Operator:          corev1.TolerationOpEqual,
				Value:             "true",
				Effect:            corev1.TaintEffectNoExecute,
				TolerationSeconds: func() *int64 { i := int64(300); return &i }(),
			},
			b: corev1.Toleration{
				Key:               "redis",
				Operator:          corev1.TolerationOpEqual,
				Value:             "true",
				Effect:            corev1.TaintEffectNoExecute,
				TolerationSeconds: func() *int64 { i := int64(600); return &i }(),
			},
			expected: false,
		},
		{
			name: "one nil TolerationSeconds",
			a: corev1.Toleration{
				Key:               "redis",
				Operator:          corev1.TolerationOpEqual,
				Value:             "true",
				Effect:            corev1.TaintEffectNoExecute,
				TolerationSeconds: nil,
			},
			b: corev1.Toleration{
				Key:               "redis",
				Operator:          corev1.TolerationOpEqual,
				Value:             "true",
				Effect:            corev1.TaintEffectNoExecute,
				TolerationSeconds: func() *int64 { i := int64(300); return &i }(),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := controller.TolerationsEqual(tt.a, tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetRedisPort(t *testing.T) {
	tests := []struct {
		name     string
		redis    *koncachev1alpha1.Redis
		expected int32
	}{
		{
			name: "default port when not specified",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Port: 0,
				},
			},
			expected: 6379,
		},
		{
			name: "custom port",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Port: 7000,
				},
			},
			expected: 7000,
		},
		{
			name: "another custom port",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Port: 16379,
				},
			},
			expected: 16379,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := controller.GetRedisPort(tt.redis)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetRedisExporterPort(t *testing.T) {
	tests := []struct {
		name     string
		redis    *koncachev1alpha1.Redis
		expected int32
	}{
		{
			name: "default exporter port when not specified",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Monitoring: koncachev1alpha1.RedisMonitoring{
						Exporter: koncachev1alpha1.RedisExporter{
							Port: 0,
						},
					},
				},
			},
			expected: 9121,
		},
		{
			name: "custom exporter port",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Monitoring: koncachev1alpha1.RedisMonitoring{
						Exporter: koncachev1alpha1.RedisExporter{
							Port: 9090,
						},
					},
				},
			},
			expected: 9090,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := controller.GetRedisExporterPort(tt.redis)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetRedisExporterImage(t *testing.T) {
	tests := []struct {
		name     string
		redis    *koncachev1alpha1.Redis
		expected string
	}{
		{
			name: "default exporter image when not specified",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Monitoring: koncachev1alpha1.RedisMonitoring{
						Exporter: koncachev1alpha1.RedisExporter{
							Image: "",
						},
					},
				},
			},
			expected: "oliver006/redis_exporter:latest",
		},
		{
			name: "custom exporter image",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Monitoring: koncachev1alpha1.RedisMonitoring{
						Exporter: koncachev1alpha1.RedisExporter{
							Image: testRedisExporterImage,
						},
					},
				},
			},
			expected: testRedisExporterImage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := controller.GetRedisExporterImage(tt.redis)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildRedisExporterContainer(t *testing.T) {
	tests := []struct {
		name      string
		redis     *koncachev1alpha1.Redis
		redisPort int32
	}{
		{
			name: "basic exporter container configuration",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Monitoring: koncachev1alpha1.RedisMonitoring{
						Exporter: koncachev1alpha1.RedisExporter{
							Image: testRedisExporterImage,
							Port:  9121,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("50m"),
									corev1.ResourceMemory: resource.MustParse("64Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
							},
						},
					},
				},
			},
			redisPort: 6379,
		},
		{
			name: "custom exporter port configuration",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Monitoring: koncachev1alpha1.RedisMonitoring{
						Exporter: koncachev1alpha1.RedisExporter{
							Port: 9090,
						},
					},
				},
			},
			redisPort: 7000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := controller.BuildRedisExporterContainer(tt.redis, tt.redisPort)

			// Test basic properties
			assert.Equal(t, "redis-exporter", result.Name)
			expectedImage := controller.GetRedisExporterImage(tt.redis)
			assert.Equal(t, expectedImage, result.Image)
			assert.Equal(t, corev1.PullIfNotPresent, result.ImagePullPolicy)

			// Test port configuration
			require.Len(t, result.Ports, 1, "Exporter container should have exactly 1 port")
			expectedPort := controller.GetRedisExporterPort(tt.redis)
			assert.Equal(t, expectedPort, result.Ports[0].ContainerPort)
			assert.Equal(t, "metrics", result.Ports[0].Name)

			// Test environment variables
			require.Len(t, result.Env, 1, "Exporter container should have exactly 1 environment variable")
			assert.Equal(t, "REDIS_ADDR", result.Env[0].Name)
			expectedRedisAddr := fmt.Sprintf("redis://localhost:%d", tt.redisPort)
			assert.Equal(t, expectedRedisAddr, result.Env[0].Value)

			// Test probes are present
			assert.NotNil(t, result.LivenessProbe, "Exporter container should have liveness probe")
			assert.NotNil(t, result.ReadinessProbe, "Exporter container should have readiness probe")

			// Test probe configuration
			assert.Equal(t, "/health", result.LivenessProbe.HTTPGet.Path)
			assert.Equal(t, "/health", result.ReadinessProbe.HTTPGet.Path)
		})
	}
}

func TestBuildRedisExporterContainerWithTLS(t *testing.T) {
	redis := &koncachev1alpha1.Redis{
		Spec: koncachev1alpha1.RedisSpec{
			Monitoring: koncachev1alpha1.RedisMonitoring{
				Exporter: koncachev1alpha1.RedisExporter{
					Image: testRedisExporterImage,
					Port:  9121,
				},
			},
			Security: koncachev1alpha1.RedisSecurity{
				TLS: &koncachev1alpha1.RedisTLS{
					Enabled:    true,
					CertSecret: testTLSCertsSecret,
				},
			},
		},
	}

	result := controller.BuildRedisExporterContainer(redis, 6379)

	// Test basic properties
	assert.Equal(t, "redis-exporter", result.Name)
	assert.Equal(t, testRedisExporterImage, result.Image)

	// Test REDIS_ADDR environment variable uses TLS
	redisAddrFound := false
	for _, env := range result.Env {
		if env.Name == "REDIS_ADDR" {
			redisAddrFound = true
			assert.Equal(t, "rediss://localhost:6380", env.Value)
			break
		}
	}
	assert.True(t, redisAddrFound, "REDIS_ADDR environment variable should be present")

	// Test TLS environment variables
	tlsCertFound := false
	tlsKeyFound := false
	for _, env := range result.Env {
		if env.Name == "REDIS_EXPORTER_TLS_CLIENT_CERT_FILE" {
			tlsCertFound = true
			assert.Equal(t, "/etc/redis/tls/tls.crt", env.Value)
		}
		if env.Name == "REDIS_EXPORTER_TLS_CLIENT_KEY_FILE" {
			tlsKeyFound = true
			assert.Equal(t, "/etc/redis/tls/tls.key", env.Value)
		}
	}
	assert.True(t, tlsCertFound, "TLS cert file environment variable should be present when TLS is enabled")
	assert.True(t, tlsKeyFound, "TLS key file environment variable should be present when TLS is enabled")

	// Test TLS volume mounts
	tlsVolumeMountFound := false
	for _, mount := range result.VolumeMounts {
		if mount.Name == controller.TLSCertsVolumeName {
			tlsVolumeMountFound = true
			assert.Equal(t, "/etc/redis/tls", mount.MountPath)
			assert.True(t, mount.ReadOnly)
			break
		}
	}
	assert.True(t, tlsVolumeMountFound, "TLS volume mount should be present when TLS is enabled")
}

func TestIsMonitoringEnabled(t *testing.T) {
	tests := []struct {
		name     string
		redis    *koncachev1alpha1.Redis
		expected bool
	}{
		{
			name: "monitoring and exporter enabled",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Monitoring: koncachev1alpha1.RedisMonitoring{
						Enabled: true,
						Exporter: koncachev1alpha1.RedisExporter{
							Enabled: true,
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "monitoring enabled but exporter disabled",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Monitoring: koncachev1alpha1.RedisMonitoring{
						Enabled: true,
						Exporter: koncachev1alpha1.RedisExporter{
							Enabled: false,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "monitoring disabled",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Monitoring: koncachev1alpha1.RedisMonitoring{
						Enabled: false,
						Exporter: koncachev1alpha1.RedisExporter{
							Enabled: true,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "both monitoring and exporter disabled",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Monitoring: koncachev1alpha1.RedisMonitoring{
						Enabled: false,
						Exporter: koncachev1alpha1.RedisExporter{
							Enabled: false,
						},
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := controller.IsMonitoringEnabled(tt.redis)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Security-related tests

func TestIsSecurityEnabled(t *testing.T) {
	tests := []struct {
		name     string
		redis    *koncachev1alpha1.Redis
		expected bool
	}{
		{
			name: "no security enabled",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{},
			},
			expected: false,
		},
		{
			name: testAuthEnabledCase,
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Security: koncachev1alpha1.RedisSecurity{
						RequireAuth: &[]bool{true}[0],
					},
				},
			},
			expected: true,
		},
		{
			name: testTLSEnabledCase,
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Security: koncachev1alpha1.RedisSecurity{
						TLS: &koncachev1alpha1.RedisTLS{
							Enabled: true,
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "command renaming enabled",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Security: koncachev1alpha1.RedisSecurity{
						RenameCommands: map[string]string{
							"FLUSHALL": "SECURE_FLUSHALL",
						},
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := controller.IsSecurityEnabled(tt.redis)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsTLSEnabled(t *testing.T) {
	tests := []struct {
		name     string
		redis    *koncachev1alpha1.Redis
		expected bool
	}{
		{
			name: "TLS not configured",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{},
			},
			expected: false,
		},
		{
			name: "TLS disabled",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Security: koncachev1alpha1.RedisSecurity{
						TLS: &koncachev1alpha1.RedisTLS{
							Enabled: false,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: testTLSEnabledCase,
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Security: koncachev1alpha1.RedisSecurity{
						TLS: &koncachev1alpha1.RedisTLS{
							Enabled: true,
						},
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := controller.IsTLSEnabled(tt.redis)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsAuthEnabled(t *testing.T) {
	tests := []struct {
		name     string
		redis    *koncachev1alpha1.Redis
		expected bool
	}{
		{
			name: "auth not configured",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{},
			},
			expected: false,
		},
		{
			name: "auth disabled",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Security: koncachev1alpha1.RedisSecurity{
						RequireAuth: &[]bool{false}[0],
					},
				},
			},
			expected: false,
		},
		{
			name: testAuthEnabledCase,
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Security: koncachev1alpha1.RedisSecurity{
						RequireAuth: &[]bool{true}[0],
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := controller.IsAuthEnabled(tt.redis)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetPasswordSecretName(t *testing.T) {
	tests := []struct {
		name     string
		redis    *koncachev1alpha1.Redis
		expected string
	}{
		{
			name: "no password secret configured",
			redis: &koncachev1alpha1.Redis{
				ObjectMeta: metav1.ObjectMeta{
					Name: testRedisInstance,
				},
				Spec: koncachev1alpha1.RedisSpec{},
			},
			expected: "test-redis-auth",
		},
		{
			name: "custom password secret configured",
			redis: &koncachev1alpha1.Redis{
				ObjectMeta: metav1.ObjectMeta{
					Name: testRedisInstance,
				},
				Spec: koncachev1alpha1.RedisSpec{
					Security: koncachev1alpha1.RedisSecurity{
						PasswordSecret: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "custom-secret",
							},
							Key: "password",
						},
					},
				},
			},
			expected: "custom-secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := controller.GetPasswordSecretName(tt.redis)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildSecurityConfig(t *testing.T) {
	tests := []struct {
		name     string
		redis    *koncachev1alpha1.Redis
		contains []string
	}{
		{
			name: "no security",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{},
			},
			contains: []string{"protected-mode no"},
		},
		{
			name: testAuthEnabledCase,
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Security: koncachev1alpha1.RedisSecurity{
						RequireAuth: &[]bool{true}[0],
					},
				},
			},
			contains: []string{
				"protected-mode yes",
				"requirepass $REDIS_PASSWORD",
			},
		},
		{
			name: testTLSEnabledCase,
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Security: koncachev1alpha1.RedisSecurity{
						TLS: &koncachev1alpha1.RedisTLS{
							Enabled: true,
						},
					},
				},
			},
			contains: []string{
				"tls-port 6380",
				"port 0",
				"tls-cert-file /etc/redis/tls/tls.crt",
				"tls-key-file /etc/redis/tls/tls.key",
				"tls-auth-clients yes",
			},
		},
		{
			name: "command renaming",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Security: koncachev1alpha1.RedisSecurity{
						RenameCommands: map[string]string{
							"FLUSHALL": "SECURE_FLUSHALL",
							"DEBUG":    "",
						},
					},
				},
			},
			contains: []string{
				"rename-command FLUSHALL SECURE_FLUSHALL",
				"rename-command DEBUG \"\"",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := controller.BuildSecurityConfig(tt.redis)
			for _, expectedSubstring := range tt.contains {
				assert.Contains(t, result, expectedSubstring)
			}
		})
	}
}

func TestBuildSecurityEnvironment(t *testing.T) {
	tests := []struct {
		name     string
		redis    *koncachev1alpha1.Redis
		expected int
	}{
		{
			name: "no auth",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{},
			},
			expected: 0,
		},
		{
			name: testAuthEnabledCase,
			redis: &koncachev1alpha1.Redis{
				ObjectMeta: metav1.ObjectMeta{
					Name: testRedisInstance,
				},
				Spec: koncachev1alpha1.RedisSpec{
					Security: koncachev1alpha1.RedisSecurity{
						RequireAuth: &[]bool{true}[0],
					},
				},
			},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := controller.BuildSecurityEnvironment(tt.redis)
			assert.Len(t, result, tt.expected)

			if tt.expected > 0 {
				assert.Equal(t, "REDIS_PASSWORD", result[0].Name)
				assert.NotNil(t, result[0].ValueFrom)
				assert.NotNil(t, result[0].ValueFrom.SecretKeyRef)
			}
		})
	}
}

func TestBuildTLSVolumes(t *testing.T) {
	tests := []struct {
		name     string
		redis    *koncachev1alpha1.Redis
		expected int
	}{
		{
			name: "TLS disabled",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{},
			},
			expected: 0,
		},
		{
			name: "TLS enabled with cert secret",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Security: koncachev1alpha1.RedisSecurity{
						TLS: &koncachev1alpha1.RedisTLS{
							Enabled:    true,
							CertSecret: testTLSCertsSecret,
						},
					},
				},
			},
			expected: 1,
		},
		{
			name: "TLS enabled with separate CA secret",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Security: koncachev1alpha1.RedisSecurity{
						TLS: &koncachev1alpha1.RedisTLS{
							Enabled:    true,
							CertSecret: testTLSCertsSecret,
							CASecret:   "ca-certs",
						},
					},
				},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := controller.BuildTLSVolumes(tt.redis)
			assert.Len(t, result, tt.expected)
		})
	}
}

func TestBuildRedisContainerWithSecurity(t *testing.T) {
	tests := []struct {
		name      string
		redis     *koncachev1alpha1.Redis
		checkFunc func(*testing.T, corev1.Container)
	}{
		{
			name: "container with auth",
			redis: &koncachev1alpha1.Redis{
				ObjectMeta: metav1.ObjectMeta{
					Name: testRedisInstance,
				},
				Spec: koncachev1alpha1.RedisSpec{
					Image: "redis:7.2",
					Security: koncachev1alpha1.RedisSecurity{
						RequireAuth: &[]bool{true}[0],
					},
				},
			},
			checkFunc: func(t *testing.T, container corev1.Container) {
				assert.Len(t, container.Env, 1)
				assert.Equal(t, "REDIS_PASSWORD", container.Env[0].Name)
			},
		},
		{
			name: "container with TLS",
			redis: &koncachev1alpha1.Redis{
				ObjectMeta: metav1.ObjectMeta{
					Name: testRedisInstance,
				},
				Spec: koncachev1alpha1.RedisSpec{
					Image: "redis:7.2",
					Security: koncachev1alpha1.RedisSecurity{
						TLS: &koncachev1alpha1.RedisTLS{
							Enabled:    true,
							CertSecret: testTLSCertsSecret,
						},
					},
				},
			},
			checkFunc: func(t *testing.T, container corev1.Container) {
				// Should have exactly 1 port (TLS port only)
				require.Len(t, container.Ports, 1, "Container should have exactly 1 port when TLS is enabled")

				// Should have TLS port
				assert.Equal(t, "redis-tls", container.Ports[0].Name)
				assert.Equal(t, int32(6380), container.Ports[0].ContainerPort)

				// Should NOT have regular Redis port
				for _, port := range container.Ports {
					assert.NotEqual(t, "redis", port.Name, "Should not have regular redis port when TLS is enabled")
				}

				// Should have TLS volume mount
				found := false
				for _, mount := range container.VolumeMounts {
					if mount.Name == controller.TLSCertsVolumeName {
						found = true
						break
					}
				}
				assert.True(t, found, "Should have TLS volume mount")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			container := controller.BuildRedisContainer(tt.redis, 6379)
			tt.checkFunc(t, container)
		})
	}
}
