package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	testBackupInitContainer = "backup-init"
	testBackupInitImage     = "backup-init:latest"
	testRedisImage72        = "redis:7.2"
	testRedisImage70        = "redis:7.0"
	testConfigHashAnno      = "config-hash"
	testRedisDataVolume     = "redis-data"
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
