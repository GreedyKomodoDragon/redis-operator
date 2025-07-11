package controller

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	koncachev1alpha1 "github.com/GreedyKomodoDragon/redis-operator/api/v1alpha1"
)

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
		SecurityContext: &corev1.SecurityContext{
			RunAsUser:  &[]int64{999}[0],
			RunAsGroup: &[]int64{999}[0],
		},
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

func BuildBackupPodTemplateForRedis(redis *koncachev1alpha1.Redis) corev1.PodTemplateSpec {
	// Build the Redis backup container
	backupContainer := BuildBackupContainer(redis)

	// Create the pod template with the backup container
	return corev1.PodTemplateSpec{
		ObjectMeta: BuildBackupObjectMeta(redis),
		Spec: corev1.PodSpec{
			Containers:      []corev1.Container{backupContainer},
			Volumes:         BuildBackupVolumes(redis),
			NodeSelector:    redis.Spec.Backup.PodTemplate.NodeSelector,
			Tolerations:     redis.Spec.Backup.PodTemplate.Tolerations,
			SecurityContext: redis.Spec.Backup.PodTemplate.SecurityContext,
			Affinity:        redis.Spec.Backup.PodTemplate.Affinity,
		},
	}
}

// BuildBackupContainer builds the container for Redis backup jobs
func BuildBackupContainer(redis *koncachev1alpha1.Redis) corev1.Container {
	// Determine the correct Redis port based on TLS configuration
	var redisPort int32
	if IsTLSEnabled(redis) {
		redisPort = 6380 // TLS port
	} else {
		redisPort = GetRedisPort(redis) // Regular port
	}

	// Build environment variables for Redis connection
	// Use FQDN for reliable service resolution across different cluster configurations
	redisHostFQDN := fmt.Sprintf("%s.%s.svc.cluster.local", redis.Name, redis.Namespace)

	envVars := []corev1.EnvVar{
		{
			Name:  "REDIS_HOST",
			Value: redisHostFQDN,
		},
		{
			Name:  "REDIS_PORT",
			Value: fmt.Sprintf("%d", redisPort),
		},
		{
			Name:  "REDIS_DB",
			Value: "0", // Default database
		},
	}

	// Add password environment variable if authentication is enabled
	if redis.Spec.Security.RequireAuth != nil && *redis.Spec.Security.RequireAuth && redis.Spec.Security.PasswordSecret != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name: "REDIS_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: redis.Spec.Security.PasswordSecret,
			},
		})
	}

	// Add TLS environment variables if TLS is enabled
	if IsTLSEnabled(redis) {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "REDIS_TLS_ENABLED",
			Value: "true",
		})
		// Note: TLS certificate paths would be mounted via volumes if needed
	}

	// Add retry configuration environment variables
	envVars = append(envVars, []corev1.EnvVar{
		{
			Name:  "MAX_RETRIES",
			Value: "5",
		},
		{
			Name:  "RETRY_DELAY_SECONDS",
			Value: "5",
		},
		{
			Name:  "MAX_RETRY_DELAY_SECONDS",
			Value: "300",
		},
	}...)

	// Add backup retention configuration
	if redis.Spec.Backup.Retention > 0 {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "BACKUP_RETENTION",
			Value: fmt.Sprintf("%d", redis.Spec.Backup.Retention),
		})
	}

	// Add fixed backup ID based on Redis instance name and namespace
	// This ensures retention works across Redis restarts and replication ID changes
	backupID := fmt.Sprintf("%s-%s", redis.Namespace, redis.Name)
	envVars = append(envVars, corev1.EnvVar{
		Name:  "BACKUP_ID",
		Value: backupID,
	})

	// Add S3 configuration if backup S3 settings are configured
	if redis.Spec.Backup.Storage.Type == "s3" && redis.Spec.Backup.Storage.S3 != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "S3_BUCKET",
			Value: redis.Spec.Backup.Storage.S3.Bucket,
		})

		if redis.Spec.Backup.Storage.S3.Region != "" {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "AWS_REGION",
				Value: redis.Spec.Backup.Storage.S3.Region,
			})
		}

		if redis.Spec.Backup.Storage.S3.Prefix != "" {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "S3_PREFIX",
				Value: redis.Spec.Backup.Storage.S3.Prefix,
			})
		}

		// Add AWS credentials from secret if configured
		if redis.Spec.Backup.Storage.S3.SecretName != "" {
			envVars = append(envVars, []corev1.EnvVar{
				{
					Name: "AWS_ACCESS_KEY_ID",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: redis.Spec.Backup.Storage.S3.SecretName,
							},
							Key: "access-key-id",
						},
					},
				},
				{
					Name: "AWS_SECRET_ACCESS_KEY",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: redis.Spec.Backup.Storage.S3.SecretName,
							},
							Key: "secret-access-key",
						},
					},
				},
				{
					Name: "AWS_ENDPOINT_URL",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: redis.Spec.Backup.Storage.S3.SecretName,
							},
							Key:      "endpoint",
							Optional: &[]bool{true}[0], // Make endpoint optional
						},
					},
				},
			}...)
		}
	}

	backupContainer := corev1.Container{
		Name:            "redis-backup",
		Image:           redis.Spec.Backup.Image,
		ImagePullPolicy: redis.Spec.ImagePullPolicy,
		// Use the image's default entrypoint (/backup)
		Env: envVars,
		// No volume mounts needed for streaming backup
	}

	return backupContainer
}

// BuildBackupInitContainer builds the init container for Redis backup restore
func BuildBackupInitContainer(redis *koncachev1alpha1.Redis) corev1.Container {
	// Build environment variables for backup restore
	envVars := []corev1.EnvVar{
		{
			Name:  "DATA_DIR",
			Value: "/data", // Redis data directory
		},
	}

	// Add S3 configuration if storage is configured
	if redis.Spec.Backup.Storage.S3 != nil {
		s3Prefix := redis.Spec.Backup.Storage.S3.Prefix
		if s3Prefix == "" {
			s3Prefix = fmt.Sprintf("%s/%s", redis.Namespace, redis.Name)
		}

		envVars = append(envVars, []corev1.EnvVar{
			{
				Name:  "S3_BUCKET",
				Value: redis.Spec.Backup.Storage.S3.Bucket,
			},
			{
				Name:  "S3_REGION",
				Value: redis.Spec.Backup.Storage.S3.Region,
			},
			{
				Name:  "S3_PREFIX",
				Value: s3Prefix,
			},
		}...)

		// Add S3 credentials from secret
		if redis.Spec.Backup.Storage.S3.SecretName != "" {
			envVars = append(envVars, []corev1.EnvVar{
				{
					Name: "AWS_ACCESS_KEY_ID",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: redis.Spec.Backup.Storage.S3.SecretName,
							},
							Key: "access-key-id",
						},
					},
				},
				{
					Name: "AWS_SECRET_ACCESS_KEY",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: redis.Spec.Backup.Storage.S3.SecretName,
							},
							Key: "secret-access-key",
						},
					},
				},
				{
					Name: "AWS_ENDPOINT_URL",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: redis.Spec.Backup.Storage.S3.SecretName,
							},
							Key:      "endpoint",
							Optional: &[]bool{true}[0], // Make endpoint optional
						},
					},
				},
			}...)
		}
	}

	// Use the image from BackupInitConfig if specified, otherwise use backup image
	image := redis.Spec.Backup.Image
	if redis.Spec.Backup.BackUpInitConfig.Image != "" {
		image = redis.Spec.Backup.BackUpInitConfig.Image
	}

	initContainer := corev1.Container{
		Name:            "backup-init",
		Image:           image,
		ImagePullPolicy: redis.Spec.ImagePullPolicy,
		Command:         []string{"/init-backup"}, // Use the init-backup entrypoint
		Env:             envVars,
		SecurityContext: &corev1.SecurityContext{
			RunAsUser:  &[]int64{999}[0], // Run as Redis user to have write permissions to /data
			RunAsGroup: &[]int64{999}[0], // Run as Redis group
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "redis-data",
				MountPath: "/data",
			},
		},
	}

	return initContainer
}
