package controller

import koncachev1alpha1 "github.com/GreedyKomodoDragon/redis-operator/api/v1alpha1"

// IsMonitoringEnabled returns true if monitoring and exporter are enabled
func IsMonitoringEnabled(redis *koncachev1alpha1.Redis) bool {
	return redis.Spec.Monitoring.Enabled && redis.Spec.Monitoring.Exporter.Enabled
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
