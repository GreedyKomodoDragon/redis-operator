package controller_test

import (
	"testing"

	koncachev1alpha1 "github.com/GreedyKomodoDragon/redis-operator/api/v1alpha1"
	"github.com/GreedyKomodoDragon/redis-operator/internal/controller"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

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
