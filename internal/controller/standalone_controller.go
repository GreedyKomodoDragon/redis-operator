package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
		// StatefulSet created successfully - return the created statefulSet and requeue
		return statefulSet, true, nil
	} else if err != nil {
		return nil, false, err
	}

	if !s.needsStatefulSetUpdate(redis, foundStatefulSet, statefulSet) {
		log.V(1).Info("StatefulSet is up-to-date, no changes needed", "StatefulSet.Name", foundStatefulSet.Name)
		// No changes needed, return the existing StatefulSet and do not requeue
		return foundStatefulSet, false, nil
	}

	log.Info("StatefulSet spec has changed, updating", "StatefulSet.Name", foundStatefulSet.Name)

	// Update the existing StatefulSet with the new spec
	foundStatefulSet.Spec = statefulSet.Spec
	foundStatefulSet.Labels = statefulSet.Labels
	foundStatefulSet.Annotations = statefulSet.Annotations

	if err := s.Update(ctx, foundStatefulSet); err != nil {
		return nil, false, err
	}
	// Return true to requeue and wait for the update to be processed
	return foundStatefulSet, true, nil

}

// configMapForRedis returns a ConfigMap object for the Redis configuration
func (s *StandaloneController) configMapForRedis(redis *koncachev1alpha1.Redis) *corev1.ConfigMap {
	labels := LabelsForRedis(redis.Name)
	configData := BuildRedisConfig(redis)

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
	labels := LabelsForRedis(redis.Name)
	port := GetRedisPort(redis)

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
	labels := LabelsForRedis(redis.Name)
	replicas := int32(1) // Standalone Redis always has 1 replica
	port := GetRedisPort(redis)

	// Build container
	container := BuildRedisContainer(redis, port)

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
			Volumes:            []corev1.Volume{BuildConfigMapVolume(redis.Name)},
		},
	}

	// Add custom labels if specified
	for k, v := range redis.Spec.PodLabels {
		podTemplate.ObjectMeta.Labels[k] = v
	}

	// Build volume claim template
	volumeClaimTemplate := BuildVolumeClaimTemplate(redis)

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

// updateRedisStatus updates the status of the Redis instance
func (s *StandaloneController) updateRedisStatus(ctx context.Context, redis *koncachev1alpha1.Redis, statefulSet *appsv1.StatefulSet) error {
	// Create a retry mechanism to handle conflicts
	return s.updateStatusWithRetry(ctx, redis, func(currentRedis *koncachev1alpha1.Redis) {
		// Update status based on StatefulSet status
		currentRedis.Status.ObservedGeneration = currentRedis.Generation

		if statefulSet.Status.ReadyReplicas == *statefulSet.Spec.Replicas {
			currentRedis.Status.Phase = koncachev1alpha1.RedisPhaseRunning
			currentRedis.Status.Ready = true
		} else {
			currentRedis.Status.Phase = koncachev1alpha1.RedisPhasePending
			currentRedis.Status.Ready = false
		}

		// Set endpoint
		port := GetRedisPort(currentRedis)
		currentRedis.Status.Endpoint = fmt.Sprintf("%s.%s.svc.cluster.local", currentRedis.Name, currentRedis.Namespace)
		currentRedis.Status.Port = port
		currentRedis.Status.Version = currentRedis.Spec.Version
	})
}

// updateStatusWithRetry handles the status update with retry mechanism to avoid conflicts
func (s *StandaloneController) updateStatusWithRetry(ctx context.Context, redis *koncachev1alpha1.Redis, updateFunc func(*koncachev1alpha1.Redis)) error {
	const maxRetries = 5

	for i := 0; i < maxRetries; i++ {
		// Fetch the latest version of the Redis object
		currentRedis := &koncachev1alpha1.Redis{}
		if err := s.Get(ctx, types.NamespacedName{Name: redis.Name, Namespace: redis.Namespace}, currentRedis); err != nil {
			return err
		}

		// Store the original status for comparison
		originalStatus := currentRedis.Status.DeepCopy()

		// Apply the update function to the current Redis object
		updateFunc(currentRedis)

		// Check if the status actually changed to avoid unnecessary updates
		if statusEqual(originalStatus, &currentRedis.Status) {
			logf.FromContext(ctx).V(1).Info("Redis status unchanged, skipping update")
			return nil
		}

		// Try to update the status
		if err := s.Status().Update(ctx, currentRedis); err != nil {
			if errors.IsConflict(err) && i < maxRetries-1 {
				// If it's a conflict error and we haven't exceeded max retries, continue
				logf.FromContext(ctx).Info("Conflict updating Redis status, retrying", "attempt", i+1)
				continue
			}
			return err
		}

		// Success - break out of retry loop
		return nil
	}

	return fmt.Errorf("failed to update Redis status after %d retries", maxRetries)
}

// statusEqual compares two RedisStatus objects for equality
func statusEqual(a, b *koncachev1alpha1.RedisStatus) bool {
	return a.Phase == b.Phase &&
		a.Ready == b.Ready &&
		a.Endpoint == b.Endpoint &&
		a.Port == b.Port &&
		a.Version == b.Version &&
		a.ObservedGeneration == b.ObservedGeneration
}

// needsStatefulSetUpdate checks if the StatefulSet needs to be updated based on the Redis spec
func (s *StandaloneController) needsStatefulSetUpdate(redis *koncachev1alpha1.Redis, existing, desired *appsv1.StatefulSet) bool {
	return s.hasBasicSpecChanges(existing, desired) || // Check basic spec differences
		s.hasContainerChanges(existing, desired) || // Check container differences
		s.hasMetadataChanges(existing, desired) // Check metadata differences
}

// hasBasicSpecChanges checks for basic StatefulSet spec changes
func (s *StandaloneController) hasBasicSpecChanges(existing, desired *appsv1.StatefulSet) bool {
	return *existing.Spec.Replicas != *desired.Spec.Replicas ||
		existing.Spec.Template.Spec.ServiceAccountName != desired.Spec.Template.Spec.ServiceAccountName
}

// hasContainerChanges checks for container-related changes
func (s *StandaloneController) hasContainerChanges(existing, desired *appsv1.StatefulSet) bool {
	if len(existing.Spec.Template.Spec.Containers) == 0 || len(desired.Spec.Template.Spec.Containers) == 0 {
		return len(existing.Spec.Template.Spec.Containers) != len(desired.Spec.Template.Spec.Containers)
	}

	existingContainer := &existing.Spec.Template.Spec.Containers[0]
	desiredContainer := &desired.Spec.Template.Spec.Containers[0]

	return existingContainer.Image != desiredContainer.Image || // Check image
		s.hasResourceChanges(existingContainer, desiredContainer) || // Check resources
		s.hasPortChanges(existingContainer, desiredContainer) // Check ports
}

// hasResourceChanges checks if container resources have changed
func (s *StandaloneController) hasResourceChanges(existing, desired *corev1.Container) bool {
	return !EqualResourceLists(existing.Resources.Requests, desired.Resources.Requests) ||
		!EqualResourceLists(existing.Resources.Limits, desired.Resources.Limits)
}

// hasPortChanges checks if container ports have changed
func (s *StandaloneController) hasPortChanges(existing, desired *corev1.Container) bool {
	if len(existing.Ports) != len(desired.Ports) {
		return true
	}

	for i, existingPort := range existing.Ports {
		if i < len(desired.Ports) && existingPort.ContainerPort != desired.Ports[i].ContainerPort {
			return true
		}
	}

	return false
}

// hasMetadataChanges checks for metadata changes
func (s *StandaloneController) hasMetadataChanges(existing, desired *appsv1.StatefulSet) bool {
	return !EqualStringMaps(existing.Spec.Template.Spec.NodeSelector, desired.Spec.Template.Spec.NodeSelector) ||
		!EqualStringMaps(existing.Spec.Template.Annotations, desired.Spec.Template.Annotations) ||
		!EqualStringMaps(existing.Spec.Template.Labels, desired.Spec.Template.Labels) ||
		!EqualStringMaps(existing.Labels, desired.Labels) ||
		!EqualStringMaps(existing.Annotations, desired.Annotations) ||
		!EqualTolerations(existing.Spec.Template.Spec.Tolerations, desired.Spec.Template.Spec.Tolerations)
}
