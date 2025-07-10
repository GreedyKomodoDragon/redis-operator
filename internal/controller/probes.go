package controller

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	koncachev1alpha1 "github.com/GreedyKomodoDragon/redis-operator/api/v1alpha1"
)

// BuildSecurityProbes builds health probes that work with security settings
func BuildSecurityProbes(redis *koncachev1alpha1.Redis, port int32) (*corev1.Probe, *corev1.Probe) {
	var livenessProbe, readinessProbe *corev1.Probe

	// Get probe configurations
	livenessEnabled := IsLivenessProbeEnabled(redis)
	readinessEnabled := IsReadinessProbeEnabled(redis)

	livenessInitialDelay, livenessPeriod, livenessTimeout, livenessFailureThreshold := GetLivenessProbeConfig(redis)
	readinessInitialDelay, readinessPeriod, readinessTimeout, readinessFailureThreshold, readinessSuccessThreshold := GetReadinessProbeConfig(redis)

	tlsEnabled := IsTLSEnabled(redis)
	authEnabled := IsAuthEnabled(redis)

	if tlsEnabled && authEnabled {
		// Both TLS and auth enabled - use TCP for liveness (basic connectivity)
		// and exec with TLS redis-cli for readiness (full functionality check)
		if livenessEnabled {
			livenessProbe = &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					TCPSocket: &corev1.TCPSocketAction{
						Port: intstr.FromInt(6380), // TLS port
					},
				},
				InitialDelaySeconds: livenessInitialDelay,
				PeriodSeconds:       livenessPeriod,
				TimeoutSeconds:      livenessTimeout,
				FailureThreshold:    livenessFailureThreshold,
			}
		}

		if readinessEnabled {
			readinessProbe = &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					Exec: &corev1.ExecAction{
						Command: []string{
							"sh", "-c",
							"redis-cli --tls --cert /etc/redis/tls/tls.crt --key /etc/redis/tls/tls.key --cacert /etc/redis/tls/ca.crt -p 6380 -a $REDIS_PASSWORD ping | grep PONG",
						},
					},
				},
				InitialDelaySeconds: readinessInitialDelay,
				PeriodSeconds:       readinessPeriod,
				TimeoutSeconds:      readinessTimeout,
				FailureThreshold:    readinessFailureThreshold,
				SuccessThreshold:    readinessSuccessThreshold,
			}
		}
	} else if tlsEnabled {
		// Only TLS enabled - use TCP for liveness and TLS redis-cli for readiness
		if livenessEnabled {
			livenessProbe = &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					TCPSocket: &corev1.TCPSocketAction{
						Port: intstr.FromInt(6380), // TLS port
					},
				},
				InitialDelaySeconds: livenessInitialDelay,
				PeriodSeconds:       livenessPeriod,
				TimeoutSeconds:      livenessTimeout,
				FailureThreshold:    livenessFailureThreshold,
			}
		}

		if readinessEnabled {
			readinessProbe = &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					Exec: &corev1.ExecAction{
						Command: []string{
							"sh", "-c",
							"redis-cli --tls --cert /etc/redis/tls/tls.crt --key /etc/redis/tls/tls.key --cacert /etc/redis/tls/ca.crt -p 6380 ping | grep PONG",
						},
					},
				},
				InitialDelaySeconds: readinessInitialDelay,
				PeriodSeconds:       readinessPeriod,
				TimeoutSeconds:      readinessTimeout,
				FailureThreshold:    readinessFailureThreshold,
				SuccessThreshold:    readinessSuccessThreshold,
			}
		}
	} else if authEnabled {
		// Only auth enabled - use TCP for liveness and authenticated ping for readiness
		if livenessEnabled {
			livenessProbe = &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					TCPSocket: &corev1.TCPSocketAction{
						Port: intstr.FromInt(int(port)),
					},
				},
				InitialDelaySeconds: livenessInitialDelay,
				PeriodSeconds:       livenessPeriod,
				TimeoutSeconds:      livenessTimeout,
				FailureThreshold:    livenessFailureThreshold,
			}
		}

		if readinessEnabled {
			readinessProbe = &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					Exec: &corev1.ExecAction{
						Command: []string{
							"sh", "-c",
							"redis-cli -a $REDIS_PASSWORD ping | grep PONG",
						},
					},
				},
				InitialDelaySeconds: readinessInitialDelay,
				PeriodSeconds:       readinessPeriod,
				TimeoutSeconds:      readinessTimeout,
				FailureThreshold:    readinessFailureThreshold,
				SuccessThreshold:    readinessSuccessThreshold,
			}
		}
	} else {
		// No security - standard probes
		if livenessEnabled {
			livenessProbe = &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					TCPSocket: &corev1.TCPSocketAction{
						Port: intstr.FromInt(int(port)),
					},
				},
				InitialDelaySeconds: livenessInitialDelay,
				PeriodSeconds:       livenessPeriod,
				TimeoutSeconds:      livenessTimeout,
				FailureThreshold:    livenessFailureThreshold,
			}
		}

		if readinessEnabled {
			readinessProbe = &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					Exec: &corev1.ExecAction{
						Command: []string{"redis-cli", "ping"},
					},
				},
				InitialDelaySeconds: readinessInitialDelay,
				PeriodSeconds:       readinessPeriod,
				TimeoutSeconds:      readinessTimeout,
				FailureThreshold:    readinessFailureThreshold,
				SuccessThreshold:    readinessSuccessThreshold,
			}
		}
	}

	return livenessProbe, readinessProbe
}

// Helper functions for probe configuration

// IsLivenessProbeEnabled checks if liveness probe is enabled
func IsLivenessProbeEnabled(redis *koncachev1alpha1.Redis) bool {
	if redis.Spec.Probes != nil && redis.Spec.Probes.LivenessProbe != nil && redis.Spec.Probes.LivenessProbe.Enabled != nil {
		return *redis.Spec.Probes.LivenessProbe.Enabled
	}
	return true // default enabled
}

// IsReadinessProbeEnabled checks if readiness probe is enabled
func IsReadinessProbeEnabled(redis *koncachev1alpha1.Redis) bool {
	if redis.Spec.Probes != nil && redis.Spec.Probes.ReadinessProbe != nil && redis.Spec.Probes.ReadinessProbe.Enabled != nil {
		return *redis.Spec.Probes.ReadinessProbe.Enabled
	}
	return true // default enabled
}

// GetLivenessProbeConfig gets liveness probe configuration with defaults
func GetLivenessProbeConfig(redis *koncachev1alpha1.Redis) (int32, int32, int32, int32) {
	var initialDelay, period, timeout, failureThreshold int32 = 30, 10, 5, 10 // defaults

	if redis.Spec.Probes != nil && redis.Spec.Probes.LivenessProbe != nil {
		probe := redis.Spec.Probes.LivenessProbe
		if probe.InitialDelaySeconds != nil {
			initialDelay = *probe.InitialDelaySeconds
		}
		if probe.PeriodSeconds != nil {
			period = *probe.PeriodSeconds
		}
		if probe.TimeoutSeconds != nil {
			timeout = *probe.TimeoutSeconds
		}
		if probe.FailureThreshold != nil {
			failureThreshold = *probe.FailureThreshold
		}
	}

	return initialDelay, period, timeout, failureThreshold
}

// GetReadinessProbeConfig gets readiness probe configuration with defaults
func GetReadinessProbeConfig(redis *koncachev1alpha1.Redis) (int32, int32, int32, int32, int32) {
	var initialDelay, period, timeout, failureThreshold, successThreshold int32 = 5, 10, 5, 3, 1 // defaults

	if redis.Spec.Probes != nil && redis.Spec.Probes.ReadinessProbe != nil {
		probe := redis.Spec.Probes.ReadinessProbe
		if probe.InitialDelaySeconds != nil {
			initialDelay = *probe.InitialDelaySeconds
		}
		if probe.PeriodSeconds != nil {
			period = *probe.PeriodSeconds
		}
		if probe.TimeoutSeconds != nil {
			timeout = *probe.TimeoutSeconds
		}
		if probe.FailureThreshold != nil {
			failureThreshold = *probe.FailureThreshold
		}
		if probe.SuccessThreshold != nil {
			successThreshold = *probe.SuccessThreshold
		}
	}

	return initialDelay, period, timeout, failureThreshold, successThreshold
}
