package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	koncachev1alpha1 "github.com/GreedyKomodoDragon/redis-operator/api/v1alpha1"
)

func TestBuildSecurityProbes(t *testing.T) {
	tests := []struct {
		name          string
		redis         *koncachev1alpha1.Redis
		port          int32
		expectedLive  string // type of liveness probe
		expectedReady string // type of readiness probe
		checkTLSPort  bool   // whether TLS port should be used
		checkAuthCmd  bool   // whether auth command should be used
		checkTLSCmd   bool   // whether TLS command should be used
	}{
		{
			name: "no security",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{},
			},
			port:          6379,
			expectedLive:  "tcp",
			expectedReady: "exec",
			checkTLSPort:  false,
			checkAuthCmd:  false,
			checkTLSCmd:   false,
		},
		{
			name: "auth only",
			redis: &koncachev1alpha1.Redis{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-redis",
				},
				Spec: koncachev1alpha1.RedisSpec{
					Security: koncachev1alpha1.RedisSecurity{
						RequireAuth: &[]bool{true}[0],
					},
				},
			},
			port:          6379,
			expectedLive:  "tcp",
			expectedReady: "exec",
			checkTLSPort:  false,
			checkAuthCmd:  true,
			checkTLSCmd:   false,
		},
		{
			name: "TLS only",
			redis: &koncachev1alpha1.Redis{
				Spec: koncachev1alpha1.RedisSpec{
					Security: koncachev1alpha1.RedisSecurity{
						TLS: &koncachev1alpha1.RedisTLS{
							Enabled:    true,
							CertSecret: "tls-certs",
						},
					},
				},
			},
			port:          6379,
			expectedLive:  "tcp",
			expectedReady: "exec",
			checkTLSPort:  true,
			checkAuthCmd:  false,
			checkTLSCmd:   true,
		},
		{
			name: "both TLS and auth",
			redis: &koncachev1alpha1.Redis{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-redis",
				},
				Spec: koncachev1alpha1.RedisSpec{
					Security: koncachev1alpha1.RedisSecurity{
						RequireAuth: &[]bool{true}[0],
						TLS: &koncachev1alpha1.RedisTLS{
							Enabled:    true,
							CertSecret: "tls-certs",
						},
					},
				},
			},
			port:          6379,
			expectedLive:  "tcp",
			expectedReady: "exec",
			checkTLSPort:  true,
			checkAuthCmd:  true,
			checkTLSCmd:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			livenessProbe, readinessProbe := BuildSecurityProbes(tt.redis, tt.port)

			require.NotNil(t, livenessProbe, "Liveness probe should not be nil")
			require.NotNil(t, readinessProbe, "Readiness probe should not be nil")

			// Check liveness probe type
			if tt.expectedLive == "tcp" {
				assert.NotNil(t, livenessProbe.TCPSocket, "Liveness probe should use TCP socket")
				assert.Nil(t, livenessProbe.Exec, "Liveness probe should not use exec")
			} else if tt.expectedLive == "exec" {
				assert.NotNil(t, livenessProbe.Exec, "Liveness probe should use exec")
				assert.Nil(t, livenessProbe.TCPSocket, "Liveness probe should not use TCP socket")
			}

			// Check readiness probe type
			if tt.expectedReady == "tcp" {
				assert.NotNil(t, readinessProbe.TCPSocket, "Readiness probe should use TCP socket")
				assert.Nil(t, readinessProbe.Exec, "Readiness probe should not use exec")
			} else if tt.expectedReady == "exec" {
				assert.NotNil(t, readinessProbe.Exec, "Readiness probe should use exec")
				assert.Nil(t, readinessProbe.TCPSocket, "Readiness probe should not use TCP socket")
			}

			// Check TLS port usage
			if tt.checkTLSPort {
				if livenessProbe.TCPSocket != nil {
					assert.Equal(t, int32(6380), livenessProbe.TCPSocket.Port.IntVal, "Liveness probe should use TLS port")
				}
				if readinessProbe.TCPSocket != nil {
					assert.Equal(t, int32(6380), readinessProbe.TCPSocket.Port.IntVal, "Readiness probe should use TLS port")
				}
			} else {
				if livenessProbe.TCPSocket != nil {
					assert.Equal(t, tt.port, livenessProbe.TCPSocket.Port.IntVal, "Liveness probe should use standard port")
				}
			}

			// Check auth command usage
			if readinessProbe.Exec != nil {
				cmdStr := ""
				if len(readinessProbe.Exec.Command) > 0 {
					cmdStr = readinessProbe.Exec.Command[len(readinessProbe.Exec.Command)-1]
				}

				if tt.checkAuthCmd {
					assert.Contains(t, cmdStr, "$REDIS_PASSWORD", "Readiness probe should include auth when auth is enabled")
				} else {
					assert.NotContains(t, cmdStr, "$REDIS_PASSWORD", "Readiness probe should not include auth when auth is disabled")
				}

				if tt.checkTLSCmd {
					assert.Contains(t, cmdStr, "--tls", "Readiness probe should include TLS flags when TLS is enabled")
					assert.Contains(t, cmdStr, "6380", "Readiness probe should use TLS port when TLS is enabled")
				} else {
					assert.NotContains(t, cmdStr, "--tls", "Readiness probe should not include TLS flags when TLS is disabled")
				}
			}

			// Verify probe timing configuration
			assert.Equal(t, int32(30), livenessProbe.InitialDelaySeconds, "Liveness probe initial delay should be 30s")
			assert.Equal(t, int32(5), readinessProbe.InitialDelaySeconds, "Readiness probe initial delay should be 5s")
			assert.Equal(t, int32(10), livenessProbe.PeriodSeconds, "Liveness probe period should be 10s")
			assert.Equal(t, int32(10), readinessProbe.PeriodSeconds, "Readiness probe period should be 10s")
		})
	}
}

func TestBuildSecurityProbesReturnsBothProbes(t *testing.T) {
	redis := &koncachev1alpha1.Redis{
		Spec: koncachev1alpha1.RedisSpec{},
	}

	livenessProbe, readinessProbe := BuildSecurityProbes(redis, 6379)

	assert.NotNil(t, livenessProbe, "Should return a liveness probe")
	assert.NotNil(t, readinessProbe, "Should return a readiness probe")
	assert.NotEqual(t, livenessProbe, readinessProbe, "Liveness and readiness probes should be different objects")
}
