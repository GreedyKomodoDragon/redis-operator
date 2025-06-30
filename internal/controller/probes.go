package controller

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	koncachev1alpha1 "github.com/GreedyKomodoDragon/redis-operator/api/v1alpha1"
)

// BuildSecurityProbes builds health probes that work with security settings
func BuildSecurityProbes(redis *koncachev1alpha1.Redis, port int32) (*corev1.Probe, *corev1.Probe) {
	var livenessProbe, readinessProbe *corev1.Probe

	tlsEnabled := IsTLSEnabled(redis)
	authEnabled := IsAuthEnabled(redis)

	if tlsEnabled && authEnabled {
		// Both TLS and auth enabled - use TCP for liveness (basic connectivity)
		// and exec with TLS redis-cli for readiness (full functionality check)
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
				Exec: &corev1.ExecAction{
					Command: []string{
						"sh", "-c",
						"redis-cli --tls --cert /etc/redis/tls/tls.crt --key /etc/redis/tls/tls.key --cacert /etc/redis/tls/ca.crt -p 6380 -a $REDIS_PASSWORD ping | grep PONG",
					},
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       10,
			TimeoutSeconds:      1,
			FailureThreshold:    3,
		}
	} else if tlsEnabled {
		// Only TLS enabled - use TCP for liveness and TLS redis-cli for readiness
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
				Exec: &corev1.ExecAction{
					Command: []string{
						"sh", "-c",
						"redis-cli --tls --cert /etc/redis/tls/tls.crt --key /etc/redis/tls/tls.key --cacert /etc/redis/tls/ca.crt -p 6380 ping | grep PONG",
					},
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       10,
			TimeoutSeconds:      1,
			FailureThreshold:    3,
		}
	} else if authEnabled {
		// Only auth enabled - use TCP for liveness and authenticated ping for readiness
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
		// No security - standard probes
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
