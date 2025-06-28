package controller_test

// Test constants to avoid duplication
const (
	testRedisExporterImage = "oliver006/redis_exporter:v1.45.0"
	testRedisName          = "my-redis"
	testRedisClusterName   = "redis-cluster-prod"
	testRedisInstance      = "test-redis"
	testTLSCertsSecret     = "tls-certs"

	// Label constants
	testAppNameLabel     = "app.kubernetes.io/name"
	testAppInstanceLabel = "app.kubernetes.io/instance"
	testAppPartOfLabel   = "app.kubernetes.io/part-of"
	testRedisValue       = "redis"
	testOperatorValue    = "redis-operator"

	// Volume constants
	testDataVolume   = "redis-data"
	testConfigVolume = "redis-config"

	// Test case names
	testAuthEnabledCase = "auth enabled"
	testTLSEnabledCase  = "TLS enabled"
)
