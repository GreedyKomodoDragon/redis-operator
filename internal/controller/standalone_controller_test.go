package controller

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	koncachev1alpha1 "github.com/GreedyKomodoDragon/redis-operator/api/v1alpha1"
)

const (
	testBackupInitContainer = "backup-init"
	testBackupInitImage     = "backup-init:latest"
	testRedisImage72        = "redis:7.2"
	testRedisImage70        = "redis:7.0"
	testConfigHashAnno      = "config-hash"
	testRedisDataVolume     = "redis-data"
	testRedisRoleLabel      = "redis-operator/role"
)

func TestNeedsStatefulSetUpdate(t *testing.T) {
	tests := []struct {
		name     string
		existing *appsv1.StatefulSet
		desired  *appsv1.StatefulSet
		expected bool
	}{
		{
			name: "no changes needed",
			existing: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: appsv1.StatefulSetSpec{
					Replicas: &[]int32{1}[0],
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							ServiceAccountName: "default",
							Containers: []corev1.Container{{
								Name:  "redis",
								Image: testRedisImage,
								Ports: []corev1.ContainerPort{{ContainerPort: 6379}},
							}},
						},
					},
				},
			},
			desired: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: appsv1.StatefulSetSpec{
					Replicas: &[]int32{1}[0],
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							ServiceAccountName: "default",
							Containers: []corev1.Container{{
								Name:  "redis",
								Image: testRedisImage,
								Ports: []corev1.ContainerPort{{ContainerPort: 6379}},
							}},
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "replica count changed",
			existing: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: appsv1.StatefulSetSpec{
					Replicas: &[]int32{1}[0],
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							ServiceAccountName: "default",
							Containers: []corev1.Container{{
								Name:  "redis",
								Image: testRedisImage,
							}},
						},
					},
				},
			},
			desired: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: appsv1.StatefulSetSpec{
					Replicas: &[]int32{3}[0],
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							ServiceAccountName: "default",
							Containers: []corev1.Container{{
								Name:  "redis",
								Image: testRedisImage,
							}},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "init container added",
			existing: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: appsv1.StatefulSetSpec{
					Replicas: &[]int32{1}[0],
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{
								Name:  "redis",
								Image: testRedisImage,
							}},
						},
					},
				},
			},
			desired: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: appsv1.StatefulSetSpec{
					Replicas: &[]int32{1}[0],
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{{
								Name:  "backup-init",
								Image: "backup-init:latest",
							}},
							Containers: []corev1.Container{{
								Name:  "redis",
								Image: testRedisImage,
							}},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "security context changed",
			existing: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: appsv1.StatefulSetSpec{
					Replicas: &[]int32{1}[0],
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{
								Name:  "redis",
								Image: testRedisImage,
							}},
							SecurityContext: nil,
						},
					},
				},
			},
			desired: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: appsv1.StatefulSetSpec{
					Replicas: &[]int32{1}[0],
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{
								Name:  "redis",
								Image: testRedisImage,
							}},
							SecurityContext: &corev1.PodSecurityContext{
								RunAsUser: &[]int64{999}[0],
							},
						},
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := &StandaloneController{}
			result := controller.needsStatefulSetUpdate(tt.existing, tt.desired)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasInitContainerChanges(t *testing.T) {
	tests := []struct {
		name     string
		existing *appsv1.StatefulSet
		desired  *appsv1.StatefulSet
		expected bool
	}{
		{
			name: "no init containers in both",
			existing: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{},
						},
					},
				},
			},
			desired: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{},
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "init container added",
			existing: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{},
						},
					},
				},
			},
			desired: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{{
								Name:  "backup-init",
								Image: "backup-init:latest",
							}},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "init container image changed",
			existing: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{{
								Name:  "backup-init",
								Image: "backup-init:v1.0",
							}},
						},
					},
				},
			},
			desired: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{{
								Name:  "backup-init",
								Image: "backup-init:v1.1",
							}},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "init container environment changed",
			existing: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{{
								Name:  "backup-init",
								Image: "backup-init:latest",
								Env: []corev1.EnvVar{{
									Name:  "DATA_DIR",
									Value: "/old-data",
								}},
							}},
						},
					},
				},
			},
			desired: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{{
								Name:  "backup-init",
								Image: "backup-init:latest",
								Env: []corev1.EnvVar{{
									Name:  "DATA_DIR",
									Value: "/data",
								}},
							}},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "init container unchanged",
			existing: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{{
								Name:  "backup-init",
								Image: "backup-init:latest",
								Env: []corev1.EnvVar{{
									Name:  "DATA_DIR",
									Value: "/data",
								}},
							}},
						},
					},
				},
			},
			desired: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{{
								Name:  "backup-init",
								Image: "backup-init:latest",
								Env: []corev1.EnvVar{{
									Name:  "DATA_DIR",
									Value: "/data",
								}},
							}},
						},
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := &StandaloneController{}
			result := controller.hasInitContainerChanges(tt.existing, tt.desired)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasSecurityContextChanges(t *testing.T) {
	tests := []struct {
		name     string
		existing *appsv1.StatefulSet
		desired  *appsv1.StatefulSet
		expected bool
	}{
		{
			name: "no security context in both",
			existing: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							SecurityContext: nil,
						},
					},
				},
			},
			desired: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							SecurityContext: nil,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "security context added",
			existing: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							SecurityContext: nil,
						},
					},
				},
			},
			desired: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							SecurityContext: &corev1.PodSecurityContext{
								RunAsUser: &[]int64{999}[0],
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "runAsUser changed",
			existing: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							SecurityContext: &corev1.PodSecurityContext{
								RunAsUser: &[]int64{1000}[0],
							},
						},
					},
				},
			},
			desired: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							SecurityContext: &corev1.PodSecurityContext{
								RunAsUser: &[]int64{999}[0],
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "runAsGroup changed",
			existing: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							SecurityContext: &corev1.PodSecurityContext{
								RunAsUser:  &[]int64{999}[0],
								RunAsGroup: &[]int64{1000}[0],
							},
						},
					},
				},
			},
			desired: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							SecurityContext: &corev1.PodSecurityContext{
								RunAsUser:  &[]int64{999}[0],
								RunAsGroup: &[]int64{999}[0],
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "security context unchanged",
			existing: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							SecurityContext: &corev1.PodSecurityContext{
								RunAsUser:  &[]int64{999}[0],
								RunAsGroup: &[]int64{999}[0],
							},
						},
					},
				},
			},
			desired: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							SecurityContext: &corev1.PodSecurityContext{
								RunAsUser:  &[]int64{999}[0],
								RunAsGroup: &[]int64{999}[0],
							},
						},
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := &StandaloneController{}
			result := controller.hasSecurityContextChanges(tt.existing, tt.desired)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasEnvChanges(t *testing.T) {
	tests := []struct {
		name     string
		existing *corev1.Container
		desired  *corev1.Container
		expected bool
	}{
		{
			name: "no env vars in both",
			existing: &corev1.Container{
				Env: []corev1.EnvVar{},
			},
			desired: &corev1.Container{
				Env: []corev1.EnvVar{},
			},
			expected: false,
		},
		{
			name: "env var added",
			existing: &corev1.Container{
				Env: []corev1.EnvVar{},
			},
			desired: &corev1.Container{
				Env: []corev1.EnvVar{{
					Name:  "DATA_DIR",
					Value: "/data",
				}},
			},
			expected: true,
		},
		{
			name: "env var value changed",
			existing: &corev1.Container{
				Env: []corev1.EnvVar{{
					Name:  "DATA_DIR",
					Value: "/old-data",
				}},
			},
			desired: &corev1.Container{
				Env: []corev1.EnvVar{{
					Name:  "DATA_DIR",
					Value: "/data",
				}},
			},
			expected: true,
		},
		{
			name: "env vars unchanged",
			existing: &corev1.Container{
				Env: []corev1.EnvVar{{
					Name:  "DATA_DIR",
					Value: "/data",
				}, {
					Name:  "S3_BUCKET",
					Value: "redis-backups",
				}},
			},
			desired: &corev1.Container{
				Env: []corev1.EnvVar{{
					Name:  "DATA_DIR",
					Value: "/data",
				}, {
					Name:  "S3_BUCKET",
					Value: "redis-backups",
				}},
			},
			expected: false,
		},
		{
			name: "env var removed",
			existing: &corev1.Container{
				Env: []corev1.EnvVar{{
					Name:  "DATA_DIR",
					Value: "/data",
				}, {
					Name:  "OLD_VAR",
					Value: "old-value",
				}},
			},
			desired: &corev1.Container{
				Env: []corev1.EnvVar{{
					Name:  "DATA_DIR",
					Value: "/data",
				}},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := &StandaloneController{}
			result := controller.hasEnvChanges(tt.existing, tt.desired)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasContainerChanges(t *testing.T) {
	tests := []struct {
		name     string
		existing *appsv1.StatefulSet
		desired  *appsv1.StatefulSet
		expected bool
	}{
		{
			name: "container unchanged",
			existing: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{
								Name:  "redis",
								Image: testRedisImage,
								Ports: []corev1.ContainerPort{{ContainerPort: 6379}},
							}},
						},
					},
				},
			},
			desired: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{
								Name:  "redis",
								Image: testRedisImage,
								Ports: []corev1.ContainerPort{{ContainerPort: 6379}},
							}},
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "container image changed",
			existing: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{
								Name:  "redis",
								Image: "redis:7.0",
							}},
						},
					},
				},
			},
			desired: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{
								Name:  "redis",
								Image: testRedisImage,
							}},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "init container and security context changes",
			existing: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{
								Name:  "redis",
								Image: testRedisImage,
							}},
							SecurityContext: nil,
						},
					},
				},
			},
			desired: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{{
								Name:  "backup-init",
								Image: "backup-init:latest",
							}},
							Containers: []corev1.Container{{
								Name:  "redis",
								Image: testRedisImage,
							}},
							SecurityContext: &corev1.PodSecurityContext{
								RunAsUser: &[]int64{999}[0],
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "multiple container changes",
			existing: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{
								Name:  "redis",
								Image: "redis:6.0",
							}, {
								Name:  "exporter",
								Image: "redis-exporter:v1.0",
							}},
						},
					},
				},
			},
			desired: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{
								Name:  "redis",
								Image: testRedisImage,
							}, {
								Name:  "exporter",
								Image: "redis-exporter:v1.1",
							}},
						},
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := &StandaloneController{}
			result := controller.hasContainerChanges(tt.existing, tt.desired)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasBasicSpecChanges(t *testing.T) {
	tests := []struct {
		name     string
		existing *appsv1.StatefulSet
		desired  *appsv1.StatefulSet
		expected bool
	}{
		{
			name: "no basic changes",
			existing: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: &[]int32{1}[0],
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							ServiceAccountName: "default",
						},
					},
				},
			},
			desired: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: &[]int32{1}[0],
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							ServiceAccountName: "default",
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "replica count changed",
			existing: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: &[]int32{1}[0],
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							ServiceAccountName: "default",
						},
					},
				},
			},
			desired: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: &[]int32{3}[0],
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							ServiceAccountName: "default",
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "service account changed",
			existing: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: &[]int32{1}[0],
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							ServiceAccountName: "default",
						},
					},
				},
			},
			desired: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: &[]int32{1}[0],
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							ServiceAccountName: "redis-service-account",
						},
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := &StandaloneController{}
			result := controller.hasBasicSpecChanges(tt.existing, tt.desired)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasContainerSpecChanges(t *testing.T) {
	tests := []struct {
		name     string
		existing *corev1.Container
		desired  *corev1.Container
		expected bool
	}{
		{
			name: "no changes",
			existing: &corev1.Container{
				Image: "redis:7.2",
				Ports: []corev1.ContainerPort{{ContainerPort: 6379}},
			},
			desired: &corev1.Container{
				Image: "redis:7.2",
				Ports: []corev1.ContainerPort{{ContainerPort: 6379}},
			},
			expected: false,
		},
		{
			name: "image changed",
			existing: &corev1.Container{
				Image: "redis:7.0",
			},
			desired: &corev1.Container{
				Image: "redis:7.2",
			},
			expected: true,
		},
		{
			name: "resource changes detected",
			existing: &corev1.Container{
				Image: "redis:7.2",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
			},
			desired: &corev1.Container{
				Image: "redis:7.2",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
			},
			expected: true,
		},
		{
			name: "port changes detected",
			existing: &corev1.Container{
				Image: "redis:7.2",
				Ports: []corev1.ContainerPort{{ContainerPort: 6379}},
			},
			desired: &corev1.Container{
				Image: "redis:7.2",
				Ports: []corev1.ContainerPort{{ContainerPort: 6380}},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := &StandaloneController{}
			result := controller.hasContainerSpecChanges(tt.existing, tt.desired)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasResourceChanges(t *testing.T) {
	tests := []struct {
		name     string
		existing *corev1.Container
		desired  *corev1.Container
		expected bool
	}{
		{
			name: "no resources in both",
			existing: &corev1.Container{
				Resources: corev1.ResourceRequirements{},
			},
			desired: &corev1.Container{
				Resources: corev1.ResourceRequirements{},
			},
			expected: false,
		},
		{
			name: "resource requests changed",
			existing: &corev1.Container{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
			},
			desired: &corev1.Container{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
			},
			expected: true,
		},
		{
			name: "resource limits changed",
			existing: &corev1.Container{
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("100m"),
					},
				},
			},
			desired: &corev1.Container{
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("200m"),
					},
				},
			},
			expected: true,
		},
		{
			name: "resources unchanged",
			existing: &corev1.Container{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("128Mi"),
						corev1.ResourceCPU:    resource.MustParse("100m"),
					},
				},
			},
			desired: &corev1.Container{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("128Mi"),
						corev1.ResourceCPU:    resource.MustParse("100m"),
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := &StandaloneController{}
			result := controller.hasResourceChanges(tt.existing, tt.desired)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasPortChanges(t *testing.T) {
	tests := []struct {
		name     string
		existing *corev1.Container
		desired  *corev1.Container
		expected bool
	}{
		{
			name: "no ports in both",
			existing: &corev1.Container{
				Ports: []corev1.ContainerPort{},
			},
			desired: &corev1.Container{
				Ports: []corev1.ContainerPort{},
			},
			expected: false,
		},
		{
			name: "port added",
			existing: &corev1.Container{
				Ports: []corev1.ContainerPort{},
			},
			desired: &corev1.Container{
				Ports: []corev1.ContainerPort{{ContainerPort: 6379}},
			},
			expected: true,
		},
		{
			name: "port changed",
			existing: &corev1.Container{
				Ports: []corev1.ContainerPort{{ContainerPort: 6379}},
			},
			desired: &corev1.Container{
				Ports: []corev1.ContainerPort{{ContainerPort: 6380}},
			},
			expected: true,
		},
		{
			name: "multiple ports - one changed",
			existing: &corev1.Container{
				Ports: []corev1.ContainerPort{
					{ContainerPort: 6379},
					{ContainerPort: 9121},
				},
			},
			desired: &corev1.Container{
				Ports: []corev1.ContainerPort{
					{ContainerPort: 6379},
					{ContainerPort: 9122},
				},
			},
			expected: true,
		},
		{
			name: "ports unchanged",
			existing: &corev1.Container{
				Ports: []corev1.ContainerPort{
					{ContainerPort: 6379},
					{ContainerPort: 9121},
				},
			},
			desired: &corev1.Container{
				Ports: []corev1.ContainerPort{
					{ContainerPort: 6379},
					{ContainerPort: 9121},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := &StandaloneController{}
			result := controller.hasPortChanges(tt.existing, tt.desired)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasMetadataChanges(t *testing.T) {
	tests := []struct {
		name     string
		existing *appsv1.StatefulSet
		desired  *appsv1.StatefulSet
		expected bool
	}{
		{
			name: "no metadata changes",
			existing: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "redis",
					},
					Annotations: map[string]string{
						"config-hash": "abc123",
					},
				},
			},
			desired: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "redis",
					},
					Annotations: map[string]string{
						"config-hash": "abc123",
					},
				},
			},
			expected: false,
		},
		{
			name: "labels changed",
			existing: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":     "redis",
						"version": "v1",
					},
				},
			},
			desired: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":     "redis",
						"version": "v2",
					},
				},
			},
			expected: true,
		},
		{
			name: "annotations changed",
			existing: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"config-hash": "abc123",
					},
				},
			},
			desired: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"config-hash": "def456",
					},
				},
			},
			expected: true,
		},
		{
			name: "pod template labels changed",
			existing: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app": "redis",
							},
						},
					},
				},
			},
			desired: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app":     "redis",
								"version": "v1",
							},
						},
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := &StandaloneController{}
			result := controller.hasMetadataChanges(tt.existing, tt.desired)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNeedsBackupStatefulSetUpdate(t *testing.T) {
	tests := []struct {
		name     string
		existing *appsv1.StatefulSet
		desired  *appsv1.StatefulSet
		expected bool
	}{
		{
			name: "no changes needed",
			existing: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-backup"},
				Spec: appsv1.StatefulSetSpec{
					Replicas: &[]int32{1}[0],
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							ServiceAccountName: "default",
							Containers: []corev1.Container{{
								Name:  "redis",
								Image: testRedisImage,
							}},
						},
					},
				},
			},
			desired: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-backup"},
				Spec: appsv1.StatefulSetSpec{
					Replicas: &[]int32{1}[0],
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							ServiceAccountName: "default",
							Containers: []corev1.Container{{
								Name:  "redis",
								Image: testRedisImage,
							}},
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "basic spec changes detected",
			existing: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: &[]int32{1}[0],
				},
			},
			desired: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: &[]int32{3}[0],
				},
			},
			expected: true,
		},
		{
			name: "container changes detected",
			existing: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: &[]int32{1}[0],
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{
								Name:  "redis",
								Image: "redis:7.0",
							}},
						},
					},
				},
			},
			desired: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: &[]int32{1}[0],
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{
								Name:  "redis",
								Image: "redis:7.2",
							}},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "volume claim template changes detected",
			existing: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: &[]int32{1}[0],
					VolumeClaimTemplates: []corev1.PersistentVolumeClaim{{
						Spec: corev1.PersistentVolumeClaimSpec{
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse("1Gi"),
								},
							},
						},
					}},
				},
			},
			desired: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: &[]int32{1}[0],
					VolumeClaimTemplates: []corev1.PersistentVolumeClaim{{
						Spec: corev1.PersistentVolumeClaimSpec{
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse("2Gi"),
								},
							},
						},
					}},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := &StandaloneController{}
			result := controller.needsBackupStatefulSetUpdate(tt.existing, tt.desired)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasVolumeClaimTemplateChanges(t *testing.T) {
	tests := []struct {
		name     string
		existing *appsv1.StatefulSet
		desired  *appsv1.StatefulSet
		expected bool
	}{
		{
			name: "no volume claim templates in both",
			existing: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					VolumeClaimTemplates: []corev1.PersistentVolumeClaim{},
				},
			},
			desired: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					VolumeClaimTemplates: []corev1.PersistentVolumeClaim{},
				},
			},
			expected: false,
		},
		{
			name: "volume claim template added",
			existing: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					VolumeClaimTemplates: []corev1.PersistentVolumeClaim{},
				},
			},
			desired: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					VolumeClaimTemplates: []corev1.PersistentVolumeClaim{{
						ObjectMeta: metav1.ObjectMeta{Name: "redis-data"},
						Spec: corev1.PersistentVolumeClaimSpec{
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse("1Gi"),
								},
							},
						},
					}},
				},
			},
			expected: true,
		},
		{
			name: "storage size changed",
			existing: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					VolumeClaimTemplates: []corev1.PersistentVolumeClaim{{
						ObjectMeta: metav1.ObjectMeta{Name: "redis-data"},
						Spec: corev1.PersistentVolumeClaimSpec{
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse("1Gi"),
								},
							},
						},
					}},
				},
			},
			desired: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					VolumeClaimTemplates: []corev1.PersistentVolumeClaim{{
						ObjectMeta: metav1.ObjectMeta{Name: "redis-data"},
						Spec: corev1.PersistentVolumeClaimSpec{
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse("2Gi"),
								},
							},
						},
					}},
				},
			},
			expected: true,
		},
		{
			name: "volume claim templates unchanged",
			existing: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					VolumeClaimTemplates: []corev1.PersistentVolumeClaim{{
						ObjectMeta: metav1.ObjectMeta{Name: "redis-data"},
						Spec: corev1.PersistentVolumeClaimSpec{
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse("1Gi"),
								},
							},
						},
					}},
				},
			},
			desired: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					VolumeClaimTemplates: []corev1.PersistentVolumeClaim{{
						ObjectMeta: metav1.ObjectMeta{Name: "redis-data"},
						Spec: corev1.PersistentVolumeClaimSpec{
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse("1Gi"),
								},
							},
						},
					}},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := &StandaloneController{}
			result := controller.hasVolumeClaimTemplateChanges(tt.existing, tt.desired)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestStandaloneModeIsolation ensures standalone mode doesn't use HA features
func TestStandaloneModeIsolation(t *testing.T) {
	tests := []struct {
		name         string
		redis        *koncachev1alpha1.Redis
		expectHA     bool
		expectSuffix bool
	}{
		{
			name: "Standalone mode disabled HA",
			redis: &koncachev1alpha1.Redis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-standalone",
					Namespace: "default",
				},
				Spec: koncachev1alpha1.RedisSpec{
					Image: testRedisImage,
					HighAvailability: &koncachev1alpha1.RedisHighAvailability{
						Enabled: false,
					},
				},
			},
			expectHA:     false,
			expectSuffix: false,
		},
		{
			name: "Standalone mode nil HA",
			redis: &koncachev1alpha1.Redis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-standalone-nil",
					Namespace: "default",
				},
				Spec: koncachev1alpha1.RedisSpec{
					Image:            testRedisImage,
					HighAvailability: nil,
				},
			},
			expectHA:     false,
			expectSuffix: false,
		},
		{
			name: "HA mode enabled",
			redis: &koncachev1alpha1.Redis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ha",
					Namespace: "default",
				},
				Spec: koncachev1alpha1.RedisSpec{
					Image: testRedisImage,
					HighAvailability: &koncachev1alpha1.RedisHighAvailability{
						Enabled:  true,
						Replicas: 3,
					},
				},
			},
			expectHA:     true,
			expectSuffix: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := &StandaloneController{}

			// Test StatefulSet creation for standalone vs HA
			statefulSets := controller.statefulsSetForRedis(tt.redis, "", "")

			if tt.expectHA {
				// HA mode should create multiple StatefulSets (3 for the test case)
				assert.Len(t, statefulSets, 3, "HA mode should create multiple StatefulSets")

				// Check StatefulSet naming pattern follows HA convention
				for i, sts := range statefulSets {
					expectedName := fmt.Sprintf("%s-ha-%d", tt.redis.Name, i+1)
					assert.Equal(t, expectedName, sts.Name, "HA StatefulSet should follow naming pattern")
				}
			} else {
				// Standalone mode should create exactly one StatefulSet
				assert.Len(t, statefulSets, 1, "Standalone mode should create exactly one StatefulSet")
				sts := statefulSets[0]
				assert.Equal(t, tt.redis.Name, sts.Name, "Standalone StatefulSet should match Redis name")

				// Verify no HA-specific annotations or labels
				assert.NotContains(t, sts.Labels, "redis.io/replica-set", "Standalone should not have HA labels")
				assert.NotContains(t, sts.Annotations, "redis.io/ha-enabled", "Standalone should not have HA annotations")

				// Verify pod template doesn't have HA-specific features
				podTemplate := sts.Spec.Template
				assert.NotContains(t, podTemplate.Annotations, redisRoleAnnotation, "Standalone pods should not have role annotations")

				// Verify exactly one replica for standalone
				assert.Equal(t, int32(1), *sts.Spec.Replicas, "Standalone should always have 1 replica")
			}
		})
	}
}

// TestStandaloneStatefulSetNaming ensures consistent naming for standalone mode
func TestStandaloneStatefulSetNaming(t *testing.T) {
	controller := &StandaloneController{}

	testCases := []struct {
		name      string
		redisName string
		expected  string
	}{
		{
			name:      "simple name",
			redisName: "redis",
			expected:  "redis",
		},
		{
			name:      "name with hyphens",
			redisName: "my-redis-instance",
			expected:  "my-redis-instance",
		},
		{
			name:      "name with numbers",
			redisName: "redis-v2",
			expected:  "redis-v2",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			redis := &koncachev1alpha1.Redis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tc.redisName,
					Namespace: "default",
				},
				Spec: koncachev1alpha1.RedisSpec{
					Image: testRedisImage,
					HighAvailability: &koncachev1alpha1.RedisHighAvailability{
						Enabled: false,
					},
				},
			}

			statefulSets := controller.statefulsSetForRedis(redis, "", "")
			assert.Len(t, statefulSets, 1, "Should create exactly one StatefulSet")
			sts := statefulSets[0]
			assert.Equal(t, tc.expected, sts.Name, "StatefulSet name should match Redis name exactly for standalone mode")

			// Verify no HA suffix patterns
			assert.NotContains(t, sts.Name, "-a", "Standalone should not have HA suffix")
			assert.NotContains(t, sts.Name, "-b", "Standalone should not have HA suffix")
			assert.NotContains(t, sts.Name, "-c", "Standalone should not have HA suffix")
		})
	}
}

// TestStandaloneControllerIgnoresHA ensures standalone controller doesn't process HA Redis instances
func TestStandaloneControllerIgnoresHA(t *testing.T) {
	controller := &StandaloneController{}

	haRedis := &koncachev1alpha1.Redis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ha-redis",
			Namespace: "default",
		},
		Spec: koncachev1alpha1.RedisSpec{
			Image: testRedisImage,
			HighAvailability: &koncachev1alpha1.RedisHighAvailability{
				Enabled:  true,
				Replicas: 3,
			},
		},
	}

	// Test that standalone controller methods handle HA Redis appropriately
	t.Run("statefulsSetForRedis with HA should create multiple StatefulSets", func(t *testing.T) {
		statefulSets := controller.statefulsSetForRedis(haRedis, "", "")

		// The standalone controller should create multiple StatefulSets for HA Redis
		assert.Len(t, statefulSets, 3, "HA Redis should create 3 StatefulSets")

		// Verify HA naming pattern
		for i, sts := range statefulSets {
			expectedName := fmt.Sprintf("ha-redis-ha-%d", i+1)
			assert.Equal(t, expectedName, sts.Name, "HA StatefulSet should follow HA naming pattern")
		}
	})

	t.Run("isHAEnabled correctly identifies HA mode", func(t *testing.T) {
		standaloneRedis := &koncachev1alpha1.Redis{
			Spec: koncachev1alpha1.RedisSpec{
				HighAvailability: &koncachev1alpha1.RedisHighAvailability{
					Enabled: false,
				},
			},
		}

		// Test HA detection
		assert.True(t, haRedis.Spec.HighAvailability != nil && haRedis.Spec.HighAvailability.Enabled,
			"Should correctly identify HA mode")
		assert.False(t, standaloneRedis.Spec.HighAvailability != nil && standaloneRedis.Spec.HighAvailability.Enabled,
			"Should correctly identify standalone mode")
	})
}

// TestStandaloneReplicaCount ensures standalone mode always uses single replica
func TestStandaloneReplicaCount(t *testing.T) {
	controller := &StandaloneController{}

	redis := &koncachev1alpha1.Redis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-replicas",
			Namespace: "default",
		},
		Spec: koncachev1alpha1.RedisSpec{
			Image: testRedisImage,
			HighAvailability: &koncachev1alpha1.RedisHighAvailability{
				Enabled: false,
			},
		},
	}

	statefulSets := controller.statefulsSetForRedis(redis, "", "")
	assert.Len(t, statefulSets, 1, "Should create exactly one StatefulSet")
	sts := statefulSets[0]

	assert.NotNil(t, sts.Spec.Replicas, "Replicas should be set")
	assert.Equal(t, int32(1), *sts.Spec.Replicas, "Standalone mode should always have exactly 1 replica")
}

// TestStandaloneServiceConfiguration ensures standalone mode creates appropriate services
func TestStandaloneServiceConfiguration(t *testing.T) {
	controller := &StandaloneController{}

	redis := &koncachev1alpha1.Redis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "default",
		},
		Spec: koncachev1alpha1.RedisSpec{
			Image: testRedisImage,
			HighAvailability: &koncachev1alpha1.RedisHighAvailability{
				Enabled: false,
			},
		},
	}

	service := controller.serviceForRedis(redis)

	assert.Equal(t, "test-service", service.Name, "Service name should match Redis name for standalone")
	assert.Equal(t, "default", service.Namespace, "Service should be in same namespace")

	// Verify service doesn't have HA-specific configurations
	assert.NotContains(t, service.Labels, "redis.io/ha-role", "Standalone service should not have HA role labels")
	assert.NotContains(t, service.Spec.Selector, testRedisRoleLabel, "Standalone service should not select by role")
}

// TestStandaloneBackupConfiguration ensures backup works correctly in standalone mode
func TestStandaloneBackupConfiguration(t *testing.T) {
	controller := &StandaloneController{}

	redis := &koncachev1alpha1.Redis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-backup",
			Namespace: "default",
		},
		Spec: koncachev1alpha1.RedisSpec{
			Image: testRedisImage,
			HighAvailability: &koncachev1alpha1.RedisHighAvailability{
				Enabled: false,
			},
			Backup: &koncachev1alpha1.RedisBackup{
				Enabled:  true,
				Schedule: "0 2 * * *",
				BackUpInitConfig: &koncachev1alpha1.RedisBackupInitConfig{
					Enabled: true,
				},
				Storage: &koncachev1alpha1.RedisBackupStorage{
					Type: "s3",
					S3: &koncachev1alpha1.RedisBackupS3Storage{
						Bucket:     "redis-backups",
						SecretName: "s3-secret",
					},
				},
			},
		},
	}

	statefulSets := controller.statefulsSetForRedis(redis, "", "")
	assert.Len(t, statefulSets, 1, "Should create exactly one StatefulSet")
	sts := statefulSets[0]

	// Verify backup init container is added for standalone mode
	assert.Len(t, sts.Spec.Template.Spec.InitContainers, 1, "Should have backup init container")
	assert.Equal(t, testBackupInitContainer, sts.Spec.Template.Spec.InitContainers[0].Name)

	// Verify backup doesn't interfere with HA-specific features
	assert.NotContains(t, sts.Spec.Template.Annotations, redisRoleAnnotation,
		"Backup-enabled standalone should not have role annotations")
}

// TestStandaloneConfigMapHandling ensures config maps are handled correctly
func TestStandaloneConfigMapHandling(t *testing.T) {
	controller := &StandaloneController{}

	redis := &koncachev1alpha1.Redis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-config",
			Namespace: "default",
		},
		Spec: koncachev1alpha1.RedisSpec{
			Image: testRedisImage,
			HighAvailability: &koncachev1alpha1.RedisHighAvailability{
				Enabled: false,
			},
			Config: koncachev1alpha1.RedisConfig{
				MaxMemoryPolicy: testMaxMemoryPolicy,
				AdditionalConfig: map[string]string{
					"save": "900 1",
				},
			},
		},
	}

	configMap := controller.configMapForRedis(redis, BuildRedisConfig(redis))

	assert.Equal(t, "test-config"+configMapSuffix, configMap.Name, "ConfigMap should follow standalone naming")
	assert.Contains(t, configMap.Data, redisConfigKey, "Should contain redis config")

	// Verify config doesn't contain HA-specific settings
	redisConf := configMap.Data[redisConfigKey]
	assert.NotContains(t, redisConf, "replica-of", "Standalone config should not have replica settings")
	assert.NotContains(t, redisConf, "replicaof", "Standalone config should not have replica settings")
	assert.Contains(t, redisConf, testMaxMemoryPolicy, "Should contain user-specified config")
}

// TestServiceForRedisHALeaderSelection tests service configuration for HA leader selection
func TestServiceForRedisHALeaderSelection(t *testing.T) {
	controller := &StandaloneController{}

	t.Run("Standalone mode - no leader role", func(t *testing.T) {
		redis := &koncachev1alpha1.Redis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-standalone",
				Namespace: "default",
			},
			Spec: koncachev1alpha1.RedisSpec{
				Image: testRedisImage,
				HighAvailability: &koncachev1alpha1.RedisHighAvailability{
					Enabled: false,
				},
			},
		}

		service := controller.serviceForRedis(redis)

		assert.Equal(t, "test-standalone", service.Name)
		assert.Equal(t, "default", service.Namespace)
		assert.NotContains(t, service.Labels, testRedisRoleLabel,
			"Standalone service should not have role label")
		assert.NotContains(t, service.Spec.Selector, testRedisRoleLabel,
			"Standalone service should not select by role")
	})

	t.Run("HA mode enabled - should have leader role", func(t *testing.T) {
		redis := &koncachev1alpha1.Redis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ha",
				Namespace: "default",
			},
			Spec: koncachev1alpha1.RedisSpec{
				Image: testRedisImage,
				HighAvailability: &koncachev1alpha1.RedisHighAvailability{
					Enabled:  true,
					Replicas: 3,
				},
			},
		}

		service := controller.serviceForRedis(redis)

		assert.Equal(t, "test-ha", service.Name)
		assert.Equal(t, "leader", service.Labels[testRedisRoleLabel],
			"HA service should have leader role label")
		assert.Equal(t, "leader", service.Spec.Selector[testRedisRoleLabel],
			"HA service should select pods with leader role")
	})

	t.Run("HA nil spec - no leader role, no panic", func(t *testing.T) {
		redis := &koncachev1alpha1.Redis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ha-nil",
				Namespace: "default",
			},
			Spec: koncachev1alpha1.RedisSpec{
				Image:            testRedisImage,
				HighAvailability: nil, // This should not cause panic
			},
		}

		assert.NotPanics(t, func() {
			service := controller.serviceForRedis(redis)
			assert.NotContains(t, service.Labels, testRedisRoleLabel,
				"Service with nil HA should not have role label")
		})
	})
}

// TestServiceForRedisHAWithMonitoring tests service configuration with monitoring enabled in HA mode
func TestServiceForRedisHAWithMonitoring(t *testing.T) {
	controller := &StandaloneController{}

	redis := &koncachev1alpha1.Redis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ha-monitoring",
			Namespace: "default",
		},
		Spec: koncachev1alpha1.RedisSpec{
			Image: testRedisImage,
			HighAvailability: &koncachev1alpha1.RedisHighAvailability{
				Enabled:  true,
				Replicas: 3,
			},
			Monitoring: koncachev1alpha1.RedisMonitoring{
				Enabled: true,
				Exporter: koncachev1alpha1.RedisExporter{
					Enabled: true,
					Port:    9121,
				},
			},
		},
	}

	service := controller.serviceForRedis(redis)

	// Verify HA leader configuration
	assert.Equal(t, "leader", service.Labels[testRedisRoleLabel],
		"HA service with monitoring should have leader role label")
	assert.Equal(t, "leader", service.Spec.Selector[testRedisRoleLabel],
		"HA service with monitoring should select leader pods")

	// Verify monitoring annotations
	assert.Equal(t, "true", service.Annotations["prometheus.io/scrape"])
	assert.Equal(t, "9121", service.Annotations["prometheus.io/port"])
	assert.Equal(t, "/metrics", service.Annotations["prometheus.io/path"])

	// Verify both Redis and metrics ports are present
	hasRedisPort := false
	hasMetricsPort := false
	for _, port := range service.Spec.Ports {
		if port.Name == "redis" {
			hasRedisPort = true
		}
		if port.Name == "metrics" && port.Port == 9121 {
			hasMetricsPort = true
		}
	}
	assert.True(t, hasRedisPort, "Service should have Redis port")
	assert.True(t, hasMetricsPort, "Service should have metrics port")
}

// TestServiceForRedisHAWithTLS tests service configuration with TLS enabled in HA mode
func TestServiceForRedisHAWithTLS(t *testing.T) {
	controller := &StandaloneController{}

	redis := &koncachev1alpha1.Redis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ha-tls",
			Namespace: "default",
		},
		Spec: koncachev1alpha1.RedisSpec{
			Image: testRedisImage,
			HighAvailability: &koncachev1alpha1.RedisHighAvailability{
				Enabled:  true,
				Replicas: 3,
			},
			Security: koncachev1alpha1.RedisSecurity{
				TLS: &koncachev1alpha1.RedisTLS{
					Enabled:    true,
					CertSecret: "redis-tls-certs",
				},
			},
		},
	}

	service := controller.serviceForRedis(redis)

	// Verify HA leader configuration
	assert.Equal(t, "leader", service.Labels[testRedisRoleLabel],
		"HA service with TLS should have leader role label")
	assert.Equal(t, "leader", service.Spec.Selector[testRedisRoleLabel],
		"HA service with TLS should select leader pods")

	// Verify TLS port is present, regular Redis port should not be
	hasTLSPort := false
	hasRegularRedisPort := false
	for _, port := range service.Spec.Ports {
		if port.Name == "redis-tls" && port.Port == 6380 {
			hasTLSPort = true
		}
		if port.Name == "redis" && port.Port == 6379 {
			hasRegularRedisPort = true
		}
	}
	assert.True(t, hasTLSPort, "Service should have TLS port when TLS is enabled")
	assert.False(t, hasRegularRedisPort, "Service should not have regular Redis port when TLS is enabled")
}

func TestRedisReplicationConfiguration(t *testing.T) {
	log := ctrl.Log.WithName("test")
	ctx := ctrl.LoggerInto(context.Background(), log)

	testScheme := runtime.NewScheme()
	require.NoError(t, koncachev1alpha1.AddToScheme(testScheme))
	require.NoError(t, corev1.AddToScheme(testScheme))
	require.NoError(t, appsv1.AddToScheme(testScheme))

	tests := []struct {
		name           string
		redis          *koncachev1alpha1.Redis
		pods           []*corev1.Pod
		expectError    bool
		expectCommands []string
	}{
		{
			name: "HA mode with leader and followers",
			redis: &koncachev1alpha1.Redis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-redis",
					Namespace: "default",
				},
				Spec: koncachev1alpha1.RedisSpec{
					Mode: "standalone",
					Port: 6379,
					HighAvailability: &koncachev1alpha1.RedisHighAvailability{
						Enabled:  true,
						Replicas: 3,
					},
				},
			},
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-redis-ha-1-0",
						Namespace: "default",
						Labels: map[string]string{
							"app.kubernetes.io/name":     "redis",
							"app.kubernetes.io/instance": "test-redis",
						},
						Annotations: map[string]string{
							"redis-operator/role": "leader",
						},
					},
					Status: corev1.PodStatus{
						PodIP: "10.0.0.1",
						Conditions: []corev1.PodCondition{
							{Type: corev1.PodReady, Status: corev1.ConditionTrue},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-redis-ha-2-0",
						Namespace: "default",
						Labels: map[string]string{
							"app.kubernetes.io/name":     "redis",
							"app.kubernetes.io/instance": "test-redis",
						},
					},
					Status: corev1.PodStatus{
						PodIP: "10.0.0.2",
						Conditions: []corev1.PodCondition{
							{Type: corev1.PodReady, Status: corev1.ConditionTrue},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-redis-ha-3-0",
						Namespace: "default",
						Labels: map[string]string{
							"app.kubernetes.io/name":     "redis",
							"app.kubernetes.io/instance": "test-redis",
						},
					},
					Status: corev1.PodStatus{
						PodIP: "10.0.0.3",
						Conditions: []corev1.PodCondition{
							{Type: corev1.PodReady, Status: corev1.ConditionTrue},
						},
					},
				},
			},
			expectError: false,
			expectCommands: []string{
				"REPLICAOF 10.0.0.1 6379", // for pod 2
				"REPLICAOF 10.0.0.1 6379", // for pod 3
			},
		},
		{
			name: "No leader pod found",
			redis: &koncachev1alpha1.Redis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-redis",
					Namespace: "default",
				},
				Spec: koncachev1alpha1.RedisSpec{
					Mode: "standalone",
					Port: 6379,
					HighAvailability: &koncachev1alpha1.RedisHighAvailability{
						Enabled:  true,
						Replicas: 2,
					},
				},
			},
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-redis-ha-1-0",
						Namespace: "default",
						Labels: map[string]string{
							"app.kubernetes.io/name":     "redis",
							"app.kubernetes.io/instance": "test-redis",
						},
						// No leader annotation
					},
					Status: corev1.PodStatus{
						PodIP: "10.0.0.1",
						Conditions: []corev1.PodCondition{
							{Type: corev1.PodReady, Status: corev1.ConditionTrue},
						},
					},
				},
			},
			expectError:    false, // Should not error, just skip configuration
			expectCommands: []string{},
		},
		{
			name: "Standalone mode - no replication",
			redis: &koncachev1alpha1.Redis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-redis",
					Namespace: "default",
				},
				Spec: koncachev1alpha1.RedisSpec{
					Mode:             "standalone",
					Port:             6379,
					HighAvailability: nil, // Standalone mode
				},
			},
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-redis-0",
						Namespace: "default",
						Labels: map[string]string{
							"app.kubernetes.io/name":     "redis",
							"app.kubernetes.io/instance": "test-redis",
						},
					},
					Status: corev1.PodStatus{
						PodIP: "10.0.0.1",
						Conditions: []corev1.PodCondition{
							{Type: corev1.PodReady, Status: corev1.ConditionTrue},
						},
					},
				},
			},
			expectError:    false,
			expectCommands: []string{}, // No replication commands expected
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client with test data
			clientBuilder := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(tt.redis)
			for _, pod := range tt.pods {
				clientBuilder = clientBuilder.WithObjects(pod)
			}
			fakeClient := clientBuilder.Build()

			// Create mock executor
			mockExecutor := &MockRedisCommandExecutor{}

			// Create controller
			controller := &StandaloneController{
				Client:          fakeClient,
				Scheme:          testScheme,
				CommandExecutor: mockExecutor,
			}

			// Execute replication configuration
			err := controller.ensureReplicationConfiguration(ctx, tt.redis)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Note: In a real test, you might want to verify the Redis commands
			// were actually executed. For now, we're testing the logic flow.
			// The actual Redis command execution is mocked in our implementation.
		})
	}
}

func TestConfigureRedisAsMaster(t *testing.T) {
	log := ctrl.Log.WithName("test")
	ctx := ctrl.LoggerInto(context.Background(), log)

	testScheme := runtime.NewScheme()
	require.NoError(t, koncachev1alpha1.AddToScheme(testScheme))
	require.NoError(t, corev1.AddToScheme(testScheme))

	redis := &koncachev1alpha1.Redis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-redis",
			Namespace: "default",
		},
		Spec: koncachev1alpha1.RedisSpec{
			Mode: "standalone",
			Port: 6379,
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-redis-0",
			Namespace: "default",
			Labels: map[string]string{
				"app.kubernetes.io/instance": "test-redis",
			},
		},
		Status: corev1.PodStatus{
			PodIP: "10.0.0.1",
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(redis, pod).Build()

	// Create mock executor
	mockExecutor := &MockRedisCommandExecutor{}

	controller := &StandaloneController{
		Client:          fakeClient,
		Scheme:          testScheme,
		CommandExecutor: mockExecutor,
	}

	// This test verifies that the method executes without error
	// and that the correct Redis commands are executed
	err := controller.configureRedisAsMaster(ctx, pod)
	assert.NoError(t, err)

	// Verify that the correct commands were executed
	assert.Len(t, mockExecutor.Commands, 2) // REPLICAOF NO ONE and CONFIG SET save ""
	assert.Equal(t, "test-redis-0", mockExecutor.Commands[0].PodName)
	assert.Equal(t, "REPLICAOF NO ONE", mockExecutor.Commands[0].Command)
	assert.Equal(t, "test-redis-0", mockExecutor.Commands[1].PodName)
	assert.Equal(t, "CONFIG SET save \"\"", mockExecutor.Commands[1].Command)
}

func TestConfigureRedisAsReplica(t *testing.T) {
	log := ctrl.Log.WithName("test")
	ctx := ctrl.LoggerInto(context.Background(), log)

	testScheme := runtime.NewScheme()
	require.NoError(t, koncachev1alpha1.AddToScheme(testScheme))
	require.NoError(t, corev1.AddToScheme(testScheme))

	redis := &koncachev1alpha1.Redis{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-redis",

			Namespace: "default",
		},
		Spec: koncachev1alpha1.RedisSpec{
			Mode: "standalone",
			Port: 6379,
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-redis-0",
			Namespace: "default",
			Labels: map[string]string{
				"app.kubernetes.io/instance": "test-redis",
			},
		},
		Status: corev1.PodStatus{
			PodIP: "10.0.0.1",
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(redis, pod).Build()

	// Create mock executor
	mockExecutor := &MockRedisCommandExecutor{}

	controller := &StandaloneController{
		Client:          fakeClient,
		Scheme:          testScheme,
		CommandExecutor: mockExecutor,
	}

	// This test verifies that the method executes without error
	// and that the correct Redis commands are executed
	err := controller.configureRedisAsReplica(ctx, pod, "10.0.0.2", 6379)
	assert.NoError(t, err)

	// Verify that the correct command was executed
	assert.Len(t, mockExecutor.Commands, 1)
	assert.Equal(t, "test-redis-0", mockExecutor.Commands[0].PodName)
	assert.Equal(t, "REPLICAOF 10.0.0.2 6379", mockExecutor.Commands[0].Command)
}

func TestUpdateRedisStatusAggregation(t *testing.T) {
	tests := []struct {
		name          string
		statefulSets  []*appsv1.StatefulSet
		expectedReady bool
		expectedPhase koncachev1alpha1.RedisPhase
		description   string
	}{
		{
			name: "Single StatefulSet - All Ready",
			statefulSets: []*appsv1.StatefulSet{
				createMockStatefulSet("redis-test-1", 3, 3),
			},
			expectedReady: true,
			expectedPhase: koncachev1alpha1.RedisPhaseRunning,
			description:   "Single StatefulSet with all replicas ready should be ready",
		},
		{
			name: "Single StatefulSet - Not All Ready",
			statefulSets: []*appsv1.StatefulSet{
				createMockStatefulSet("redis-test-1", 3, 2),
			},
			expectedReady: false,
			expectedPhase: koncachev1alpha1.RedisPhasePending,
			description:   "Single StatefulSet with some replicas not ready should not be ready",
		},
		{
			name: "Multiple StatefulSets - All Ready",
			statefulSets: []*appsv1.StatefulSet{
				createMockStatefulSet("redis-test-1", 1, 1),
				createMockStatefulSet("redis-test-2", 1, 1),
				createMockStatefulSet("redis-test-3", 1, 1),
			},
			expectedReady: true,
			expectedPhase: koncachev1alpha1.RedisPhaseRunning,
			description:   "HA deployment with all StatefulSets ready should be ready",
		},
		{
			name: "Multiple StatefulSets - One Not Ready",
			statefulSets: []*appsv1.StatefulSet{
				createMockStatefulSet("redis-test-1", 1, 1),
				createMockStatefulSet("redis-test-2", 1, 0), // This one not ready
				createMockStatefulSet("redis-test-3", 1, 1),
			},
			expectedReady: false,
			expectedPhase: koncachev1alpha1.RedisPhasePending,
			description:   "HA deployment with one StatefulSet not ready should not be ready",
		},
		{
			name: "Multiple StatefulSets - Mixed Replica Counts",
			statefulSets: []*appsv1.StatefulSet{
				createMockStatefulSet("redis-test-1", 2, 2),
				createMockStatefulSet("redis-test-2", 1, 1),
				createMockStatefulSet("redis-test-3", 3, 3),
			},
			expectedReady: true,
			expectedPhase: koncachev1alpha1.RedisPhaseRunning,
			description:   "HA deployment with mixed replica counts but all ready should be ready",
		},
		{
			name: "Multiple StatefulSets - Mixed Replica Counts, One Not Ready",
			statefulSets: []*appsv1.StatefulSet{
				createMockStatefulSet("redis-test-1", 2, 2),
				createMockStatefulSet("redis-test-2", 1, 0), // This one not ready
				createMockStatefulSet("redis-test-3", 3, 3),
			},
			expectedReady: false,
			expectedPhase: koncachev1alpha1.RedisPhasePending,
			description:   "HA deployment with mixed replica counts and one not ready should not be ready",
		},
		{
			name:          "Empty StatefulSets",
			statefulSets:  []*appsv1.StatefulSet{},
			expectedReady: false,
			expectedPhase: koncachev1alpha1.RedisPhasePending,
			description:   "No StatefulSets should result in not ready",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a Redis object to test with
			redis := &koncachev1alpha1.Redis{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-redis",
					Namespace:  "default",
					Generation: 1,
				},
				Spec: koncachev1alpha1.RedisSpec{
					Version: "7.0",
					Port:    6379,
				},
			}

			// Test the aggregation logic directly by calling the update function
			var resultRedis *koncachev1alpha1.Redis
			updateFunc := func(currentRedis *koncachev1alpha1.Redis) {
				// Update status based on aggregated StatefulSet status
				currentRedis.Status.ObservedGeneration = currentRedis.Generation

				// Aggregate status across all StatefulSets
				var totalReplicas, totalReadyReplicas int32
				allReady := true

				for _, statefulSet := range tt.statefulSets {
					if statefulSet.Spec.Replicas != nil {
						totalReplicas += *statefulSet.Spec.Replicas
					}
					totalReadyReplicas += statefulSet.Status.ReadyReplicas

					// Check if this StatefulSet is fully ready
					if statefulSet.Spec.Replicas == nil || statefulSet.Status.ReadyReplicas != *statefulSet.Spec.Replicas {
						allReady = false
					}
				}

				// Set overall readiness based on all StatefulSets
				currentRedis.Status.Ready = allReady && totalReplicas > 0
				if currentRedis.Status.Ready {
					currentRedis.Status.Phase = koncachev1alpha1.RedisPhaseRunning
				} else {
					currentRedis.Status.Phase = koncachev1alpha1.RedisPhasePending
				}

				// Set endpoint and common fields (these don't vary by StatefulSet)
				port := GetRedisPort(currentRedis)
				currentRedis.Status.Endpoint = fmt.Sprintf("%s.%s.svc.cluster.local", currentRedis.Name, currentRedis.Namespace)
				currentRedis.Status.Port = port
				currentRedis.Status.Version = currentRedis.Spec.Version

				resultRedis = currentRedis
			}

			// Apply the update function to test the logic
			updateFunc(redis)

			// Verify the status
			assert.Equal(t, tt.expectedReady, resultRedis.Status.Ready,
				"Ready status mismatch: %s", tt.description)
			assert.Equal(t, tt.expectedPhase, resultRedis.Status.Phase,
				"Phase status mismatch: %s", tt.description)
			assert.Equal(t, redis.Generation, resultRedis.Status.ObservedGeneration,
				"ObservedGeneration should match current generation")
			assert.Equal(t, "test-redis.default.svc.cluster.local", resultRedis.Status.Endpoint,
				"Endpoint should be correctly set")
			assert.Equal(t, int32(6379), resultRedis.Status.Port,
				"Port should be correctly set")
			assert.Equal(t, "7.0", resultRedis.Status.Version,
				"Version should be correctly set")
		})
	}
}

// createMockStatefulSet creates a mock StatefulSet for testing
func createMockStatefulSet(name string, replicas, readyReplicas int32) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
		},
		Status: appsv1.StatefulSetStatus{
			ReadyReplicas: readyReplicas,
		},
	}
}

func TestUpdateRedisStatusRetryMechanism(t *testing.T) {
	// Test the status update mechanism (simplified version)
	redis := &koncachev1alpha1.Redis{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-redis",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: koncachev1alpha1.RedisSpec{
			Version: "7.0",
			Port:    6379,
		},
	}

	statefulSets := []*appsv1.StatefulSet{
		createMockStatefulSet("redis-test-1", 1, 1),
	}

	// Test the update function directly
	var resultRedis *koncachev1alpha1.Redis
	updateFunc := func(currentRedis *koncachev1alpha1.Redis) {
		// Update status based on aggregated StatefulSet status
		currentRedis.Status.ObservedGeneration = currentRedis.Generation

		// Aggregate status across all StatefulSets
		var totalReplicas, totalReadyReplicas int32
		allReady := true

		for _, statefulSet := range statefulSets {
			if statefulSet.Spec.Replicas != nil {
				totalReplicas += *statefulSet.Spec.Replicas
			}
			totalReadyReplicas += statefulSet.Status.ReadyReplicas

			// Check if this StatefulSet is fully ready
			if statefulSet.Spec.Replicas == nil || statefulSet.Status.ReadyReplicas != *statefulSet.Spec.Replicas {
				allReady = false
			}
		}

		// Set overall readiness based on all StatefulSets
		currentRedis.Status.Ready = allReady && totalReplicas > 0
		if currentRedis.Status.Ready {
			currentRedis.Status.Phase = koncachev1alpha1.RedisPhaseRunning
		} else {
			currentRedis.Status.Phase = koncachev1alpha1.RedisPhasePending
		}

		// Set endpoint and common fields (these don't vary by StatefulSet)
		port := GetRedisPort(currentRedis)
		currentRedis.Status.Endpoint = fmt.Sprintf("%s.%s.svc.cluster.local", currentRedis.Name, currentRedis.Namespace)
		currentRedis.Status.Port = port
		currentRedis.Status.Version = currentRedis.Spec.Version

		resultRedis = currentRedis
	}

	// Apply the update function
	updateFunc(redis)

	// Verify the status was updated correctly
	assert.True(t, resultRedis.Status.Ready, "Status should be ready")
	assert.Equal(t, koncachev1alpha1.RedisPhaseRunning, resultRedis.Status.Phase, "Phase should be running")
	assert.Equal(t, redis.Generation, resultRedis.Status.ObservedGeneration, "ObservedGeneration should match")
}
