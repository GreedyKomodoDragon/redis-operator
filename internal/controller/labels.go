package controller

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
