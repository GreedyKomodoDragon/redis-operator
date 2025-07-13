package controller

import (
	"fmt"
	"sort"

	koncachev1alpha1 "github.com/GreedyKomodoDragon/redis-operator/api/v1alpha1"
)

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

	// Databases configuration - use default value of 16 if not specified or 0
	databases := redis.Spec.Config.Databases
	if databases <= 0 {
		databases = 16
	}
	config += fmt.Sprintf("databases %d\n", databases)

	// Logging
	if redis.Spec.Config.LogLevel != "" {
		config += fmt.Sprintf("loglevel %s\n", redis.Spec.Config.LogLevel)
	}

	// Security configuration
	config += BuildSecurityConfig(redis)

	// Additional custom configuration
	if len(redis.Spec.Config.AdditionalConfig) > 0 {
		// Sort keys to ensure deterministic config generation
		keys := make([]string, 0, len(redis.Spec.Config.AdditionalConfig))
		for key := range redis.Spec.Config.AdditionalConfig {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		for _, key := range keys {
			config += fmt.Sprintf("%s %s\n", key, redis.Spec.Config.AdditionalConfig[key])
		}
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
