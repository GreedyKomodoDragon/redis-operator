package controller_test

import (
	"testing"

	koncachev1alpha1 "github.com/GreedyKomodoDragon/redis-operator/api/v1alpha1"
	"github.com/GreedyKomodoDragon/redis-operator/internal/controller"
	"github.com/stretchr/testify/assert"
)

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
