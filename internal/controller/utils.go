package controller

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	koncachev1alpha1 "github.com/GreedyKomodoDragon/redis-operator/api/v1alpha1"
)

// LabelsForRedis returns the labels for selecting the resources
func LabelsForRedis(name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":     "redis",
		"app.kubernetes.io/instance": name,
		"app.kubernetes.io/part-of":  "redis-operator",
	}
}

// LabelsForRedisCluster returns the labels for selecting cluster resources
func LabelsForRedisCluster(name string) map[string]string {
	labels := LabelsForRedis(name)
	labels["app.kubernetes.io/component"] = "cluster"
	return labels
}

// LabelsForRedisSentinel returns the labels for selecting sentinel resources
func LabelsForRedisSentinel(name string) map[string]string {
	labels := LabelsForRedis(name)
	labels["app.kubernetes.io/component"] = "sentinel"
	return labels
}

// BuildRedisConfig builds the Redis configuration string
func BuildRedisConfig(redis *koncachev1alpha1.Redis) string {
	config := ""

	// Basic configuration
	if redis.Spec.Config.MaxMemory != "" {
		config += fmt.Sprintf("maxmemory %s\n", redis.Spec.Config.MaxMemory)
	}
	if redis.Spec.Config.MaxMemoryPolicy != "" {
		config += fmt.Sprintf("maxmemory-policy %s\n", redis.Spec.Config.MaxMemoryPolicy)
	}

	// Persistence configuration
	if len(redis.Spec.Config.Save) > 0 {
		for _, save := range redis.Spec.Config.Save {
			config += fmt.Sprintf("save %s\n", save)
		}
	}

	if redis.Spec.Config.AppendOnly != nil && *redis.Spec.Config.AppendOnly {
		config += "appendonly yes\n"
		if redis.Spec.Config.AppendFsync != "" {
			config += fmt.Sprintf("appendfsync %s\n", redis.Spec.Config.AppendFsync)
		}
	}

	// Network configuration
	config += fmt.Sprintf("timeout %d\n", redis.Spec.Config.Timeout)
	config += fmt.Sprintf("tcp-keepalive %d\n", redis.Spec.Config.TCPKeepAlive)
	config += fmt.Sprintf("databases %d\n", redis.Spec.Config.Databases)

	// Logging
	if redis.Spec.Config.LogLevel != "" {
		config += fmt.Sprintf("loglevel %s\n", redis.Spec.Config.LogLevel)
	}

	// Security
	if redis.Spec.Security.RequireAuth != nil && *redis.Spec.Security.RequireAuth {
		config += "protected-mode yes\n"
		// Note: Password will be set via environment variable or secret
	}

	// Additional custom configuration
	for key, value := range redis.Spec.Config.AdditionalConfig {
		config += fmt.Sprintf("%s %s\n", key, value)
	}

	// Default settings if config is empty
	if config == "" {
		config = `# Redis configuration
bind 0.0.0.0
port 6379
dir /data
appendonly yes
appendfsync everysec
maxmemory-policy allkeys-lru
`
	}

	return config
}

// BuildRedisContainer builds the Redis container specification
func BuildRedisContainer(redis *koncachev1alpha1.Redis, port int32) corev1.Container {
	container := corev1.Container{
		Name:            "redis",
		Image:           redis.Spec.Image,
		ImagePullPolicy: redis.Spec.ImagePullPolicy,
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: port,
				Name:          "redis",
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Resources: redis.Spec.Resources,
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "redis-data",
				MountPath: "/data",
			},
			{
				Name:      "redis-config",
				MountPath: "/usr/local/etc/redis/redis.conf",
				SubPath:   "redis.conf",
				ReadOnly:  true,
			},
		},
		Command: []string{"redis-server"},
		Args:    []string{"/usr/local/etc/redis/redis.conf"},
	}

	// Add liveness and readiness probes
	container.LivenessProbe = &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			TCPSocket: &corev1.TCPSocketAction{
				Port: intstr.FromInt(int(port)),
			},
		},
		InitialDelaySeconds: 30,
		PeriodSeconds:       10,
		TimeoutSeconds:      5,
		FailureThreshold:    3,
	}

	container.ReadinessProbe = &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: []string{"redis-cli", "ping"},
			},
		},
		InitialDelaySeconds: 5,
		PeriodSeconds:       10,
		TimeoutSeconds:      1,
		FailureThreshold:    3,
	}

	return container
}

// BuildVolumeClaimTemplate builds the volume claim template for StatefulSets
func BuildVolumeClaimTemplate(redis *koncachev1alpha1.Redis) corev1.PersistentVolumeClaim {
	storageSize := redis.Spec.Storage.Size
	if storageSize.IsZero() {
		storageSize = resource.MustParse("1Gi")
	}

	accessModes := redis.Spec.Storage.AccessModes
	if len(accessModes) == 0 {
		accessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
	}

	return corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: "redis-data",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      accessModes,
			StorageClassName: redis.Spec.Storage.StorageClassName,
			VolumeMode:       redis.Spec.Storage.VolumeMode,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: storageSize,
				},
			},
		},
	}
}

// BuildConfigMapVolume creates a ConfigMap volume for Redis configuration
func BuildConfigMapVolume(redisName string) corev1.Volume {
	return corev1.Volume{
		Name: "redis-config",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: redisName + "-config",
				},
			},
		},
	}
}

// EqualResourceLists compares two ResourceList objects
func EqualResourceLists(a, b corev1.ResourceList) bool {
	if len(a) != len(b) {
		return false
	}

	for key, aVal := range a {
		bVal, exists := b[key]
		if !exists || !aVal.Equal(bVal) {
			return false
		}
	}

	return true
}

// EqualStringMaps compares two string maps for equality
func EqualStringMaps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// EqualTolerations compares two toleration slices for equality
func EqualTolerations(a, b []corev1.Toleration) bool {
	if len(a) != len(b) {
		return false
	}

	// Simple comparison - check if each toleration in a exists in b
	for _, aTol := range a {
		found := false
		for _, bTol := range b {
			if TolerationsEqual(aTol, bTol) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// TolerationsEqual compares two individual tolerations
func TolerationsEqual(a, b corev1.Toleration) bool {
	if a.Key != b.Key || a.Operator != b.Operator || a.Effect != b.Effect || a.Value != b.Value {
		return false
	}

	// Compare TolerationSeconds
	if a.TolerationSeconds == nil && b.TolerationSeconds == nil {
		return true
	}
	if a.TolerationSeconds != nil && b.TolerationSeconds != nil {
		return *a.TolerationSeconds == *b.TolerationSeconds
	}

	return false
}

// GetRedisPort returns the Redis port, defaulting to 6379 if not specified
func GetRedisPort(redis *koncachev1alpha1.Redis) int32 {
	port := redis.Spec.Port
	if port == 0 {
		return 6379
	}
	return port
}

// GetRedisExporterPort returns the Redis exporter port, defaulting to 9121 if not specified
func GetRedisExporterPort(redis *koncachev1alpha1.Redis) int32 {
	port := redis.Spec.Monitoring.Exporter.Port
	if port == 0 {
		return 9121
	}
	return port
}

// GetRedisExporterImage returns the Redis exporter image, defaulting to oliver006/redis_exporter:latest if not specified
func GetRedisExporterImage(redis *koncachev1alpha1.Redis) string {
	image := redis.Spec.Monitoring.Exporter.Image
	if image == "" {
		return "oliver006/redis_exporter:latest"
	}
	return image
}

// BuildRedisExporterContainer builds the Redis exporter sidecar container specification
func BuildRedisExporterContainer(redis *koncachev1alpha1.Redis, redisPort int32) corev1.Container {
	exporterPort := GetRedisExporterPort(redis)
	exporterImage := GetRedisExporterImage(redis)

	container := corev1.Container{
		Name:            "redis-exporter",
		Image:           exporterImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: exporterPort,
				Name:          "metrics",
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Resources: redis.Spec.Monitoring.Exporter.Resources,
		Env: []corev1.EnvVar{
			{
				Name:  "REDIS_ADDR",
				Value: fmt.Sprintf("redis://localhost:%d", redisPort),
			},
		},
	}

	// Add liveness and readiness probes for the exporter
	container.LivenessProbe = &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/health",
				Port: intstr.FromInt(int(exporterPort)),
			},
		},
		InitialDelaySeconds: 15,
		PeriodSeconds:       20,
		TimeoutSeconds:      5,
		FailureThreshold:    3,
	}

	container.ReadinessProbe = &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/health",
				Port: intstr.FromInt(int(exporterPort)),
			},
		},
		InitialDelaySeconds: 5,
		PeriodSeconds:       10,
		TimeoutSeconds:      3,
		FailureThreshold:    3,
	}

	return container
}

// IsMonitoringEnabled returns true if monitoring and exporter are enabled
func IsMonitoringEnabled(redis *koncachev1alpha1.Redis) bool {
	return redis.Spec.Monitoring.Enabled && redis.Spec.Monitoring.Exporter.Enabled
}
