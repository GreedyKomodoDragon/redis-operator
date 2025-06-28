package controller_test

import (
	"testing"

	"github.com/GreedyKomodoDragon/redis-operator/internal/controller"
	"github.com/stretchr/testify/assert"
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
