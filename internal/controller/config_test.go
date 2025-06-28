package controller_test

import (
	"testing"

	koncachev1alpha1 "github.com/GreedyKomodoDragon/redis-operator/api/v1alpha1"
	"github.com/GreedyKomodoDragon/redis-operator/internal/controller"
	"github.com/stretchr/testify/assert"
)

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
