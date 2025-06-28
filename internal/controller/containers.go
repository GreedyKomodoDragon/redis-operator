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
