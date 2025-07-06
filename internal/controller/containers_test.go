package controller_test

import (
	"testing"

	koncachev1alpha1 "github.com/GreedyKomodoDragon/redis-operator/api/v1alpha1"
	"github.com/GreedyKomodoDragon/redis-operator/internal/controller"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
			expectedCommand := []string{"sh", "-c"}
			assert.Equal(t, expectedCommand, result.Command)

			// Test that args contain the Redis startup script with environment variable substitution
			require.Len(t, result.Args, 1, "Container should have exactly 1 arg")
			assert.Contains(t, result.Args[0], "sed 's/\\$REDIS_PASSWORD/'", "Args should contain password substitution logic")
			assert.Contains(t, result.Args[0], "redis-server /tmp/redis.conf", "Args should contain redis-server command")
		})
	}
}

func TestBuildBackupContainer(t *testing.T) {
	tests := []struct {
		name            string
		redis           *koncachev1alpha1.Redis
		expectedEnvVars []string
	}{
		{
			name: "basic backup container without S3",
			redis: &koncachev1alpha1.Redis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-redis",
					Namespace: "default",
				},
				Spec: koncachev1alpha1.RedisSpec{
					Image:           "redis:7.2-alpine",
					ImagePullPolicy: corev1.PullIfNotPresent,
					Backup: koncachev1alpha1.RedisBackup{
						Enabled: true,
						Image:   "koncache/redis-backup:latest",
						Storage: koncachev1alpha1.RedisBackupStorage{
							// No S3 configuration
						},
					},
				},
			},
			expectedEnvVars: []string{
				"REDIS_HOST", "REDIS_PORT", "REDIS_DB",
				"MAX_RETRIES", "RETRY_DELAY_SECONDS", "MAX_RETRY_DELAY_SECONDS",
			},
		},
		{
			name: "backup container with S3 storage",
			redis: &koncachev1alpha1.Redis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-redis",
					Namespace: "default",
				},
				Spec: koncachev1alpha1.RedisSpec{
					Image:           "redis:7.2-alpine",
					ImagePullPolicy: corev1.PullIfNotPresent,
					Backup: koncachev1alpha1.RedisBackup{
						Enabled: true,
						Image:   "koncache/redis-backup:latest",
						Storage: koncachev1alpha1.RedisBackupStorage{
							Type: "s3",
							S3: &koncachev1alpha1.RedisS3Storage{
								Bucket:     "test-bucket",
								Region:     "us-west-2",
								Prefix:     "backups/",
								SecretName: "s3-credentials",
							},
						},
					},
				},
			},
			expectedEnvVars: []string{
				"REDIS_HOST", "REDIS_PORT", "REDIS_DB",
				"MAX_RETRIES", "RETRY_DELAY_SECONDS", "MAX_RETRY_DELAY_SECONDS",
				"S3_BUCKET", "AWS_REGION", "S3_PREFIX", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY",
			},
		},
		{
			name: "backup container with auth and TLS",
			redis: &koncachev1alpha1.Redis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-redis",
					Namespace: "default",
				},
				Spec: koncachev1alpha1.RedisSpec{
					Image:           "redis:7.2-alpine",
					ImagePullPolicy: corev1.PullIfNotPresent,
					Security: koncachev1alpha1.RedisSecurity{
						RequireAuth: &[]bool{true}[0],
						PasswordSecret: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "redis-password",
							},
							Key: "password",
						},
						TLS: &koncachev1alpha1.RedisTLS{
							Enabled:    true,
							CertSecret: "redis-tls",
						},
					},
					Backup: koncachev1alpha1.RedisBackup{
						Enabled: true,
						Image:   "koncache/redis-backup:latest",
						Storage: koncachev1alpha1.RedisBackupStorage{
							Type: "s3",
							S3: &koncachev1alpha1.RedisS3Storage{
								Bucket:     "test-bucket",
								Region:     "us-east-1",
								SecretName: "s3-credentials",
							},
						},
					},
				},
			},
			expectedEnvVars: []string{
				"REDIS_HOST", "REDIS_PORT", "REDIS_DB", "REDIS_PASSWORD", "REDIS_TLS_ENABLED",
				"MAX_RETRIES", "RETRY_DELAY_SECONDS", "MAX_RETRY_DELAY_SECONDS",
				"S3_BUCKET", "AWS_REGION", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := controller.BuildBackupContainer(tt.redis)

			// Test basic properties
			assert.Equal(t, "redis-backup", result.Name)
			assert.Equal(t, tt.redis.Spec.Backup.Image, result.Image)
			assert.Equal(t, tt.redis.Spec.ImagePullPolicy, result.ImagePullPolicy)

			// Test environment variables
			envVarNames := make([]string, len(result.Env))
			for i, env := range result.Env {
				envVarNames[i] = env.Name
			}

			for _, expectedEnv := range tt.expectedEnvVars {
				assert.Contains(t, envVarNames, expectedEnv, "Environment variable %s should be present", expectedEnv)
			}

			// Test specific environment variable values
			envVarMap := make(map[string]corev1.EnvVar)
			for _, env := range result.Env {
				envVarMap[env.Name] = env
			}

			// Verify Redis connection details
			expectedRedisHost := "test-redis.default.svc.cluster.local"
			assert.Equal(t, expectedRedisHost, envVarMap["REDIS_HOST"].Value)

			// Verify Redis port (6380 for TLS, 6379 for non-TLS)
			expectedPort := "6379"
			if tt.redis.Spec.Security.TLS != nil && tt.redis.Spec.Security.TLS.Enabled {
				expectedPort = "6380"
			}
			assert.Equal(t, expectedPort, envVarMap["REDIS_PORT"].Value)
			assert.Equal(t, "0", envVarMap["REDIS_DB"].Value)

			// Verify retry configuration
			assert.Equal(t, "5", envVarMap["MAX_RETRIES"].Value)
			assert.Equal(t, "5", envVarMap["RETRY_DELAY_SECONDS"].Value)
			assert.Equal(t, "300", envVarMap["MAX_RETRY_DELAY_SECONDS"].Value)
			// BACKUP_DIR is no longer used for streaming backup

			// Verify S3 configuration if applicable
			if tt.redis.Spec.Backup.Storage.Type == "s3" && tt.redis.Spec.Backup.Storage.S3 != nil {
				s3Config := tt.redis.Spec.Backup.Storage.S3
				assert.Equal(t, s3Config.Bucket, envVarMap["S3_BUCKET"].Value)
				if s3Config.Region != "" {
					assert.Equal(t, s3Config.Region, envVarMap["AWS_REGION"].Value)
				}
				if s3Config.Prefix != "" {
					assert.Equal(t, s3Config.Prefix, envVarMap["S3_PREFIX"].Value)
				}
				if s3Config.SecretName != "" {
					assert.NotNil(t, envVarMap["AWS_ACCESS_KEY_ID"].ValueFrom)
					assert.NotNil(t, envVarMap["AWS_SECRET_ACCESS_KEY"].ValueFrom)
					assert.Equal(t, s3Config.SecretName, envVarMap["AWS_ACCESS_KEY_ID"].ValueFrom.SecretKeyRef.Name)
					assert.Equal(t, s3Config.SecretName, envVarMap["AWS_SECRET_ACCESS_KEY"].ValueFrom.SecretKeyRef.Name)
				}
			}

			// Verify TLS configuration if applicable
			if tt.redis.Spec.Security.TLS != nil && tt.redis.Spec.Security.TLS.Enabled {
				assert.Equal(t, "true", envVarMap["REDIS_TLS_ENABLED"].Value)
			}

			// Verify password configuration if applicable
			if tt.redis.Spec.Security.RequireAuth != nil && *tt.redis.Spec.Security.RequireAuth {
				assert.NotNil(t, envVarMap["REDIS_PASSWORD"].ValueFrom)
				assert.NotNil(t, envVarMap["REDIS_PASSWORD"].ValueFrom.SecretKeyRef)
			}

			// Test volume mounts - no longer needed for streaming backup
			require.Len(t, result.VolumeMounts, 0, "Backup container should have no volume mounts for streaming backup")
		})
	}
}
