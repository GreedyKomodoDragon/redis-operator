package controller_test

import (
	"testing"

	koncachev1alpha1 "github.com/GreedyKomodoDragon/redis-operator/api/v1alpha1"
	"github.com/GreedyKomodoDragon/redis-operator/internal/controller"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
			contains: []string{protectedModeNo},
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
				protectedModeYes,
				requirepassConfig,
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
			name: "auth enabled with HA",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Security: koncachev1alpha1.RedisSecurity{
						RequireAuth: &[]bool{true}[0],
					},
					HighAvailability: &koncachev1alpha1.RedisHighAvailability{
						Enabled:  true,
						Replicas: 3,
					},
				},
			},
			contains: []string{
				protectedModeYes,
				requirepassConfig,
				masterauthConfig,
			},
		},
		{
			name: "auth enabled without HA",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Security: koncachev1alpha1.RedisSecurity{
						RequireAuth: &[]bool{true}[0],
					},
				},
			},
			contains: []string{
				protectedModeYes,
				requirepassConfig,
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

func TestBuildAuthConfigMasterAuthOnlyForHA(t *testing.T) {
	tests := []struct {
		name                    string
		redis                   *koncachev1alpha1.Redis
		shouldContainMasterAuth bool
	}{
		{
			name: "auth enabled with HA should include masterauth",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Security: koncachev1alpha1.RedisSecurity{
						RequireAuth: &[]bool{true}[0],
					},
					HighAvailability: &koncachev1alpha1.RedisHighAvailability{
						Enabled:  true,
						Replicas: 3,
					},
				},
			},
			shouldContainMasterAuth: true,
		},
		{
			name: "auth enabled without HA should not include masterauth",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Security: koncachev1alpha1.RedisSecurity{
						RequireAuth: &[]bool{true}[0],
					},
				},
			},
			shouldContainMasterAuth: false,
		},
		{
			name: "auth enabled with HA disabled should not include masterauth",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Security: koncachev1alpha1.RedisSecurity{
						RequireAuth: &[]bool{true}[0],
					},
					HighAvailability: &koncachev1alpha1.RedisHighAvailability{
						Enabled:  false,
						Replicas: 3,
					},
				},
			},
			shouldContainMasterAuth: false,
		},
		{
			name: "no auth should not include masterauth",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					HighAvailability: &koncachev1alpha1.RedisHighAvailability{
						Enabled:  true,
						Replicas: 3,
					},
				},
			},
			shouldContainMasterAuth: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := controller.BuildSecurityConfig(tt.redis)

			if tt.shouldContainMasterAuth {
				assert.Contains(t, result, masterauthConfig, "Should contain masterauth for HA deployments with auth")
			} else {
				assert.NotContains(t, result, masterauthConfig, "Should not contain masterauth for non-HA or non-auth deployments")
			}
		})
	}
}
