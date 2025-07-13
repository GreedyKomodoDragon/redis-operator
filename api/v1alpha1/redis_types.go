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
	Backup RedisBackup `json:"backup,omitempty"`

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

// RedisPodDisruptionBudget defines pod disruption budget configuration
type RedisPodDisruptionBudget struct {
	// Enabled specifies whether PDB is enabled
	Enabled bool `json:"enabled"`

	// MinAvailable specifies the minimum number of available pods
	MinAvailable *int32 `json:"minAvailable,omitempty"`

	// MaxUnavailable specifies the maximum number of unavailable pods
	MaxUnavailable *int32 `json:"maxUnavailable,omitempty"`
}

// RedisProbes defines health probe configuration
type RedisProbes struct {
	// LivenessProbe specifies liveness probe configuration
	LivenessProbe *RedisProbeConfig `json:"livenessProbe,omitempty"`

	// ReadinessProbe specifies readiness probe configuration
	ReadinessProbe *RedisProbeConfig `json:"readinessProbe,omitempty"`
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

	// FailureThreshold specifies the failure threshold
	// +kubebuilder:default=3
	FailureThreshold *int32 `json:"failureThreshold,omitempty"`

	// SuccessThreshold specifies the success threshold
	// +kubebuilder:default=1
	SuccessThreshold *int32 `json:"successThreshold,omitempty"`
}

// RedisSecurity defines security configuration
type RedisSecurity struct {
	// PasswordSecret specifies the secret containing the Redis password
	PasswordSecret *corev1.SecretKeySelector `json:"passwordSecret,omitempty"`

	// TLS specifies TLS configuration
	TLS *RedisTLS `json:"tls,omitempty"`

	// RequireAuth enables authentication
	RequireAuth *bool `json:"requireAuth,omitempty"`

	// RenameCommands specifies commands to rename for security
	RenameCommands map[string]string `json:"renameCommands,omitempty"`
}

// RedisTLS defines TLS configuration
type RedisTLS struct {
	// Enabled specifies whether TLS is enabled
	Enabled bool `json:"enabled"`

	// CertSecret specifies the secret containing TLS certificates
	CertSecret string `json:"certSecret,omitempty"`

	// CASecret specifies the secret containing CA certificate
	CASecret string `json:"caSecret,omitempty"`

	// CertFile specifies the path to the certificate file
	CertFile string `json:"certFile,omitempty"`

	// KeyFile specifies the path to the private key file
	KeyFile string `json:"keyFile,omitempty"`

	// CAFile specifies the path to the CA certificate file
	CAFile string `json:"caFile,omitempty"`
}

// RedisMonitoring defines monitoring configuration
type RedisMonitoring struct {
	// Enabled specifies whether monitoring is enabled
	Enabled bool `json:"enabled"`

	// ServiceMonitor specifies whether to create a ServiceMonitor
	ServiceMonitor bool `json:"serviceMonitor,omitempty"`

	// PrometheusRule specifies whether to create PrometheusRule
	PrometheusRule bool `json:"prometheusRule,omitempty"`

	// Exporter specifies Redis exporter configuration
	Exporter RedisExporter `json:"exporter,omitempty"`
}

// RedisExporter defines Redis exporter configuration
type RedisExporter struct {
	// Enabled specifies whether the exporter is enabled
	Enabled bool `json:"enabled"`

	// Image specifies the exporter image
	// +kubebuilder:default="oliver006/redis_exporter:latest"
	Image string `json:"image,omitempty"`

	// Port specifies the exporter port
	// +kubebuilder:default=9121
	Port int32 `json:"port,omitempty"`

	// Resources specifies resource requirements for the exporter
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// RedisBackup defines backup configuration
type RedisBackup struct {
	// Enabled specifies whether backup is enabled
	Enabled bool `json:"enabled"`

	// Image specifies the backup container image
	// +kubebuilder:default="koncache/redis-backup:latest"
	Image string `json:"image,omitempty"`

	// Schedule specifies the backup schedule in cron format
	Schedule string `json:"schedule,omitempty"`

	// Retention specifies how many backups to retain
	// +kubebuilder:default=7
	Retention int32 `json:"retention,omitempty"`

	// Storage specifies backup storage configuration
	Storage RedisBackupStorage `json:"storage"`

	// BackupInit specifies whether to run an init container for backup
	BackUpInitConfig BackupInitConfig `json:"backupInitConfig"`

	// PodTemplate specifies the pod template for backup jobs
	PodTemplate PodSpec `json:"podTemplate,omitempty"`

	// Annotations specifies additional annotations for the backup pod
	Annotations map[string]string `json:"annotations,omitempty"`
}

type PodSpec struct {
	NodeSelector     map[string]string             `json:"nodeSelector,omitempty" protobuf:"bytes,7,rep,name=nodeSelector"`
	SecurityContext  *corev1.PodSecurityContext    `json:"securityContext,omitempty" protobuf:"bytes,14,opt,name=securityContext"`
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty" patchStrategy:"merge" patchMergeKey:"name" protobuf:"bytes,15,rep,name=imagePullSecrets"`
	Affinity         *corev1.Affinity              `json:"affinity,omitempty" protobuf:"bytes,18,opt,name=affinity"`
	Tolerations      []corev1.Toleration           `json:"tolerations,omitempty" protobuf:"bytes,22,opt,name=tolerations"`
	Resources        *corev1.ResourceRequirements  `json:"resources,omitempty" protobuf:"bytes,40,opt,name=resources"`
}

// RedisBackupStorage defines backup storage configuration
type RedisBackupStorage struct {
	// Type specifies the backup storage type (s3)
	Type string `json:"type"`

	// S3 specifies S3 storage configuration
	S3 *RedisS3Storage `json:"s3,omitempty"`
}

type BackupInitConfig struct {
	// Enabled specifies whether to run an init container for backup
	Enabled bool `json:"enabled,omitempty"`

	// Image specifies the init container image for backup
	Image string `json:"image,omitempty"`
}

// RedisS3Storage defines S3 storage configuration
type RedisS3Storage struct {
	// Bucket specifies the S3 bucket name
	Bucket string `json:"bucket"`

	// Region specifies the S3 region
	Region string `json:"region,omitempty"`

	// Prefix specifies the S3 key prefix
	Prefix string `json:"prefix,omitempty"`

	// SecretName specifies the secret containing S3 credentials
	SecretName string `json:"secretName"`
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
