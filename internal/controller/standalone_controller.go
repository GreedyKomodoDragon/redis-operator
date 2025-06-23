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

package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	koncachev1alpha1 "github.com/GreedyKomodoDragon/redis-operator/api/v1alpha1"
)

// StandaloneController handles the reconciliation of standalone Redis instances
type StandaloneController struct {
	client.Client
	Scheme *runtime.Scheme
}

// NewStandaloneController creates a new StandaloneController
func NewStandaloneController(client client.Client, scheme *runtime.Scheme) *StandaloneController {
	return &StandaloneController{
		Client: client,
		Scheme: scheme,
	}
}

// Reconcile handles the reconciliation of a standalone Redis instance
func (s *StandaloneController) Reconcile(ctx context.Context, redis *koncachev1alpha1.Redis) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Create or update ConfigMap
	if err := s.reconcileConfigMap(ctx, redis); err != nil {
		log.Error(err, "Failed to reconcile ConfigMap")
		return ctrl.Result{}, err
	}

	// Create or update Service
	if err := s.reconcileService(ctx, redis); err != nil {
		log.Error(err, "Failed to reconcile Service")
		return ctrl.Result{}, err
	}

	// Create or update StatefulSet
	statefulSet, requeue, err := s.reconcileStatefulSet(ctx, redis)
	if err != nil {
		log.Error(err, "Failed to reconcile StatefulSet")
		return ctrl.Result{}, err
	}
	if requeue {
		return ctrl.Result{Requeue: true}, nil
	}

	// Update the Redis status
	if err := s.updateRedisStatus(ctx, redis, statefulSet); err != nil {
		log.Error(err, "Failed to update Redis status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// reconcileConfigMap creates or updates the ConfigMap for Redis
func (s *StandaloneController) reconcileConfigMap(ctx context.Context, redis *koncachev1alpha1.Redis) error {
	log := logf.FromContext(ctx)

	configMap := s.configMapForRedis(redis)
	if err := controllerutil.SetControllerReference(redis, configMap, s.Scheme); err != nil {
		return err
	}

	foundConfigMap := &corev1.ConfigMap{}
	err := s.Get(ctx, types.NamespacedName{Name: configMap.Name, Namespace: configMap.Namespace}, foundConfigMap)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating a new ConfigMap", "ConfigMap.Namespace", configMap.Namespace, "ConfigMap.Name", configMap.Name)
		return s.Create(ctx, configMap)
	}
	return err
}

// reconcileService creates or updates the Service for Redis
func (s *StandaloneController) reconcileService(ctx context.Context, redis *koncachev1alpha1.Redis) error {
	log := logf.FromContext(ctx)

	service := s.serviceForRedis(redis)
	if err := controllerutil.SetControllerReference(redis, service, s.Scheme); err != nil {
		return err
	}

	foundService := &corev1.Service{}
	err := s.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, foundService)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating a new Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
		return s.Create(ctx, service)
	}
	return err
}

// reconcileStatefulSet creates or updates the StatefulSet for Redis
func (s *StandaloneController) reconcileStatefulSet(ctx context.Context, redis *koncachev1alpha1.Redis) (*appsv1.StatefulSet, bool, error) {
	log := logf.FromContext(ctx)

	statefulSet := s.statefulSetForRedis(redis)
	if err := controllerutil.SetControllerReference(redis, statefulSet, s.Scheme); err != nil {
		return nil, false, err
	}

	foundStatefulSet := &appsv1.StatefulSet{}
	err := s.Get(ctx, types.NamespacedName{Name: statefulSet.Name, Namespace: statefulSet.Namespace}, foundStatefulSet)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating a new StatefulSet", "StatefulSet.Namespace", statefulSet.Namespace, "StatefulSet.Name", statefulSet.Name)
		err = s.Create(ctx, statefulSet)
		if err != nil {
			return nil, false, err
		}
		// StatefulSet created successfully - return and requeue
		return foundStatefulSet, true, nil
	} else if err != nil {
		return nil, false, err
	}

	return foundStatefulSet, false, nil
}

// configMapForRedis returns a ConfigMap object for the Redis configuration
func (s *StandaloneController) configMapForRedis(redis *koncachev1alpha1.Redis) *corev1.ConfigMap {
	labels := labelsForRedis(redis.Name)
	configData := s.buildRedisConfig(redis)

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redis.Name + "-config",
			Namespace: redis.Namespace,
			Labels:    labels,
		},
		Data: map[string]string{
			"redis.conf": configData,
		},
	}
}

// serviceForRedis returns a Service object for Redis
func (s *StandaloneController) serviceForRedis(redis *koncachev1alpha1.Redis) *corev1.Service {
	labels := labelsForRedis(redis.Name)
	port := redis.Spec.Port
	if port == 0 {
		port = 6379 // Default Redis port
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        redis.Name,
			Namespace:   redis.Namespace,
			Labels:      labels,
			Annotations: redis.Spec.ServiceAnnotations,
		},
		Spec: corev1.ServiceSpec{
			Type:     redis.Spec.ServiceType,
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "redis",
					Port:       port,
					TargetPort: intstr.FromInt(int(port)),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
}

// statefulSetForRedis returns a StatefulSet object for Redis
func (s *StandaloneController) statefulSetForRedis(redis *koncachev1alpha1.Redis) *appsv1.StatefulSet {
	labels := labelsForRedis(redis.Name)
	replicas := int32(1) // Standalone Redis always has 1 replica
	port := redis.Spec.Port
	if port == 0 {
		port = 6379
	}

	// Build container
	container := s.buildRedisContainer(redis, port)

	// Build pod template
	podTemplate := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      labels,
			Annotations: redis.Spec.PodAnnotations,
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: redis.Spec.ServiceAccount,
			NodeSelector:       redis.Spec.NodeSelector,
			Tolerations:        redis.Spec.Tolerations,
			Affinity:           redis.Spec.Affinity,
			ImagePullSecrets:   redis.Spec.ImagePullSecrets,
			Containers:         []corev1.Container{container},
			Volumes: []corev1.Volume{
				{
					Name: "redis-config",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: redis.Name + "-config",
							},
						},
					},
				},
			},
		},
	}

	// Add custom labels if specified
	for k, v := range redis.Spec.PodLabels {
		podTemplate.ObjectMeta.Labels[k] = v
	}

	// Build volume claim template
	volumeClaimTemplate := s.buildVolumeClaimTemplate(redis)

	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redis.Name,
			Namespace: redis.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:             &replicas,
			ServiceName:          redis.Name,
			Selector:             &metav1.LabelSelector{MatchLabels: labels},
			Template:             podTemplate,
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{volumeClaimTemplate},
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
			},
		},
	}
}

// buildRedisContainer builds the Redis container specification
func (s *StandaloneController) buildRedisContainer(redis *koncachev1alpha1.Redis, port int32) corev1.Container {
	container := corev1.Container{
		Name:            "redis",
		Image:           redis.Spec.Image,
		ImagePullPolicy: redis.Spec.ImagePullPolicy,
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: port,
				Name:          "redis",
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Resources: redis.Spec.Resources,
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "redis-data",
				MountPath: "/data",
			},
			{
				Name:      "redis-config",
				MountPath: "/usr/local/etc/redis/redis.conf",
				SubPath:   "redis.conf",
				ReadOnly:  true,
			},
		},
		Command: []string{"redis-server"},
		Args:    []string{"/usr/local/etc/redis/redis.conf"},
	}

	// Add liveness and readiness probes
	container.LivenessProbe = &corev1.Probe{
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

	container.ReadinessProbe = &corev1.Probe{
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

	return container
}

// buildVolumeClaimTemplate builds the volume claim template for the StatefulSet
func (s *StandaloneController) buildVolumeClaimTemplate(redis *koncachev1alpha1.Redis) corev1.PersistentVolumeClaim {
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
			Name: "redis-data",
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

// buildRedisConfig builds the Redis configuration string
func (s *StandaloneController) buildRedisConfig(redis *koncachev1alpha1.Redis) string {
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

	// Security
	if redis.Spec.Security.RequireAuth != nil && *redis.Spec.Security.RequireAuth {
		config += "protected-mode yes\n"
		// Note: Password will be set via environment variable or secret
	}

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

// updateRedisStatus updates the status of the Redis instance
func (s *StandaloneController) updateRedisStatus(ctx context.Context, redis *koncachev1alpha1.Redis, statefulSet *appsv1.StatefulSet) error {
	// Update status based on StatefulSet status
	redis.Status.ObservedGeneration = redis.Generation

	if statefulSet.Status.ReadyReplicas == *statefulSet.Spec.Replicas {
		redis.Status.Phase = koncachev1alpha1.RedisPhaseRunning
		redis.Status.Ready = true
	} else {
		redis.Status.Phase = koncachev1alpha1.RedisPhasePending
		redis.Status.Ready = false
	}

	// Set endpoint
	port := redis.Spec.Port
	if port == 0 {
		port = 6379
	}
	redis.Status.Endpoint = fmt.Sprintf("%s.%s.svc.cluster.local", redis.Name, redis.Namespace)
	redis.Status.Port = port
	redis.Status.Version = redis.Spec.Version

	return s.Status().Update(ctx, redis)
}
