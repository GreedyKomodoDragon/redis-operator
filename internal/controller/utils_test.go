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
