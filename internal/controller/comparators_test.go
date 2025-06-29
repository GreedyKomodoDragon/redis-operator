package controller_test

import (
	"testing"

	"github.com/GreedyKomodoDragon/redis-operator/internal/controller"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

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

func TestComputeStringHash(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "empty string",
			content:  "",
			expected: "",
		},
		{
			name:    "simple redis config",
			content: "port 6379\nmaxmemory 512mb",
			// We'll verify consistency rather than exact value for non-empty strings
		},
		{
			name:    "complex redis config",
			content: "# Redis configuration\nport 6379\nmaxmemory 1gb\nmaxmemory-policy allkeys-lru\nsave 900 1\nsave 300 10\nappendonly yes",
		},
		{
			name:    "config with special characters",
			content: "requirepass \"my-$ecret-p@ssw0rd!\"\ntcp-backlog 511",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := controller.ComputeStringHash(tt.content)

			if tt.content == "" {
				assert.Equal(t, "", result, "Empty string should return empty hash")
			} else {
				assert.Len(t, result, 16, "Hash should be 16 characters long")
				assert.Regexp(t, "^[a-f0-9]+$", result, "Hash should be lowercase hex")
			}
		})
	}
}

func TestComputeStringHashConsistency(t *testing.T) {
	// Test that the same string produces the same hash
	content := "port 6379\nmaxmemory 512mb\nmaxmemory-policy allkeys-lru"

	hash1 := controller.ComputeStringHash(content)
	hash2 := controller.ComputeStringHash(content)

	assert.Equal(t, hash1, hash2, "Same string should produce same hash")
	assert.Len(t, hash1, 16, "Hash should be 16 characters long")
}
