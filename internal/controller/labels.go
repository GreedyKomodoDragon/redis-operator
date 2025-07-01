package controller

import (
	koncachev1alpha1 "github.com/GreedyKomodoDragon/redis-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// BuildBackupObjectMeta creates the ObjectMeta for a Redis backup pod
func BuildBackupObjectMeta(redis *koncachev1alpha1.Redis) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Labels:      LabelsForRedisBackup(redis),
		Annotations: BuildBackupAnnotations(redis),
	}
}

// LabelsForRedisBackup returns the labels for selecting Redis backup resources
func LabelsForRedisBackup(redis *koncachev1alpha1.Redis) map[string]string {
	labels := LabelsForRedis(redis.Name)
	labels["app.kubernetes.io/component"] = "backup"
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
