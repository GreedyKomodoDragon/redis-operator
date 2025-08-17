package controller

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	koncachev1alpha1 "github.com/GreedyKomodoDragon/redis-operator/api/v1alpha1"
)

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
		config := "protected-mode yes\nrequirepass $REDIS_PASSWORD\n"

		// For HA deployments, also add masterauth for replication authentication
		if redis.Spec.HighAvailability != nil && redis.Spec.HighAvailability.Enabled {
			config += "masterauth $REDIS_PASSWORD\n"
		}

		return config
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
	} else if redis.Spec.Security.TLS.CASecret != nil && redis.Spec.Security.TLS.CASecret.Name != "" {
		// If CA secret is the same as cert secret, CA is in the same volume
		if redis.Spec.Security.TLS.CASecret.Name == redis.Spec.Security.TLS.CertSecret {
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
