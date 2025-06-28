package controller

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	koncachev1alpha1 "github.com/GreedyKomodoDragon/redis-operator/api/v1alpha1"
)

// Constants for volume and mount names
const (
	RedisDataVolumeName   = "redis-data"
	RedisConfigVolumeName = "redis-config"
	TLSCertsVolumeName    = "tls-certs"
	TLSCAVolumeName       = "tls-ca"
)

// Constants for TLS certificate keys
const (
	TLSCertKey = "tls.crt"
	TLSKeyKey  = "tls.key"
	TLSCAKey   = "ca.crt"
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

	// Security configuration
	config += BuildSecurityConfig(redis)

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
	// Build container ports based on TLS configuration
	var containerPorts []corev1.ContainerPort
	if IsTLSEnabled(redis) {
		// When TLS is enabled, only expose the TLS port
		containerPorts = []corev1.ContainerPort{
			{
				ContainerPort: 6380,
				Name:          "redis-tls",
				Protocol:      corev1.ProtocolTCP,
			},
		}
	} else {
		// When TLS is disabled, expose the regular Redis port
		containerPorts = []corev1.ContainerPort{
			{
				ContainerPort: port,
				Name:          "redis",
				Protocol:      corev1.ProtocolTCP,
			},
		}
	}

	container := corev1.Container{
		Name:            "redis",
		Image:           redis.Spec.Image,
		ImagePullPolicy: redis.Spec.ImagePullPolicy,
		Ports:           containerPorts,
		Resources:       redis.Spec.Resources,
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      RedisDataVolumeName,
				MountPath: "/data",
			},
			{
				Name:      RedisConfigVolumeName,
				MountPath: "/usr/local/etc/redis/redis.conf",
				SubPath:   "redis.conf",
				ReadOnly:  true,
			},
		},
		Command: []string{"sh", "-c"},
		Args: []string{
			`# Replace environment variables in config and start Redis
			sed 's/\$REDIS_PASSWORD/'"$REDIS_PASSWORD"'/g' /usr/local/etc/redis/redis.conf > /tmp/redis.conf && redis-server /tmp/redis.conf`,
		},
	}

	// Add security environment variables
	securityEnv := BuildSecurityEnvironment(redis)
	if len(securityEnv) > 0 {
		container.Env = append(container.Env, securityEnv...)
	}

	// Add TLS volume mounts
	tlsVolumeMounts := BuildTLSVolumeMounts(redis)
	if len(tlsVolumeMounts) > 0 {
		container.VolumeMounts = append(container.VolumeMounts, tlsVolumeMounts...)
	}

	// Add security-aware probes
	livenessProbe, readinessProbe := BuildSecurityProbes(redis, port)
	container.LivenessProbe = livenessProbe
	container.ReadinessProbe = readinessProbe

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
			Name: RedisDataVolumeName,
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
		Name: RedisConfigVolumeName,
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

	// Build Redis connection URL based on security settings
	var redisAddr string
	if IsTLSEnabled(redis) {
		// For TLS connections, use rediss:// protocol with TLS port
		redisAddr = "rediss://localhost:6380"
	} else {
		redisAddr = fmt.Sprintf("redis://localhost:%d", redisPort)
	}

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
				Value: redisAddr,
			},
		},
	}

	// Add Redis password environment variable if auth is enabled
	if IsAuthEnabled(redis) {
		container.Env = append(container.Env, corev1.EnvVar{
			Name: "REDIS_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: GetPasswordSecretName(redis),
					},
					Key: GetPasswordSecretKey(redis),
				},
			},
		})
	}

	// Add TLS configuration if TLS is enabled
	if IsTLSEnabled(redis) {
		// Mount TLS certificates
		tlsVolumeMounts := BuildTLSVolumeMounts(redis)
		if len(tlsVolumeMounts) > 0 {
			container.VolumeMounts = append(container.VolumeMounts, tlsVolumeMounts...)
		}

		// Add TLS environment variables for redis_exporter
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  "REDIS_EXPORTER_TLS_CLIENT_CERT_FILE",
			Value: "/etc/redis/tls/tls.crt",
		})
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  "REDIS_EXPORTER_TLS_CLIENT_KEY_FILE",
			Value: "/etc/redis/tls/tls.key",
		})

		// Add CA certificate if configured
		if redis.Spec.Security.TLS.CASecret != "" {
			container.Env = append(container.Env, corev1.EnvVar{
				Name:  "REDIS_EXPORTER_TLS_CA_CERT_FILE",
				Value: "/etc/redis/tls/ca.crt",
			})
		}

		// For self-signed certificates, allow skipping TLS verification
		// This is commonly needed in development/testing environments
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  "REDIS_EXPORTER_SKIP_TLS_VERIFICATION",
			Value: "true",
		})
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

// Security utility functions

// IsSecurityEnabled returns true if any security feature is enabled
func IsSecurityEnabled(redis *koncachev1alpha1.Redis) bool {
	return (redis.Spec.Security.RequireAuth != nil && *redis.Spec.Security.RequireAuth) ||
		(redis.Spec.Security.TLS != nil && redis.Spec.Security.TLS.Enabled) ||
		len(redis.Spec.Security.RenameCommands) > 0
}

// IsAuthEnabled returns true if authentication is enabled
func IsAuthEnabled(redis *koncachev1alpha1.Redis) bool {
	return redis.Spec.Security.RequireAuth != nil && *redis.Spec.Security.RequireAuth
}

// IsTLSEnabled returns true if TLS is enabled
func IsTLSEnabled(redis *koncachev1alpha1.Redis) bool {
	return redis.Spec.Security.TLS != nil && redis.Spec.Security.TLS.Enabled
}

// GetPasswordSecretName returns the secret name containing the Redis password
func GetPasswordSecretName(redis *koncachev1alpha1.Redis) string {
	if redis.Spec.Security.PasswordSecret != nil {
		return redis.Spec.Security.PasswordSecret.Name
	}
	return redis.Name + "-auth"
}

// GetPasswordSecretKey returns the secret key containing the Redis password
func GetPasswordSecretKey(redis *koncachev1alpha1.Redis) string {
	if redis.Spec.Security.PasswordSecret != nil {
		return redis.Spec.Security.PasswordSecret.Key
	}
	return "password"
}

// BuildSecurityConfig builds the security configuration for Redis
func BuildSecurityConfig(redis *koncachev1alpha1.Redis) string {
	config := ""

	// Authentication configuration
	config += buildAuthConfig(redis)

	// TLS configuration
	config += buildTLSConfig(redis)

	// Command renaming for security
	config += buildCommandRenameConfig(redis)

	return config
}

// buildAuthConfig builds authentication configuration
func buildAuthConfig(redis *koncachev1alpha1.Redis) string {
	if IsAuthEnabled(redis) {
		return "protected-mode yes\nrequirepass $REDIS_PASSWORD\n"
	}
	return "protected-mode no\n"
}

// buildTLSConfig builds TLS configuration
func buildTLSConfig(redis *koncachev1alpha1.Redis) string {
	if !IsTLSEnabled(redis) {
		return ""
	}

	config := "tls-port 6380\nport 0\n" // Disable non-TLS port

	// Certificate files
	if redis.Spec.Security.TLS.CertFile != "" {
		config += fmt.Sprintf("tls-cert-file %s\n", redis.Spec.Security.TLS.CertFile)
	} else {
		config += "tls-cert-file /etc/redis/tls/tls.crt\n"
	}

	if redis.Spec.Security.TLS.KeyFile != "" {
		config += fmt.Sprintf("tls-key-file %s\n", redis.Spec.Security.TLS.KeyFile)
	} else {
		config += "tls-key-file /etc/redis/tls/tls.key\n"
	}

	// CA certificate
	if redis.Spec.Security.TLS.CAFile != "" {
		config += fmt.Sprintf("tls-ca-cert-file %s\n", redis.Spec.Security.TLS.CAFile)
	} else if redis.Spec.Security.TLS.CASecret != "" {
		// If CA secret is the same as cert secret, CA is in the same volume
		if redis.Spec.Security.TLS.CASecret == redis.Spec.Security.TLS.CertSecret {
			config += "tls-ca-cert-file /etc/redis/tls/ca.crt\n"
		} else {
			// CA secret is separate, mounted in different volume
			config += "tls-ca-cert-file /etc/redis/tls-ca/ca.crt\n"
		}
	}

	// TLS security settings
	config += "tls-auth-clients yes\ntls-protocols \"TLSv1.2 TLSv1.3\"\n"

	return config
}

// buildCommandRenameConfig builds command renaming configuration
func buildCommandRenameConfig(redis *koncachev1alpha1.Redis) string {
	config := ""
	for oldCmd, newCmd := range redis.Spec.Security.RenameCommands {
		if newCmd == "" {
			config += fmt.Sprintf("rename-command %s \"\"\n", oldCmd)
		} else {
			config += fmt.Sprintf("rename-command %s %s\n", oldCmd, newCmd)
		}
	}
	return config
}

// BuildSecurityEnvironment builds environment variables for security
func BuildSecurityEnvironment(redis *koncachev1alpha1.Redis) []corev1.EnvVar {
	var envVars []corev1.EnvVar

	// Add Redis password from secret
	if IsAuthEnabled(redis) {
		envVars = append(envVars, corev1.EnvVar{
			Name: "REDIS_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: GetPasswordSecretName(redis),
					},
					Key: GetPasswordSecretKey(redis),
				},
			},
		})
	}

	return envVars
}

// BuildTLSVolumeMounts builds volume mounts for TLS certificates
func BuildTLSVolumeMounts(redis *koncachev1alpha1.Redis) []corev1.VolumeMount {
	var volumeMounts []corev1.VolumeMount

	if IsTLSEnabled(redis) {
		// TLS certificate volume mount
		if redis.Spec.Security.TLS.CertSecret != "" {
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      TLSCertsVolumeName,
				MountPath: "/etc/redis/tls",
				ReadOnly:  true,
			})
		}

		// CA certificate volume mount (if different from cert secret)
		if redis.Spec.Security.TLS.CASecret != "" &&
			redis.Spec.Security.TLS.CASecret != redis.Spec.Security.TLS.CertSecret {
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      TLSCAVolumeName,
				MountPath: "/etc/redis/tls-ca",
				ReadOnly:  true,
			})
		}
	}

	return volumeMounts
}

// BuildTLSVolumes builds volumes for TLS certificates
func BuildTLSVolumes(redis *koncachev1alpha1.Redis) []corev1.Volume {
	var volumes []corev1.Volume

	if IsTLSEnabled(redis) {
		// TLS certificate volume
		if redis.Spec.Security.TLS.CertSecret != "" {
			items := []corev1.KeyToPath{
				{
					Key:  TLSCertKey,
					Path: TLSCertKey,
				},
				{
					Key:  TLSKeyKey,
					Path: TLSKeyKey,
				},
			}

			// If CA secret is the same as cert secret, include ca.crt in the same volume
			if redis.Spec.Security.TLS.CASecret == redis.Spec.Security.TLS.CertSecret {
				items = append(items, corev1.KeyToPath{
					Key:  TLSCAKey,
					Path: TLSCAKey,
				})
			}

			volumes = append(volumes, corev1.Volume{
				Name: TLSCertsVolumeName,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: redis.Spec.Security.TLS.CertSecret,
						Items:      items,
					},
				},
			})
		}

		// CA certificate volume (if different from cert secret)
		if redis.Spec.Security.TLS.CASecret != "" &&
			redis.Spec.Security.TLS.CASecret != redis.Spec.Security.TLS.CertSecret {
			volumes = append(volumes, corev1.Volume{
				Name: TLSCAVolumeName,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: redis.Spec.Security.TLS.CASecret,
						Items: []corev1.KeyToPath{
							{
								Key:  TLSCAKey,
								Path: TLSCAKey,
							},
						},
					},
				},
			})
		}
	}

	return volumes
}

// BuildSecurityProbes builds health probes that work with security settings
func BuildSecurityProbes(redis *koncachev1alpha1.Redis, port int32) (*corev1.Probe, *corev1.Probe) {
	var livenessProbe, readinessProbe *corev1.Probe

	if IsTLSEnabled(redis) {
		// For TLS, use TCP socket probe since redis-cli might not support TLS
		livenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(6380), // TLS port
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       10,
			TimeoutSeconds:      5,
			FailureThreshold:    3,
		}

		readinessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(6380), // TLS port
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       10,
			TimeoutSeconds:      1,
			FailureThreshold:    3,
		}
	} else if IsAuthEnabled(redis) {
		// For auth-enabled Redis, use authenticated ping
		livenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: []string{
						"sh", "-c",
						"redis-cli -a $REDIS_PASSWORD ping | grep PONG",
					},
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       10,
			TimeoutSeconds:      5,
			FailureThreshold:    3,
		}

		readinessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: []string{
						"sh", "-c",
						"redis-cli -a $REDIS_PASSWORD ping | grep PONG",
					},
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       10,
			TimeoutSeconds:      1,
			FailureThreshold:    3,
		}
	} else {
		// Standard probes for non-secured Redis
		livenessProbe = &corev1.Probe{
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

		readinessProbe = &corev1.Probe{
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
	}

	return livenessProbe, readinessProbe
}

// buildVolumes builds all volumes needed for the Redis pod including config and TLS
func buildVolumes(redis *koncachev1alpha1.Redis) []corev1.Volume {
	volumes := []corev1.Volume{
		{
			Name: RedisConfigVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: redis.Name + "-config",
					},
				},
			},
		},
	}

	// Add TLS volumes if TLS is enabled
	tlsVolumes := BuildTLSVolumes(redis)
	if len(tlsVolumes) > 0 {
		volumes = append(volumes, tlsVolumes...)
	}

	return volumes
}
