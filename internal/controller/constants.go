package controller

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
