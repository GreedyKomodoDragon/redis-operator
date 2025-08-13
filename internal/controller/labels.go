package controller

import (
	"strings"

	koncachev1alpha1 "github.com/GreedyKomodoDragon/redis-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// extractVersionFromImage extracts the tag from container image string
// Examples:
//
//	redis:7.2 -> "7.2"
//	redis:latest -> "latest"
//	redis -> "latest" (no tag)
//	myregistry.com/redis:6.2.1 -> "6.2.1"
func extractVersionFromImage(image string) string {
	if image == "" {
		return "latest"
	}

	// Extract tag after the last colon
	parts := strings.Split(image, ":")
	if len(parts) < 2 {
		return "latest"
	}

	// Return the tag as-is
	return parts[len(parts)-1]
}

// LabelsForRedis returns the labels for selecting the resources
func LabelsForRedis(name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":     "redis",
		"app.kubernetes.io/instance": name,
		"app.kubernetes.io/part-of":  "redis-operator",
		"app.kubernetes.io/version":  "latest", // Default version, use LabelsForRedisWithVersion for dynamic version
		// Standard Istio/service mesh labels
		"app":     name,     // Required by Kiali/Istio for linking workloads to applications
		"version": "latest", // Required by Kiali/Istio for versioning telemetry
	}
}

// LabelsForRedisWithVersion returns the labels with dynamic version from Redis spec
func LabelsForRedisWithVersion(redis *koncachev1alpha1.Redis) map[string]string {
	version := extractVersionFromImage(redis.Spec.Image)
	return map[string]string{
		"app.kubernetes.io/name":     "redis",
		"app.kubernetes.io/instance": redis.Name,
		"app.kubernetes.io/part-of":  "redis-operator",
		"app.kubernetes.io/version":  version, // Dynamic Redis version from image
		// Standard Istio/service mesh labels
		"app":     redis.Name, // Required by Kiali/Istio for linking workloads to applications
		"version": version,    // Required by Kiali/Istio for versioning telemetry
	}
}

// LabelsForRedisCluster returns the labels for selecting cluster resources
func LabelsForRedisCluster(name string) map[string]string {
	labels := LabelsForRedis(name)
	labels["app.kubernetes.io/component"] = "cluster"
	// Override app label for cluster to include component
	labels["app"] = name + "-cluster"
	return labels
}

// LabelsForRedisClusterWithVersion returns the labels for cluster resources with dynamic version
func LabelsForRedisClusterWithVersion(redis *koncachev1alpha1.Redis) map[string]string {
	labels := LabelsForRedisWithVersion(redis)
	labels["app.kubernetes.io/component"] = "cluster"
	// Override app label for cluster to include component
	labels["app"] = redis.Name + "-cluster"
	return labels
}

// BuildBackupObjectMeta creates the ObjectMeta for a Redis backup pod
func BuildBackupObjectMeta(redis *koncachev1alpha1.Redis) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Labels:      LabelsForRedisBackup(redis),
		Annotations: BuildBackupAnnotations(redis),
	}
}

// LabelsForRedisBackup returns the labels for selecting Redis backup resources
func LabelsForRedisBackup(redis *koncachev1alpha1.Redis) map[string]string {
	labels := LabelsForRedisWithVersion(redis)
	labels["app.kubernetes.io/component"] = "backup"
	// Override app label for backup to include component
	labels["app"] = redis.Name + "-backup"
	return labels
}

// BuildBackupAnnotations creates annotations for a Redis backup pod
func BuildBackupAnnotations(redis *koncachev1alpha1.Redis) map[string]string {
	annotations := make(map[string]string)
	if redis.Spec.Backup.Annotations != nil {
		for key, value := range redis.Spec.Backup.Annotations {
			annotations[key] = value
		}
	}
	return annotations
}
