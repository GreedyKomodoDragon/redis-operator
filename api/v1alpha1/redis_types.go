/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// RedisSpec defines the desired state of Redis.
type RedisSpec struct {
	// Mode specifies the Redis deployment mode (standalone, cluster)
	// +kubebuilder:default="standalone"
	// +kubebuilder:validation:Enum=standalone;cluster
	Mode RedisMode `json:"mode,omitempty"`

	// Version specifies the Redis version to deploy
	// +kubebuilder:default="7.2"
	Version string `json:"version,omitempty"`

	// Image specifies the Redis container image
	// +kubebuilder:default="redis:7.2-alpine"
	Image string `json:"image,omitempty"`

	// ImagePullPolicy specifies the image pull policy
	// +kubebuilder:default="IfNotPresent"
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// ImagePullSecrets specifies the secrets to use for pulling images
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// Port specifies the Redis port
	// +kubebuilder:default=6379
	Port int32 `json:"port,omitempty"`

	// Resources specifies the resource requirements for the Redis container
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Storage specifies the storage configuration
	Storage RedisStorage `json:"storage,omitempty"`

	// Config specifies Redis-specific configuration options
	Config RedisConfig `json:"config,omitempty"`

	// Cluster specifies cluster-specific configuration (only for cluster mode)
	Cluster *RedisCluster `json:"cluster,omitempty"`

	// Security specifies security-related configuration
	Security RedisSecurity `json:"security,omitempty"`

	// Monitoring specifies monitoring configuration
	Monitoring RedisMonitoring `json:"monitoring,omitempty"`

	// ServiceAccount specifies the service account to use
	ServiceAccount string `json:"serviceAccount,omitempty"`

	// NodeSelector specifies node selection constraints
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations specifies pod tolerations
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Affinity specifies pod affinity rules
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// PodAnnotations specifies additional annotations for the pod
	PodAnnotations map[string]string `json:"podAnnotations,omitempty"`

	// PodLabels specifies additional labels for the pod
	PodLabels map[string]string `json:"podLabels,omitempty"`

	// ServiceAnnotations specifies additional annotations for the service
	ServiceAnnotations map[string]string `json:"serviceAnnotations,omitempty"`

	// ServiceLabels specifies additional labels for the service
	ServiceLabels map[string]string `json:"serviceLabels,omitempty"`

	// ServiceType specifies the type of service to create
	// +kubebuilder:default="ClusterIP"
	ServiceType corev1.ServiceType `json:"serviceType,omitempty"`

	// Backup specifies backup configuration
	Backup *RedisBackup `json:"backup,omitempty"`

	// HighAvailability specifies high availability configuration
	HighAvailability *RedisHighAvailability `json:"highAvailability,omitempty"`

	// Networking specifies advanced networking configuration
	Networking *RedisNetworking `json:"networking,omitempty"`

	// Probes specifies health probe configuration
	Probes *RedisProbes `json:"probes,omitempty"`
}

// RedisStorage defines storage configuration for Redis
type RedisStorage struct {
	// Size specifies the size of the persistent volume
	// +kubebuilder:default="1Gi"
	Size resource.Quantity `json:"size,omitempty"`

	// StorageClassName specifies the storage class name
	StorageClassName *string `json:"storageClassName,omitempty"`

	// AccessModes specifies the access modes for the persistent volume
	// +kubebuilder:default={"ReadWriteOnce"}
	AccessModes []corev1.PersistentVolumeAccessMode `json:"accessModes,omitempty"`

	// VolumeMode specifies the volume mode
	VolumeMode *corev1.PersistentVolumeMode `json:"volumeMode,omitempty"`
}

// RedisConfig defines Redis-specific configuration
type RedisConfig struct {
	// MaxMemory specifies the maximum memory usage
	MaxMemory string `json:"maxMemory,omitempty"`

	// MaxMemoryPolicy specifies the policy when max memory is reached
	// +kubebuilder:default="allkeys-lru"
	MaxMemoryPolicy string `json:"maxMemoryPolicy,omitempty"`

	// Save specifies the save intervals for RDB snapshots
	// +kubebuilder:default={"900 1","300 10","60 10000"}
	Save []string `json:"save,omitempty"`

	// AppendOnly enables AOF persistence
	// +kubebuilder:default=true
	AppendOnly *bool `json:"appendOnly,omitempty"`

	// AppendFsync specifies the AOF fsync policy
	// +kubebuilder:default="everysec"
	AppendFsync string `json:"appendFsync,omitempty"`

	// Timeout specifies the client timeout in seconds
	// +kubebuilder:default=0
	Timeout int32 `json:"timeout,omitempty"`

	// TCP keepalive time
	// +kubebuilder:default=300
	TCPKeepAlive int32 `json:"tcpKeepAlive,omitempty"`

	// Databases specifies the number of databases
	// +kubebuilder:default=16
	Databases int32 `json:"databases,omitempty"`

	// LogLevel specifies the log level
	// +kubebuilder:default="notice"
	LogLevel string `json:"logLevel,omitempty"`

	// Additional custom configuration
	AdditionalConfig map[string]string `json:"additionalConfig,omitempty"`
}

// RedisMode represents the deployment mode of Redis
type RedisMode string

const (
	// RedisModeStandalone indicates a standalone Redis instance
	RedisModeStandalone RedisMode = "standalone"

	// RedisModeCluster indicates a Redis cluster
	RedisModeCluster RedisMode = "cluster"
)

// RedisCluster defines cluster-specific configuration
type RedisCluster struct {
	// Replicas specifies the number of master nodes in the cluster
	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=3
	Replicas int32 `json:"replicas,omitempty"`

	// ReplicasPerMaster specifies the number of replica nodes per master
	// +kubebuilder:default=1
	ReplicasPerMaster int32 `json:"replicasPerMaster,omitempty"`

	// ClusterBusPort specifies the cluster bus port
	// +kubebuilder:default=16379
	ClusterBusPort int32 `json:"clusterBusPort,omitempty"`

	// InitialMaster specifies whether to initialize cluster automatically
	// +kubebuilder:default=true
	InitialMaster *bool `json:"initialMaster,omitempty"`

	// ClusterRequireFullCoverage specifies whether to require full coverage
	// +kubebuilder:default=true
	ClusterRequireFullCoverage *bool `json:"clusterRequireFullCoverage,omitempty"`

	// ClusterNodeTimeout specifies the cluster node timeout in milliseconds
	// +kubebuilder:default=15000
	ClusterNodeTimeout int32 `json:"clusterNodeTimeout,omitempty"`

	// ClusterMigrationBarrier specifies the migration barrier
	// +kubebuilder:default=1
	ClusterMigrationBarrier int32 `json:"clusterMigrationBarrier,omitempty"`

	// ClusterSlaveValidityFactor specifies the slave validity factor
	// +kubebuilder:default=10
	ClusterSlaveValidityFactor int32 `json:"clusterSlaveValidityFactor,omitempty"`

	// AntiAffinity enables pod anti-affinity for cluster nodes
	// +kubebuilder:default=true
	AntiAffinity *bool `json:"antiAffinity,omitempty"`

	// PodDisruptionBudget specifies PDB configuration for cluster
	PodDisruptionBudget *RedisPodDisruptionBudget `json:"podDisruptionBudget,omitempty"`
}

// RedisHighAvailability defines high availability configuration
type RedisHighAvailability struct {
	// Enabled specifies whether high availability is enabled
	Enabled bool `json:"enabled"`

	// AutoFailover enables automatic failover
	// +kubebuilder:default=true
	AutoFailover *bool `json:"autoFailover,omitempty"`

	// HealthCheck specifies health check configuration
	HealthCheck RedisHealthCheck `json:"healthCheck,omitempty"`

	// BackupOnFailover enables backup before failover
	// +kubebuilder:default=true
	BackupOnFailover *bool `json:"backupOnFailover,omitempty"`

	// Replicas specifies the replication factor for HA
	// +kubebuilder:default=2
	Replicas int32 `json:"replicas,omitempty"`
}

// RedisHealthCheck defines health check configuration
type RedisHealthCheck struct {
	// Enabled specifies whether health checks are enabled
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty"`

	// InitialDelaySeconds specifies the initial delay for health checks
	// +kubebuilder:default=30
	InitialDelaySeconds int32 `json:"initialDelaySeconds,omitempty"`

	// PeriodSeconds specifies the period between health checks
	// +kubebuilder:default=10
	PeriodSeconds int32 `json:"periodSeconds,omitempty"`

	// TimeoutSeconds specifies the timeout for health checks
	// +kubebuilder:default=5
	TimeoutSeconds int32 `json:"timeoutSeconds,omitempty"`

	// FailureThreshold specifies the failure threshold
	// +kubebuilder:default=3
	FailureThreshold int32 `json:"failureThreshold,omitempty"`

	// SuccessThreshold specifies the success threshold
	// +kubebuilder:default=1
	SuccessThreshold int32 `json:"successThreshold,omitempty"`
}

// RedisNetworking defines advanced networking configuration
type RedisNetworking struct {
	// LoadBalancer specifies load balancer configuration
	LoadBalancer *RedisLoadBalancer `json:"loadBalancer,omitempty"`

	// Ingress specifies ingress configuration
	Ingress *RedisIngress `json:"ingress,omitempty"`

	// NetworkPolicy specifies network policy configuration
	NetworkPolicy *RedisNetworkPolicy `json:"networkPolicy,omitempty"`

	// ExternalAccess specifies external access configuration
	ExternalAccess *RedisExternalAccess `json:"externalAccess,omitempty"`

	// Istio specifies Istio service mesh configuration
	Istio *RedisIstio `json:"istio,omitempty"`
}

// RedisLoadBalancer defines load balancer configuration
type RedisLoadBalancer struct {
	// Enabled specifies whether load balancer is enabled
	Enabled bool `json:"enabled"`

	// Type specifies the load balancer type
	Type string `json:"type,omitempty"`

	// Annotations specifies additional annotations for the load balancer
	Annotations map[string]string `json:"annotations,omitempty"`

	// LoadBalancerIP specifies the IP address for the load balancer
	LoadBalancerIP string `json:"loadBalancerIP,omitempty"`
}

// RedisIngress defines ingress configuration
type RedisIngress struct {
	// Enabled specifies whether ingress is enabled
	Enabled bool `json:"enabled"`

	// ClassName specifies the ingress class name
	ClassName string `json:"className,omitempty"`

	// Host specifies the ingress host
	Host string `json:"host,omitempty"`

	// Path specifies the ingress path
	Path string `json:"path,omitempty"`

	// Annotations specifies additional annotations for the ingress
	Annotations map[string]string `json:"annotations,omitempty"`

	// TLS specifies TLS configuration for ingress
	TLS []networkingv1.IngressTLS `json:"tls,omitempty"`
}

// RedisNetworkPolicy defines network policy configuration
type RedisNetworkPolicy struct {
	// Enabled specifies whether network policy is enabled
	Enabled bool `json:"enabled"`

	// IngressRules specifies ingress rules
	IngressRules []RedisNetworkPolicyRule `json:"ingressRules,omitempty"`

	// EgressRules specifies egress rules
	EgressRules []RedisNetworkPolicyRule `json:"egressRules,omitempty"`
}

// RedisNetworkPolicyRule defines a network policy rule
type RedisNetworkPolicyRule struct {
	// Ports specifies the ports for the rule
	Ports []networkingv1.NetworkPolicyPort `json:"ports,omitempty"`

	// From specifies the source for ingress rules
	From []networkingv1.NetworkPolicyPeer `json:"from,omitempty"`

	// To specifies the destination for egress rules
	To []networkingv1.NetworkPolicyPeer `json:"to,omitempty"`
}

// RedisExternalAccess defines external access configuration
type RedisExternalAccess struct {
	// Enabled specifies whether external access is enabled
	Enabled bool `json:"enabled"`

	// Type specifies the external access type (NodePort, LoadBalancer)
	Type string `json:"type,omitempty"`

	// NodePort specifies the node port (for NodePort type)
	NodePort int32 `json:"nodePort,omitempty"`

	// ExternalIPs specifies external IPs
	ExternalIPs []string `json:"externalIPs,omitempty"`
}

// RedisIstio defines Istio service mesh configuration
type RedisIstio struct {
	// Enabled specifies whether Istio integration is enabled
	Enabled bool `json:"enabled"`

	// SidecarInjection specifies sidecar injection configuration
	SidecarInjection *RedisIstioSidecarInjection `json:"sidecarInjection,omitempty"`

	// VirtualService specifies VirtualService configuration
	VirtualService *RedisIstioVirtualService `json:"virtualService,omitempty"`

	// DestinationRule specifies DestinationRule configuration
	DestinationRule *RedisIstioDestinationRule `json:"destinationRule,omitempty"`

	// ServiceEntry specifies ServiceEntry configuration
	ServiceEntry *RedisIstioServiceEntry `json:"serviceEntry,omitempty"`

	// PeerAuthentication specifies PeerAuthentication configuration
	PeerAuthentication *RedisIstioPeerAuthentication `json:"peerAuthentication,omitempty"`
}

// RedisIstioSidecarInjection defines sidecar injection configuration
type RedisIstioSidecarInjection struct {
	// Enabled specifies whether automatic sidecar injection is enabled
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty"`

	// Template specifies the sidecar injection template
	Template string `json:"template,omitempty"`

	// ProxyResources specifies resource limits for the Istio proxy
	ProxyResources *corev1.ResourceRequirements `json:"proxyResources,omitempty"`

	// ProxyMetadata specifies additional metadata for the proxy
	ProxyMetadata map[string]string `json:"proxyMetadata,omitempty"`
}

// RedisIstioVirtualService defines VirtualService configuration
type RedisIstioVirtualService struct {
	// Enabled specifies whether VirtualService creation is enabled
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty"`

	// Hosts specifies the hosts for the VirtualService
	Hosts []string `json:"hosts,omitempty"`

	// Gateways specifies the gateways for the VirtualService
	Gateways []string `json:"gateways,omitempty"`

	// TrafficPolicy specifies traffic management policies
	TrafficPolicy *RedisIstioTrafficPolicy `json:"trafficPolicy,omitempty"`

	// FaultInjection specifies fault injection configuration
	FaultInjection *RedisIstioFaultInjection `json:"faultInjection,omitempty"`

	// Timeout specifies request timeout
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// Retries specifies retry configuration
	Retries *RedisIstioRetryPolicy `json:"retries,omitempty"`
}

// RedisIstioDestinationRule defines DestinationRule configuration
type RedisIstioDestinationRule struct {
	// Enabled specifies whether DestinationRule creation is enabled
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty"`

	// TrafficPolicy specifies traffic policies
	TrafficPolicy *RedisIstioTrafficPolicy `json:"trafficPolicy,omitempty"`

	// Subsets specifies traffic subsets
	Subsets []RedisIstioSubset `json:"subsets,omitempty"`
}

// RedisIstioServiceEntry defines ServiceEntry configuration
type RedisIstioServiceEntry struct {
	// Enabled specifies whether ServiceEntry creation is enabled
	Enabled *bool `json:"enabled,omitempty"`

	// Hosts specifies the hosts for the ServiceEntry
	Hosts []string `json:"hosts,omitempty"`

	// Ports specifies the ports for the ServiceEntry
	Ports []RedisIstioServiceEntryPort `json:"ports,omitempty"`

	// Location specifies the location of the service (MESH_EXTERNAL, MESH_INTERNAL)
	// +kubebuilder:default="MESH_EXTERNAL"
	Location string `json:"location,omitempty"`

	// Resolution specifies how to resolve the service (DNS, STATIC, NONE)
	// +kubebuilder:default="DNS"
	Resolution string `json:"resolution,omitempty"`
}

// RedisIstioPeerAuthentication defines PeerAuthentication configuration
type RedisIstioPeerAuthentication struct {
	// Enabled specifies whether PeerAuthentication creation is enabled
	Enabled *bool `json:"enabled,omitempty"`

	// MutualTLS specifies mutual TLS configuration
	MutualTLS *RedisIstioMutualTLS `json:"mutualTLS,omitempty"`
}

// RedisIstioTrafficPolicy defines traffic management policies
type RedisIstioTrafficPolicy struct {
	// LoadBalancer specifies load balancing configuration
	LoadBalancer *RedisIstioLoadBalancer `json:"loadBalancer,omitempty"`

	// ConnectionPool specifies connection pool settings
	ConnectionPool *RedisIstioConnectionPool `json:"connectionPool,omitempty"`

	// OutlierDetection specifies outlier detection configuration
	OutlierDetection *RedisIstioOutlierDetection `json:"outlierDetection,omitempty"`
}

// RedisIstioLoadBalancer defines load balancing configuration
type RedisIstioLoadBalancer struct {
	// Simple specifies simple load balancing algorithm (ROUND_ROBIN, LEAST_CONN, RANDOM, PASSTHROUGH)
	// +kubebuilder:default="ROUND_ROBIN"
	Simple string `json:"simple,omitempty"`

	// ConsistentHash specifies consistent hash-based load balancing
	ConsistentHash *RedisIstioConsistentHash `json:"consistentHash,omitempty"`
}

// RedisIstioConsistentHash defines consistent hash configuration
type RedisIstioConsistentHash struct {
	// HTTPHeaderName specifies HTTP header name for hashing
	HTTPHeaderName string `json:"httpHeaderName,omitempty"`

	// HTTPCookie specifies HTTP cookie for hashing
	HTTPCookie *RedisIstioHTTPCookie `json:"httpCookie,omitempty"`

	// UseSourceIP specifies whether to use source IP for hashing
	UseSourceIP bool `json:"useSourceIp,omitempty"`

	// RingHash specifies ring hash configuration
	RingHash *RedisIstioRingHash `json:"ringHash,omitempty"`
}

// RedisIstioHTTPCookie defines HTTP cookie configuration
type RedisIstioHTTPCookie struct {
	// Name specifies the cookie name
	Name string `json:"name"`

	// TTL specifies the cookie TTL
	TTL *metav1.Duration `json:"ttl,omitempty"`
}

// RedisIstioRingHash defines ring hash configuration
type RedisIstioRingHash struct {
	// MinimumRingSize specifies minimum ring size
	MinimumRingSize int32 `json:"minimumRingSize,omitempty"`

	// MaximumRingSize specifies maximum ring size
	MaximumRingSize int32 `json:"maximumRingSize,omitempty"`
}

// RedisIstioConnectionPool defines connection pool settings
type RedisIstioConnectionPool struct {
	// TCP specifies TCP connection pool settings
	TCP *RedisIstioTCPSettings `json:"tcp,omitempty"`

	// HTTP specifies HTTP connection pool settings
	HTTP *RedisIstioHTTPSettings `json:"http,omitempty"`
}

// RedisIstioTCPSettings defines TCP connection settings
type RedisIstioTCPSettings struct {
	// MaxConnections specifies maximum connections
	MaxConnections int32 `json:"maxConnections,omitempty"`

	// ConnectTimeout specifies connection timeout
	ConnectTimeout *metav1.Duration `json:"connectTimeout,omitempty"`

	// TCPNoDelay specifies whether to disable TCP delay
	TCPNoDelay bool `json:"tcpNoDelay,omitempty"`

	// KeepaliveTime specifies keepalive time
	KeepaliveTime *metav1.Duration `json:"keepaliveTime,omitempty"`
}

// RedisIstioHTTPSettings defines HTTP connection settings
type RedisIstioHTTPSettings struct {
	// HTTP1MaxPendingRequests specifies max pending HTTP/1.1 requests
	HTTP1MaxPendingRequests int32 `json:"http1MaxPendingRequests,omitempty"`

	// HTTP2MaxRequests specifies max HTTP/2 requests
	HTTP2MaxRequests int32 `json:"http2MaxRequests,omitempty"`

	// MaxRequestsPerConnection specifies max requests per connection
	MaxRequestsPerConnection int32 `json:"maxRequestsPerConnection,omitempty"`

	// MaxRetries specifies max retries
	MaxRetries int32 `json:"maxRetries,omitempty"`

	// IdleTimeout specifies idle timeout
	IdleTimeout *metav1.Duration `json:"idleTimeout,omitempty"`

	// H2UpgradePolicy specifies HTTP/2 upgrade policy
	H2UpgradePolicy string `json:"h2UpgradePolicy,omitempty"`
}

// RedisIstioOutlierDetection defines outlier detection configuration
type RedisIstioOutlierDetection struct {
	// Consecutive5xxErrors specifies threshold for consecutive 5xx errors
	Consecutive5xxErrors int32 `json:"consecutive5xxErrors,omitempty"`

	// ConsecutiveGatewayErrors specifies threshold for consecutive gateway errors
	ConsecutiveGatewayErrors int32 `json:"consecutiveGatewayErrors,omitempty"`

	// Interval specifies analysis interval
	Interval *metav1.Duration `json:"interval,omitempty"`

	// BaseEjectionTime specifies minimum ejection duration
	BaseEjectionTime *metav1.Duration `json:"baseEjectionTime,omitempty"`

	// MaxEjectionPercent specifies maximum percentage of hosts ejected
	MaxEjectionPercent int32 `json:"maxEjectionPercent,omitempty"`

	// MinHealthPercent specifies minimum health percentage
	MinHealthPercent int32 `json:"minHealthPercent,omitempty"`
}

// RedisIstioFaultInjection defines fault injection configuration
type RedisIstioFaultInjection struct {
	// Delay specifies delay injection
	Delay *RedisIstioDelay `json:"delay,omitempty"`

	// Abort specifies abort injection
	Abort *RedisIstioAbort `json:"abort,omitempty"`
}

// RedisIstioDelay defines delay injection configuration
type RedisIstioDelay struct {
	// Percentage specifies delay injection percentage (as string to avoid float64 issues)
	Percentage string `json:"percentage,omitempty"`

	// FixedDelay specifies fixed delay
	FixedDelay *metav1.Duration `json:"fixedDelay,omitempty"`

	// ExponentialDelay specifies exponential delay (not commonly used)
	ExponentialDelay *metav1.Duration `json:"exponentialDelay,omitempty"`
}

// RedisIstioAbort defines abort injection configuration
type RedisIstioAbort struct {
	// Percentage specifies abort injection percentage (as string to avoid float64 issues)
	Percentage string `json:"percentage,omitempty"`

	// HTTPStatus specifies HTTP status code for abort
	HTTPStatus int32 `json:"httpStatus,omitempty"`

	// GRPCStatus specifies gRPC status for abort
	GRPCStatus string `json:"grpcStatus,omitempty"`
}

// RedisIstioRetryPolicy defines retry policy configuration
type RedisIstioRetryPolicy struct {
	// Attempts specifies number of retry attempts
	Attempts int32 `json:"attempts,omitempty"`

	// PerTryTimeout specifies timeout per retry attempt
	PerTryTimeout *metav1.Duration `json:"perTryTimeout,omitempty"`

	// RetryOn specifies conditions for retries
	RetryOn string `json:"retryOn,omitempty"`

	// RetryRemoteLocalities specifies whether to retry remote localities
	RetryRemoteLocalities bool `json:"retryRemoteLocalities,omitempty"`
}

// RedisIstioSubset defines traffic subset configuration
type RedisIstioSubset struct {
	// Name specifies the subset name
	Name string `json:"name"`

	// Labels specifies labels for the subset
	Labels map[string]string `json:"labels"`

	// TrafficPolicy specifies traffic policy for the subset
	TrafficPolicy *RedisIstioTrafficPolicy `json:"trafficPolicy,omitempty"`
}

// RedisIstioServiceEntryPort defines ServiceEntry port configuration
type RedisIstioServiceEntryPort struct {
	// Number specifies the port number
	Number int32 `json:"number"`

	// Name specifies the port name
	Name string `json:"name,omitempty"`

	// Protocol specifies the port protocol (HTTP, HTTPS, TCP, etc.)
	Protocol string `json:"protocol"`

	// TargetPort specifies the target port
	TargetPort int32 `json:"targetPort,omitempty"`
}

// RedisIstioMutualTLS defines mutual TLS configuration
type RedisIstioMutualTLS struct {
	// Mode specifies the mutual TLS mode (STRICT, PERMISSIVE, DISABLE)
	// +kubebuilder:default="STRICT"
	Mode string `json:"mode,omitempty"`
}

// RedisSecurity defines security configuration
type RedisSecurity struct {
	// RequireAuth specifies whether password authentication is required
	RequireAuth *bool `json:"requireAuth,omitempty"`

	// PasswordSecret specifies the secret containing the Redis password
	PasswordSecret *corev1.SecretKeySelector `json:"passwordSecret,omitempty"`

	// RenameCommands specifies command renaming configuration
	RenameCommands map[string]string `json:"renameCommands,omitempty"`

	// Password specifies password configuration
	Password *RedisPassword `json:"password,omitempty"`

	// TLS specifies TLS configuration
	TLS *RedisTLS `json:"tls,omitempty"`

	// ACL specifies Access Control List configuration
	ACL *RedisACL `json:"acl,omitempty"`
}

// RedisPassword defines password configuration
type RedisPassword struct {
	// Enabled specifies whether password authentication is enabled
	Enabled bool `json:"enabled"`

	// SecretRef specifies the secret containing the password
	SecretRef *corev1.SecretKeySelector `json:"secretRef,omitempty"`

	// Value specifies the password value directly (not recommended for production)
	Value string `json:"value,omitempty"`
}

// RedisTLS defines TLS configuration
type RedisTLS struct {
	// Enabled specifies whether TLS is enabled
	Enabled bool `json:"enabled"`

	// CertFile specifies the path to certificate file
	CertFile string `json:"certFile,omitempty"`

	// KeyFile specifies the path to key file
	KeyFile string `json:"keyFile,omitempty"`

	// CAFile specifies the path to CA file
	CAFile string `json:"caFile,omitempty"`

	// CertSecret specifies the secret containing TLS certificate and key
	CertSecret string `json:"certSecret,omitempty"`

	// CertificateRef specifies the secret containing TLS certificates
	CertificateRef *corev1.SecretKeySelector `json:"certificateRef,omitempty"`

	// CARef specifies the secret containing CA certificate
	CARef *corev1.SecretKeySelector `json:"caRef,omitempty"`

	// CASecret specifies the secret containing CA certificate (legacy field)
	CASecret *corev1.SecretKeySelector `json:"caSecret,omitempty"`

	// InsecureSkipVerify specifies whether to skip certificate verification
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
}

// RedisACL defines Access Control List configuration
type RedisACL struct {
	// Enabled specifies whether ACL is enabled
	Enabled bool `json:"enabled"`

	// ConfigRef specifies the configmap containing ACL configuration
	ConfigRef *corev1.ConfigMapKeySelector `json:"configRef,omitempty"`

	// Users specifies ACL users configuration
	Users []RedisACLUser `json:"users,omitempty"`
}

// RedisACLUser defines individual ACL user configuration
type RedisACLUser struct {
	// Username specifies the username
	Username string `json:"username"`

	// PasswordRef specifies the secret containing the user password
	PasswordRef *corev1.SecretKeySelector `json:"passwordRef,omitempty"`

	// Categories specifies allowed categories
	Categories []string `json:"categories,omitempty"`

	// Commands specifies allowed commands
	Commands []string `json:"commands,omitempty"`

	// Keys specifies allowed key patterns
	Keys []string `json:"keys,omitempty"`
}

// RedisMonitoring defines monitoring configuration
type RedisMonitoring struct {
	// Enabled specifies whether monitoring is enabled
	Enabled bool `json:"enabled"`

	// PodMonitor specifies whether to create PodMonitor for Prometheus
	PodMonitor bool `json:"podMonitor,omitempty"`

	// ServiceMonitor specifies whether to create ServiceMonitor for Prometheus
	ServiceMonitor bool `json:"serviceMonitor,omitempty"`

	// Exporter specifies Redis exporter configuration
	Exporter RedisExporter `json:"exporter,omitempty"`

	// PrometheusRule specifies whether to create PrometheusRule
	PrometheusRule bool `json:"prometheusRule,omitempty"`

	// Grafana specifies Grafana dashboard configuration
	Grafana *RedisGrafana `json:"grafana,omitempty"`
}

// RedisExporter defines Redis exporter configuration
type RedisExporter struct {
	// Enabled specifies whether exporter is enabled
	Enabled bool `json:"enabled"`

	// Image specifies the exporter image
	// +kubebuilder:default="oliver006/redis_exporter:latest"
	Image string `json:"image,omitempty"`

	// Port specifies the exporter port
	// +kubebuilder:default=9121
	Port int32 `json:"port,omitempty"`

	// Resources specifies resource requirements for the exporter
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Args specifies additional arguments for the exporter
	Args []string `json:"args,omitempty"`

	// Env specifies environment variables for the exporter
	Env []corev1.EnvVar `json:"env,omitempty"`
}

// RedisGrafana defines Grafana configuration
type RedisGrafana struct {
	// Enabled specifies whether Grafana dashboard is enabled
	Enabled bool `json:"enabled"`

	// Dashboard specifies dashboard configuration
	Dashboard *RedisGrafanaDashboard `json:"dashboard,omitempty"`
}

// RedisGrafanaDashboard defines Grafana dashboard configuration
type RedisGrafanaDashboard struct {
	// ConfigMapRef specifies the configmap containing dashboard JSON
	ConfigMapRef *corev1.ConfigMapKeySelector `json:"configMapRef,omitempty"`

	// Labels specifies labels for dashboard discovery
	Labels map[string]string `json:"labels,omitempty"`
}

// RedisBackup defines backup configuration
type RedisBackup struct {
	// Enabled specifies whether backup is enabled
	Enabled bool `json:"enabled"`

	// Image specifies the backup container image
	Image string `json:"image,omitempty"`

	// Annotations specifies additional annotations for backup pods
	Annotations map[string]string `json:"annotations,omitempty"`

	// BackUpInitConfig specifies backup initialization configuration
	BackUpInitConfig *RedisBackupInitConfig `json:"backupInitConfig,omitempty"`

	// PodTemplate specifies pod template for backup jobs
	PodTemplate *corev1.PodTemplateSpec `json:"podTemplate,omitempty"`

	// Schedule specifies the backup schedule (cron format)
	Schedule string `json:"schedule,omitempty"`

	// Retention specifies backup retention policy
	Retention *RedisBackupRetention `json:"retention,omitempty"`

	// Storage specifies backup storage configuration
	Storage *RedisBackupStorage `json:"storage,omitempty"`

	// Suspend specifies whether backup is suspended
	Suspend bool `json:"suspend,omitempty"`
}

// RedisBackupInitConfig defines backup initialization configuration
type RedisBackupInitConfig struct {
	// Enabled specifies whether backup initialization is enabled
	Enabled bool `json:"enabled"`

	// Image specifies the backup init image
	Image string `json:"image,omitempty"`

	// Resources specifies resource requirements for backup init
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// RedisBackupRetention defines backup retention policy
type RedisBackupRetention struct {
	// Days specifies retention in days
	Days int32 `json:"days,omitempty"`

	// Count specifies maximum number of backups to retain
	Count int32 `json:"count,omitempty"`
}

// RedisBackupStorage defines backup storage configuration
type RedisBackupStorage struct {
	// Type specifies the storage type (S3, GCS, Azure, etc.)
	Type string `json:"type"`

	// S3 specifies S3 storage configuration
	S3 *RedisBackupS3Storage `json:"s3,omitempty"`

	// GCS specifies Google Cloud Storage configuration
	GCS *RedisBackupGCSStorage `json:"gcs,omitempty"`

	// PVC specifies PVC storage configuration
	PVC *RedisBackupPVCStorage `json:"pvc,omitempty"`
}

// RedisBackupS3Storage defines S3 backup storage configuration
type RedisBackupS3Storage struct {
	// Bucket specifies the S3 bucket name
	Bucket string `json:"bucket"`

	// Region specifies the S3 region
	Region string `json:"region,omitempty"`

	// Prefix specifies the S3 key prefix
	Prefix string `json:"prefix,omitempty"`

	// CredentialsSecret specifies the secret containing S3 credentials
	CredentialsSecret *corev1.SecretKeySelector `json:"credentialsSecret,omitempty"`

	// SecretName specifies the secret name containing S3 credentials (legacy field)
	SecretName string `json:"secretName,omitempty"`
}

// RedisBackupGCSStorage defines Google Cloud Storage backup configuration
type RedisBackupGCSStorage struct {
	// Bucket specifies the GCS bucket name
	Bucket string `json:"bucket"`

	// Prefix specifies the GCS key prefix
	Prefix string `json:"prefix,omitempty"`

	// CredentialsSecret specifies the secret containing GCS credentials
	CredentialsSecret *corev1.SecretKeySelector `json:"credentialsSecret,omitempty"`
}

// RedisBackupPVCStorage defines PVC backup storage configuration
type RedisBackupPVCStorage struct {
	// StorageClassName specifies the storage class for backup PVC
	StorageClassName string `json:"storageClassName,omitempty"`

	// Size specifies the size of backup PVC
	Size resource.Quantity `json:"size"`

	// AccessModes specifies access modes for backup PVC
	AccessModes []corev1.PersistentVolumeAccessMode `json:"accessModes,omitempty"`
}

// RedisPodDisruptionBudget defines pod disruption budget configuration
type RedisPodDisruptionBudget struct {
	// Enabled specifies whether PDB is enabled
	Enabled bool `json:"enabled"`

	// MinAvailable specifies the minimum number of available pods
	MinAvailable *intstr.IntOrString `json:"minAvailable,omitempty"`

	// MaxUnavailable specifies the maximum number of unavailable pods
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`
}

// RedisProbes defines health probe configuration
type RedisProbes struct {
	// LivenessProbe specifies liveness probe configuration
	LivenessProbe *RedisProbeConfig `json:"livenessProbe,omitempty"`

	// ReadinessProbe specifies readiness probe configuration
	ReadinessProbe *RedisProbeConfig `json:"readinessProbe,omitempty"`

	// StartupProbe specifies startup probe configuration
	StartupProbe *RedisProbeConfig `json:"startupProbe,omitempty"`
}

// RedisProbeConfig defines individual probe configuration
type RedisProbeConfig struct {
	// Enabled specifies whether the probe is enabled
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty"`

	// InitialDelaySeconds specifies the initial delay before probes start
	// +kubebuilder:default=30
	InitialDelaySeconds *int32 `json:"initialDelaySeconds,omitempty"`

	// PeriodSeconds specifies how often the probe runs
	// +kubebuilder:default=10
	PeriodSeconds *int32 `json:"periodSeconds,omitempty"`

	// TimeoutSeconds specifies the probe timeout
	// +kubebuilder:default=5
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`

	// SuccessThreshold specifies the success threshold
	// +kubebuilder:default=1
	SuccessThreshold *int32 `json:"successThreshold,omitempty"`

	// FailureThreshold specifies the failure threshold
	// +kubebuilder:default=3
	FailureThreshold *int32 `json:"failureThreshold,omitempty"`
}

// RedisStatus defines the observed state of Redis.
type RedisStatus struct {
	// Phase represents the current phase of the Redis instance
	Phase RedisPhase `json:"phase,omitempty"`

	// Conditions represents the latest available observations of the Redis state
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Ready indicates whether the Redis instance is ready
	Ready bool `json:"ready"`

	// Endpoint specifies the Redis connection endpoint
	Endpoint string `json:"endpoint,omitempty"`

	// Port specifies the Redis port
	Port int32 `json:"port,omitempty"`

	// Version specifies the actual Redis version running
	Version string `json:"version,omitempty"`

	// LastBackup specifies the timestamp of the last successful backup
	LastBackup *metav1.Time `json:"lastBackup,omitempty"`

	// ObservedGeneration represents the generation observed by the controller
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// RedisPhase represents the phase of a Redis instance
type RedisPhase string

const (
	// RedisPhasePending indicates the Redis instance is being created
	RedisPhasePending RedisPhase = "Pending"

	// RedisPhaseRunning indicates the Redis instance is running
	RedisPhaseRunning RedisPhase = "Running"

	// RedisPhaseFailed indicates the Redis instance has failed
	RedisPhaseFailed RedisPhase = "Failed"

	// RedisPhaseUpgrading indicates the Redis instance is being upgraded
	RedisPhaseUpgrading RedisPhase = "Upgrading"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Redis is the Schema for the redis API.
type Redis struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RedisSpec   `json:"spec,omitempty"`
	Status RedisStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RedisList contains a list of Redis.
type RedisList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Redis `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Redis{}, &RedisList{})
}
