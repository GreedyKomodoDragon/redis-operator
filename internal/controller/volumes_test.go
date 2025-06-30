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
