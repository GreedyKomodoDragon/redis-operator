package controller

import (
	"context"
	"crypto/tls"
	"fmt"
	"maps"
	"net"
	"strings"
	"time"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	redisClient "github.com/redis/go-redis/v9"
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

const (
	statefulSetNameField         = "StatefulSet.Name"
	statefulSetNamespaceField    = "StatefulSet.Namespace"
	configMapNameField           = "ConfigMap.Name"
	configMapNamespaceField      = "ConfigMap.Namespace"
	serviceMonitorNameField      = "ServiceMonitor.Name"
	serviceMonitorNamespaceField = "ServiceMonitor.Namespace"
	configHashField              = "redis-operator/config-hash"
	leaderAnnotationField        = "redis-operator/role"
)

// StandaloneController handles the reconciliation of standalone Redis instances
type StandaloneController struct {
	client.Client
	Scheme          *runtime.Scheme
	CommandExecutor RedisCommandExecutor
}

// NewStandaloneController creates a new StandaloneController
func NewStandaloneController(client client.Client, scheme *runtime.Scheme) *StandaloneController {
	controller := &StandaloneController{
		Client: client,
		Scheme: scheme,
	}
	controller.CommandExecutor = &RealRedisCommandExecutor{controller: controller}
	return controller
}

// Reconcile handles the reconciliation of a standalone Redis instance
func (s *StandaloneController) Reconcile(ctx context.Context, redis *koncachev1alpha1.Redis) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Generate Redis configuration once for reuse
	redisConfig := BuildRedisConfig(redis)

	// Create or update ConfigMap
	if err := s.reconcileConfigMap(ctx, redis, redisConfig); err != nil {
		log.Error(err, "Failed to reconcile ConfigMap")
		return ctrl.Result{}, err
	}

	// Create or update Service
	if err := s.reconcileService(ctx, redis); err != nil {
		log.Error(err, "Failed to reconcile Service")
		return ctrl.Result{}, err
	}

	// Create or update ServiceMonitor if monitoring is enabled
	if IsMonitoringEnabled(redis) {
		if err := s.reconcileServiceMonitor(ctx, redis); err != nil {
			log.Error(err, "Failed to reconcile ServiceMonitor")
			return ctrl.Result{}, err
		}
	} else {
		// Clean up ServiceMonitor if it exists but monitoring is disabled
		if err := s.cleanupServiceMonitor(ctx, redis); err != nil {
			log.Error(err, "Failed to cleanup ServiceMonitor")
			return ctrl.Result{}, err
		}
	}

	// Create or update backup statefulset if backups are enabled
	if redis.Spec.Backup.Enabled {
		if err := s.reconcileBackupStatefulSet(ctx, redis); err != nil {
			log.Error(err, "Failed to reconcile backup StatefulSet")
			return ctrl.Result{}, err
		}
	} else {
		// Clean up backup StatefulSet if it exists but backups are disabled
		// if err := s.cleanupBackupStatefulSet(ctx, redis); err != nil {
		// 	log.Error(err, "Failed to cleanup backup StatefulSet")
		// 	return ctrl.Result{}, err
		// }
	}

	// Create or update StatefulSet
	configHash := ComputeStringHash(redisConfig)
	log.V(1).Info("Generated Redis config", "hash", configHash, "config_length", len(redisConfig))
	statefulSets, requeue, err := s.reconcileStatefulSets(ctx, redis, redisConfig, configHash)
	if err != nil {
		log.Error(err, "Failed to reconcile StatefulSet")
		return ctrl.Result{}, err
	}

	// Clean up orphaned StatefulSets (e.g., when HA replica count is reduced)
	if err := s.cleanupOrphanedStatefulSets(ctx, redis, statefulSets); err != nil {
		log.Error(err, "Failed to cleanup orphaned StatefulSets")
		// Don't fail the reconciliation, just log the error
	}

	// Still try to assign a leader if HA is enabled and StatefulSets exist
	if redis.Spec.HighAvailability != nil && redis.Spec.HighAvailability.Enabled {
		if err := s.ensureLeaderExists(ctx, redis); err != nil {
			log.Error(err, "Failed to ensure leader exists during requeue")
			// Don't fail the reconciliation for leader assignment issues
		}

		// Configure Redis replication for HA
		if err := s.ensureReplicationConfiguration(ctx, redis); err != nil {
			log.Error(err, "Failed to configure Redis replication")
			// Don't fail the reconciliation for replication config issues
		}
	}

	if requeue {
		return ctrl.Result{Requeue: true}, nil
	}

	// Update the Redis status
	if err := s.updateRedisStatus(ctx, redis, statefulSets); err != nil {
		log.Error(err, "Failed to update Redis status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (s *StandaloneController) reconcileServiceMonitor(ctx context.Context, redis *koncachev1alpha1.Redis) error {
	log := logf.FromContext(ctx)

	serviceMonitor := s.serviceMonitorForRedis(redis)
	if err := controllerutil.SetControllerReference(redis, serviceMonitor, s.Scheme); err != nil {
		return err
	}

	foundServiceMonitor := &monitoringv1.ServiceMonitor{}
	err := s.Get(ctx, types.NamespacedName{Name: serviceMonitor.Name, Namespace: serviceMonitor.Namespace}, foundServiceMonitor)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating a new ServiceMonitor", serviceMonitorNamespaceField, serviceMonitor.Namespace, serviceMonitorNameField, serviceMonitor.Name)
		return s.Create(ctx, serviceMonitor)
	}

	return err
}

func (s *StandaloneController) cleanupServiceMonitor(ctx context.Context, redis *koncachev1alpha1.Redis) error {
	log := logf.FromContext(ctx)
	serviceMonitor := &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redis.Name + "-monitor",
			Namespace: redis.Namespace,
		},
	}
	err := s.Get(ctx, types.NamespacedName{Name: serviceMonitor.Name, Namespace: serviceMonitor.Namespace}, serviceMonitor)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("ServiceMonitor not found, nothing to clean up", serviceMonitorNamespaceField, serviceMonitor.Namespace, serviceMonitorNameField, serviceMonitor.Name)
			return nil
		}
		log.Error(err, "Failed to get ServiceMonitor for cleanup", serviceMonitorNamespaceField, serviceMonitor.Namespace, serviceMonitorNameField, serviceMonitor.Name)
		return err
	}

	log.Info("Deleting ServiceMonitor", serviceMonitorNamespaceField, serviceMonitor.Namespace, serviceMonitorNameField, serviceMonitor.Name)
	return s.Delete(ctx, serviceMonitor)
}

func (s *StandaloneController) serviceMonitorForRedis(redis *koncachev1alpha1.Redis) *monitoringv1.ServiceMonitor {
	labels := LabelsForRedis(redis.Name)
	return &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redis.Name + "-monitor",
			Namespace: redis.Namespace,
			Labels:    labels,
		},
		Spec: monitoringv1.ServiceMonitorSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: labels,
			},
			Endpoints: []monitoringv1.Endpoint{
				{
					Port:     "metrics",
					Interval: "30s",
				},
			},
			NamespaceSelector: monitoringv1.NamespaceSelector{
				MatchNames: []string{redis.Namespace},
			},
		},
	}
}

// reconcileConfigMap creates or updates the ConfigMap for Redis
func (s *StandaloneController) reconcileConfigMap(ctx context.Context, redis *koncachev1alpha1.Redis, redisConfig string) error {
	log := logf.FromContext(ctx)

	configMap := s.configMapForRedis(redis, redisConfig)
	if err := controllerutil.SetControllerReference(redis, configMap, s.Scheme); err != nil {
		return err
	}

	foundConfigMap := &corev1.ConfigMap{}
	err := s.Get(ctx, types.NamespacedName{Name: configMap.Name, Namespace: configMap.Namespace}, foundConfigMap)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating a new ConfigMap", configMapNamespaceField, configMap.Namespace, configMapNameField, configMap.Name)
		return s.Create(ctx, configMap)
	} else if err != nil {
		return err
	}

	// ConfigMap exists, check if it needs to be updated
	if !EqualStringMaps(foundConfigMap.Data, configMap.Data) {
		log.Info("ConfigMap data has changed, updating", configMapNameField, configMap.Name)
		foundConfigMap.Data = configMap.Data
		foundConfigMap.Labels = configMap.Labels
		foundConfigMap.Annotations = configMap.Annotations
		return s.Update(ctx, foundConfigMap)
	}

	log.V(1).Info("ConfigMap is up-to-date, no changes needed", configMapNameField, configMap.Name)
	return nil
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

// reconcileStatefulSets creates or updates the StatefulSet for Redis
func (s *StandaloneController) reconcileStatefulSets(ctx context.Context, redis *koncachev1alpha1.Redis, redisConfig, configHash string) ([]*appsv1.StatefulSet, bool, error) {
	log := logf.FromContext(ctx)

	statefulSets := s.statefulsSetForRedis(redis, redisConfig, configHash)
	foundStatefulSets := make([]*appsv1.StatefulSet, 0, len(statefulSets))
	needsRequeue := false

	for _, statefulSet := range statefulSets {
		if err := controllerutil.SetControllerReference(redis, statefulSet, s.Scheme); err != nil {
			return nil, false, err
		}

		foundStatefulSet := &appsv1.StatefulSet{}
		err := s.Get(ctx, types.NamespacedName{Name: statefulSet.Name, Namespace: statefulSet.Namespace}, foundStatefulSet)
		if err != nil && errors.IsNotFound(err) {
			log.Info("Creating a new StatefulSet", statefulSetNamespaceField, statefulSet.Namespace, statefulSetNameField, statefulSet.Name)
			err = s.Create(ctx, statefulSet)
			if err != nil {
				return nil, false, err
			}
			// StatefulSet created successfully - mark for requeue but continue creating other StatefulSets
			foundStatefulSets = append(foundStatefulSets, statefulSet)
			needsRequeue = true
			continue
		} else if err != nil {
			return nil, false, err
		}

		needsUpdate := s.needsStatefulSetUpdate(foundStatefulSet, statefulSet)
		log.V(1).Info("StatefulSet update check completed",
			statefulSetNameField, foundStatefulSet.Name,
			"needs_update", needsUpdate)

		if !needsUpdate {
			log.V(1).Info("StatefulSet is up-to-date, no changes needed", statefulSetNameField, foundStatefulSet.Name)
			// No changes needed, add to found StatefulSets and continue
			foundStatefulSets = append(foundStatefulSets, foundStatefulSet)
			continue
		}

		log.Info("StatefulSet spec has changed, updating", statefulSetNameField, foundStatefulSet.Name)

		// Update the existing StatefulSet with the new spec
		foundStatefulSet.Spec = statefulSet.Spec
		foundStatefulSet.Labels = statefulSet.Labels
		foundStatefulSet.Annotations = statefulSet.Annotations

		if err := s.Update(ctx, foundStatefulSet); err != nil {
			return nil, false, err
		}

		log.Info("StatefulSet updated successfully", statefulSetNameField, foundStatefulSet.Name)
		foundStatefulSets = append(foundStatefulSets, foundStatefulSet)
		needsRequeue = true
	}

	// Return requeue status based on whether any StatefulSets were created or updated
	return foundStatefulSets, needsRequeue, nil
}

// configMapForRedis returns a ConfigMap object for the Redis configuration
func (s *StandaloneController) configMapForRedis(redis *koncachev1alpha1.Redis, redisConfig string) *corev1.ConfigMap {
	labels := LabelsForRedis(redis.Name)

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redis.Name + "-config",
			Namespace: redis.Namespace,
			Labels:    labels,
		},
		Data: map[string]string{
			"redis.conf": redisConfig,
		},
	}
}

// serviceForRedis returns a Service object for Redis
func (s *StandaloneController) serviceForRedis(redis *koncachev1alpha1.Redis) *corev1.Service {
	labels := LabelsForRedis(redis.Name)
	port := GetRedisPort(redis)

	if redis.Spec.HighAvailability != nil && redis.Spec.HighAvailability.Enabled {
		// add the leader label for HA Redis
		labels[leaderAnnotationField] = "leader"
	}

	// Build service ports
	var servicePorts []corev1.ServicePort

	// If TLS is enabled, only expose TLS port, otherwise expose regular Redis port
	if IsTLSEnabled(redis) {
		servicePorts = append(servicePorts, corev1.ServicePort{
			Name:       "redis-tls",
			Port:       6380,
			TargetPort: intstr.FromInt(6380),
			Protocol:   corev1.ProtocolTCP,
		})
	} else {
		servicePorts = append(servicePorts, corev1.ServicePort{
			Name:       "redis",
			Port:       port,
			TargetPort: intstr.FromInt(int(port)),
			Protocol:   corev1.ProtocolTCP,
		})
	}

	// Add metrics port if monitoring is enabled
	if IsMonitoringEnabled(redis) {
		exporterPort := GetRedisExporterPort(redis)
		servicePorts = append(servicePorts, corev1.ServicePort{
			Name:       "metrics",
			Port:       exporterPort,
			TargetPort: intstr.FromInt(int(exporterPort)),
			Protocol:   corev1.ProtocolTCP,
		})
	}

	// Add prometheus.io annotations for metrics scraping if monitoring is enabled
	annotations := make(map[string]string)
	for k, v := range redis.Spec.ServiceAnnotations {
		annotations[k] = v
	}

	if IsMonitoringEnabled(redis) {
		annotations["prometheus.io/scrape"] = "true"
		annotations["prometheus.io/port"] = fmt.Sprintf("%d", GetRedisExporterPort(redis))
		annotations["prometheus.io/path"] = "/metrics"
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        redis.Name,
			Namespace:   redis.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Type:     redis.Spec.ServiceType,
			Selector: labels,
			Ports:    servicePorts,
		},
	}
}

// statefulsSetForRedis returns a StatefulSet object for Redis
func (s *StandaloneController) statefulsSetForRedis(redis *koncachev1alpha1.Redis, redisConfig, configHash string) []*appsv1.StatefulSet {
	labels := LabelsForRedis(redis.Name)
	replicas := int32(1) // Standalone Redis always has 1 replica
	port := GetRedisPort(redis)

	// Build container
	container := BuildRedisContainer(redis, port)

	// Build containers list starting with Redis container
	containers := []corev1.Container{container}

	// Add Redis exporter sidecar if monitoring is enabled
	if IsMonitoringEnabled(redis) {
		exporterContainer := BuildRedisExporterContainer(redis, port)
		containers = append(containers, exporterContainer)
	}

	// Prepare pod annotations - start with user-specified annotations
	podAnnotations := make(map[string]string)
	if redis.Spec.PodAnnotations != nil {
		maps.Copy(podAnnotations, redis.Spec.PodAnnotations)
	}

	// Add config hash annotation to trigger pod restart when ConfigMap changes
	podAnnotations[configHashField] = configHash

	// Build init containers if backup init is enabled
	var initContainers []corev1.Container
	if redis.Spec.Backup.BackUpInitConfig.Enabled {
		backupInitContainer := BuildBackupInitContainer(redis)
		initContainers = append(initContainers, backupInitContainer)
	}

	// Build pod template
	podTemplate := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      labels,
			Annotations: podAnnotations,
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: redis.Spec.ServiceAccount,
			NodeSelector:       redis.Spec.NodeSelector,
			Tolerations:        redis.Spec.Tolerations,
			Affinity:           redis.Spec.Affinity,
			ImagePullSecrets:   redis.Spec.ImagePullSecrets,
			InitContainers:     initContainers,
			Containers:         containers,
			Volumes:            buildVolumes(redis),
		},
	}

	// Add custom labels if specified
	maps.Copy(podTemplate.ObjectMeta.Labels, redis.Spec.PodLabels)

	// Build volume claim template
	volumeClaimTemplate := BuildVolumeClaimTemplate(redis)

	// Create the StatefulSet objects
	// Note: We only create one StatefulSet for standalone Redis
	//       but create multiple StatefulSets for HA Redis

	if redis.Spec.HighAvailability == nil || !redis.Spec.HighAvailability.Enabled {
		statefulsets := make([]*appsv1.StatefulSet, 0, 1)
		statefulsets = append(statefulsets, &appsv1.StatefulSet{
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
		})

		return statefulsets
	}

	statefulsets := make([]*appsv1.StatefulSet, 0, int(redis.Spec.HighAvailability.Replicas))

	// For HA Redis, we create multiple StatefulSets
	for i := 0; i < int(redis.Spec.HighAvailability.Replicas); i++ {
		// if i == 0 {
		// 	// mark the first StatefulSet as primary
		// 	labels["app.kubernetes.io/role"] = "primary"
		// } else {
		// 	// mark subsequent StatefulSets as replicas
		// 	labels["app.kubernetes.io/role"] = "replica"
		// }

		statefulsets = append(statefulsets, &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      redis.Name + fmt.Sprintf("-ha-%d", i+1),
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
		})
	}

	return statefulsets
}

// updateRedisStatus updates the status of the Redis instance
func (s *StandaloneController) updateRedisStatus(ctx context.Context, redis *koncachev1alpha1.Redis, statefulSets []*appsv1.StatefulSet) error {
	// Create a retry mechanism to handle conflicts
	return s.updateStatusWithRetry(ctx, redis, func(currentRedis *koncachev1alpha1.Redis) {
		for _, statefulSet := range statefulSets {
			// Update status based on StatefulSet status
			currentRedis.Status.ObservedGeneration = currentRedis.Generation

			currentRedis.Status.Ready = statefulSet.Status.ReadyReplicas == *statefulSet.Spec.Replicas
			if currentRedis.Status.Ready {
				currentRedis.Status.Phase = koncachev1alpha1.RedisPhaseRunning
			} else {
				currentRedis.Status.Phase = koncachev1alpha1.RedisPhasePending
			}

			// Set endpoint
			port := GetRedisPort(currentRedis)
			currentRedis.Status.Endpoint = fmt.Sprintf("%s.%s.svc.cluster.local", currentRedis.Name, currentRedis.Namespace)
			currentRedis.Status.Port = port
			currentRedis.Status.Version = currentRedis.Spec.Version
		}
	})
}

// updateStatusWithRetry handles the status update with retry mechanism to avoid conflicts
func (s *StandaloneController) updateStatusWithRetry(ctx context.Context, redis *koncachev1alpha1.Redis, updateFunc func(*koncachev1alpha1.Redis)) error {
	const maxRetries = 5

	for i := range maxRetries {
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

// reconcileBackupStatefulSet creates or updates the backup StatefulSet
func (s *StandaloneController) reconcileBackupStatefulSet(ctx context.Context, redis *koncachev1alpha1.Redis) error {
	log := logf.FromContext(ctx)

	statefulSetForBackup := s.backupStatefulSetForRedis(redis)
	if err := controllerutil.SetControllerReference(redis, statefulSetForBackup, s.Scheme); err != nil {
		return err
	}

	foundBackupStatefulSet := &appsv1.StatefulSet{}
	err := s.Get(ctx, types.NamespacedName{Name: statefulSetForBackup.Name, Namespace: statefulSetForBackup.Namespace}, foundBackupStatefulSet)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating a new backup StatefulSet",
			"StatefulSet.Namespace", statefulSetForBackup.Namespace,
			statefulSetNameField, statefulSetForBackup.Name)
		return s.Create(ctx, statefulSetForBackup)
	} else if err != nil {
		log.Error(err, "Failed to get backup StatefulSet",
			"StatefulSet.Namespace", statefulSetForBackup.Namespace,
			statefulSetNameField, statefulSetForBackup.Name)
		return err
	}

	// Check if backup StatefulSet needs to be updated
	if !s.needsBackupStatefulSetUpdate(foundBackupStatefulSet, statefulSetForBackup) {
		log.V(1).Info("Backup StatefulSet is up-to-date, no changes needed",
			statefulSetNameField, foundBackupStatefulSet.Name)
		return nil
	}

	log.Info("Backup StatefulSet spec has changed, updating",
		statefulSetNameField, foundBackupStatefulSet.Name)

	// Update the existing backup StatefulSet with the new spec
	// Note: VolumeClaimTemplates cannot be updated in StatefulSets, so we only update what's allowed
	foundBackupStatefulSet.Spec.Replicas = statefulSetForBackup.Spec.Replicas
	foundBackupStatefulSet.Spec.Template = statefulSetForBackup.Spec.Template
	foundBackupStatefulSet.Spec.UpdateStrategy = statefulSetForBackup.Spec.UpdateStrategy
	foundBackupStatefulSet.Labels = statefulSetForBackup.Labels
	foundBackupStatefulSet.Annotations = statefulSetForBackup.Annotations

	if err := s.Update(ctx, foundBackupStatefulSet); err != nil {
		return err
	}

	return nil
}

func (s *StandaloneController) backupStatefulSetForRedis(redis *koncachev1alpha1.Redis) *appsv1.StatefulSet {
	var single int32 = 1

	// Create backup-specific labels
	labels := LabelsForRedis(redis.Name)
	labels["app.kubernetes.io/component"] = "backup"

	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redis.Name + "-backup",
			Namespace: redis.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &single,
			ServiceName: redis.Name + "-backup",
			Selector:    &metav1.LabelSelector{MatchLabels: labels},
			Template:    BuildBackupPodTemplateForRedis(redis),
			// No volume claim templates needed for streaming backup
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
			},
		},
	}
}

// needsStatefulSetUpdate checks if the StatefulSet needs to be updated based on the Redis spec
func (s *StandaloneController) needsStatefulSetUpdate(existing, desired *appsv1.StatefulSet) bool {
	log := logf.Log.WithValues("statefulset", existing.Name)

	basicChanges := s.hasBasicSpecChanges(existing, desired)
	containerChanges := s.hasContainerChanges(existing, desired)
	metadataChanges := s.hasMetadataChanges(existing, desired)

	log.V(1).Info("StatefulSet update analysis",
		"statefulset", desired.Name,
		"basic_changes", basicChanges,
		"container_changes", containerChanges,
		"metadata_changes", metadataChanges)

	return basicChanges || containerChanges || metadataChanges
}

// hasBasicSpecChanges checks for basic StatefulSet spec changes
func (s *StandaloneController) hasBasicSpecChanges(existing, desired *appsv1.StatefulSet) bool {
	return *existing.Spec.Replicas != *desired.Spec.Replicas ||
		existing.Spec.Template.Spec.ServiceAccountName != desired.Spec.Template.Spec.ServiceAccountName
}

// hasContainerChanges checks for container-related changes
func (s *StandaloneController) hasContainerChanges(existing, desired *appsv1.StatefulSet) bool {
	// Check if the number of containers has changed
	if len(existing.Spec.Template.Spec.Containers) != len(desired.Spec.Template.Spec.Containers) {
		return true
	}

	// Check Redis container (always the first container)
	if len(existing.Spec.Template.Spec.Containers) == 0 || len(desired.Spec.Template.Spec.Containers) == 0 {
		return true
	}

	existingRedisContainer := &existing.Spec.Template.Spec.Containers[0]
	desiredRedisContainer := &desired.Spec.Template.Spec.Containers[0]

	if s.hasContainerSpecChanges(existingRedisContainer, desiredRedisContainer) {
		return true
	}

	// Check exporter container if it exists (always the second container if present)
	if len(existing.Spec.Template.Spec.Containers) > 1 && len(desired.Spec.Template.Spec.Containers) > 1 {
		existingExporterContainer := &existing.Spec.Template.Spec.Containers[1]
		desiredExporterContainer := &desired.Spec.Template.Spec.Containers[1]

		if s.hasContainerSpecChanges(existingExporterContainer, desiredExporterContainer) {
			return true
		}
	}

	// Check init containers
	if s.hasInitContainerChanges(existing, desired) {
		return true
	}

	// Check SecurityContext
	if s.hasSecurityContextChanges(existing, desired) {
		return true
	}

	return false
}

// hasContainerSpecChanges checks if a specific container has changed
func (s *StandaloneController) hasContainerSpecChanges(existing, desired *corev1.Container) bool {
	return existing.Image != desired.Image || // Check image
		s.hasResourceChanges(existing, desired) || // Check resources
		s.hasPortChanges(existing, desired) || // Check ports
		s.hasEnvChanges(existing, desired) // Check environment variables
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
	log := logf.Log.WithValues("statefulset", existing.Name)

	nodeSelectorsEqual := EqualStringMaps(existing.Spec.Template.Spec.NodeSelector, desired.Spec.Template.Spec.NodeSelector)
	annotationsEqual := EqualStringMaps(existing.Spec.Template.Annotations, desired.Spec.Template.Annotations)
	labelsEqual := EqualStringMaps(existing.Spec.Template.Labels, desired.Spec.Template.Labels)
	statefulSetLabelsEqual := EqualStringMaps(existing.Labels, desired.Labels)
	statefulSetAnnotationsEqual := EqualStringMaps(existing.Annotations, desired.Annotations)
	tolerationsEqual := EqualTolerations(existing.Spec.Template.Spec.Tolerations, desired.Spec.Template.Spec.Tolerations)

	hasChanges := !nodeSelectorsEqual || !annotationsEqual || !labelsEqual || !statefulSetLabelsEqual || !statefulSetAnnotationsEqual || !tolerationsEqual

	log.V(1).Info("Metadata changes analysis",
		"statefulset", desired.Name,
		"node_selectors_equal", nodeSelectorsEqual,
		"annotations_equal", annotationsEqual,
		"labels_equal", labelsEqual,
		"statefulset_labels_equal", statefulSetLabelsEqual,
		"statefulset_annotations_equal", statefulSetAnnotationsEqual,
		"tolerations_equal", tolerationsEqual,
		"has_changes", hasChanges)

	if !annotationsEqual {
		existingConfigHash := ""
		desiredConfigHash := ""
		if existing.Spec.Template.Annotations != nil {
			existingConfigHash = existing.Spec.Template.Annotations["redis-operator/config-hash"]
		}
		if desired.Spec.Template.Annotations != nil {
			desiredConfigHash = desired.Spec.Template.Annotations["redis-operator/config-hash"]
		}

		log.V(1).Info("Pod template annotations differ",
			"statefulset", desired.Name,
			"existing_config_hash", existingConfigHash,
			"desired_config_hash", desiredConfigHash,
			"existing_annotations", existing.Spec.Template.Annotations,
			"desired_annotations", desired.Spec.Template.Annotations)
	}

	return hasChanges
}

// needsBackupStatefulSetUpdate checks if the backup StatefulSet needs to be updated
func (s *StandaloneController) needsBackupStatefulSetUpdate(existing, desired *appsv1.StatefulSet) bool {
	// Check basic spec differences
	if s.hasBasicSpecChanges(existing, desired) {
		return true
	}

	// Check container differences
	if s.hasContainerChanges(existing, desired) {
		return true
	}

	// Check metadata differences
	if s.hasMetadataChanges(existing, desired) {
		return true
	}

	// Check VolumeClaimTemplates differences (this is critical for PVC mismatch issues)
	// Note: VolumeClaimTemplates cannot be updated, but we can detect if they differ
	// and log a warning or handle it appropriately
	if s.hasVolumeClaimTemplateChanges(existing, desired) {
		// Log a warning since VolumeClaimTemplates cannot be updated
		logf.Log.Info("VolumeClaimTemplates differ but cannot be updated in existing StatefulSet",
			statefulSetNameField, existing.Name,
			"StatefulSet.Namespace", existing.Namespace)
		// Return false here as we cannot update VolumeClaimTemplates
		// The operator would need to delete and recreate the StatefulSet manually
		return false
	}

	return false
}

// hasVolumeClaimTemplateChanges checks if VolumeClaimTemplates have changed
func (s *StandaloneController) hasVolumeClaimTemplateChanges(existing, desired *appsv1.StatefulSet) bool {
	if len(existing.Spec.VolumeClaimTemplates) != len(desired.Spec.VolumeClaimTemplates) {
		return true
	}

	for i, existingVCT := range existing.Spec.VolumeClaimTemplates {
		if i >= len(desired.Spec.VolumeClaimTemplates) {
			return true
		}

		desiredVCT := desired.Spec.VolumeClaimTemplates[i]

		// Check if storage size has changed
		existingSize := existingVCT.Spec.Resources.Requests[corev1.ResourceStorage]
		desiredSize := desiredVCT.Spec.Resources.Requests[corev1.ResourceStorage]
		if !existingSize.Equal(desiredSize) {
			return true
		}

		// Check if storage class has changed
		if (existingVCT.Spec.StorageClassName == nil) != (desiredVCT.Spec.StorageClassName == nil) {
			return true
		}
		if existingVCT.Spec.StorageClassName != nil && desiredVCT.Spec.StorageClassName != nil &&
			*existingVCT.Spec.StorageClassName != *desiredVCT.Spec.StorageClassName {
			return true
		}

		// Check if access modes have changed
		if len(existingVCT.Spec.AccessModes) != len(desiredVCT.Spec.AccessModes) {
			return true
		}
		for j, existingMode := range existingVCT.Spec.AccessModes {
			if j >= len(desiredVCT.Spec.AccessModes) || existingMode != desiredVCT.Spec.AccessModes[j] {
				return true
			}
		}
	}

	return false
}

// hasInitContainerChanges checks if init containers have changed
func (s *StandaloneController) hasInitContainerChanges(existing, desired *appsv1.StatefulSet) bool {
	log := logf.Log.WithValues("statefulset", existing.Name)

	existingInitContainers := existing.Spec.Template.Spec.InitContainers
	desiredInitContainers := desired.Spec.Template.Spec.InitContainers

	if len(existingInitContainers) != len(desiredInitContainers) {
		log.V(1).Info("Init container count changed",
			"existing_count", len(existingInitContainers),
			"desired_count", len(desiredInitContainers))
		return true
	}

	for i, existingInitContainer := range existingInitContainers {
		if i >= len(desiredInitContainers) {
			return true
		}
		desiredInitContainer := desiredInitContainers[i]

		if s.hasContainerSpecChanges(&existingInitContainer, &desiredInitContainer) {
			log.V(1).Info("Init container spec changed",
				"container_name", existingInitContainer.Name,
				"existing_image", existingInitContainer.Image,
				"desired_image", desiredInitContainer.Image)
			return true
		}
	}

	return false
}

// hasSecurityContextChanges checks if pod SecurityContext has changed
func (s *StandaloneController) hasSecurityContextChanges(existing, desired *appsv1.StatefulSet) bool {
	log := logf.Log.WithValues("statefulset", existing.Name)

	existingSC := existing.Spec.Template.Spec.SecurityContext
	desiredSC := desired.Spec.Template.Spec.SecurityContext

	// Both nil
	if existingSC == nil && desiredSC == nil {
		return false
	}

	// One nil, one not
	if (existingSC == nil) != (desiredSC == nil) {
		log.V(1).Info("SecurityContext nil state changed",
			"existing_nil", existingSC == nil,
			"desired_nil", desiredSC == nil)
		return true
	}

	// Both not nil, compare fields
	if existingSC.RunAsUser != nil && desiredSC.RunAsUser != nil {
		if *existingSC.RunAsUser != *desiredSC.RunAsUser {
			log.V(1).Info("RunAsUser changed",
				"existing", *existingSC.RunAsUser,
				"desired", *desiredSC.RunAsUser)
			return true
		}
	} else if (existingSC.RunAsUser == nil) != (desiredSC.RunAsUser == nil) {
		return true
	}

	if existingSC.RunAsGroup != nil && desiredSC.RunAsGroup != nil {
		if *existingSC.RunAsGroup != *desiredSC.RunAsGroup {
			log.V(1).Info("RunAsGroup changed",
				"existing", *existingSC.RunAsGroup,
				"desired", *desiredSC.RunAsGroup)
			return true
		}
	} else if (existingSC.RunAsGroup == nil) != (desiredSC.RunAsGroup == nil) {
		return true
	}

	return false
}

// hasEnvChanges checks if container environment variables have changed
func (s *StandaloneController) hasEnvChanges(existing, desired *corev1.Container) bool {
	if len(existing.Env) != len(desired.Env) {
		return true
	}

	// Create maps for easier comparison
	existingEnvMap := make(map[string]string)
	for _, env := range existing.Env {
		existingEnvMap[env.Name] = env.Value
	}

	desiredEnvMap := make(map[string]string)
	for _, env := range desired.Env {
		desiredEnvMap[env.Name] = env.Value
	}

	return !EqualStringMaps(existingEnvMap, desiredEnvMap)
}

// ensureLeaderExists ensures that there's at least one leader pod in HA mode
func (s *StandaloneController) ensureLeaderExists(ctx context.Context, redis *koncachev1alpha1.Redis) error {
	log := logf.FromContext(ctx)

	// List all pods for this Redis instance
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(redis.Namespace),
		client.MatchingLabels{
			"app.kubernetes.io/name":     "redis",
			"app.kubernetes.io/instance": redis.Name,
		},
	}

	err := s.List(ctx, podList, listOpts...)
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	// Check if there's already a leader
	var leaderPod *corev1.Pod
	var readyPods []*corev1.Pod

	for i := range podList.Items {
		pod := &podList.Items[i]

		// Check if this pod is the leader
		if pod.Annotations != nil && pod.Annotations["redis-operator/role"] == "leader" {
			leaderPod = pod
			log.Info("Found existing leader pod", "pod", pod.Name)
		}

		// Track ready pods as potential leader candidates
		if s.isPodReadyForLeadership(pod) {
			readyPods = append(readyPods, pod)
		}
	}

	// If there's already a leader, check if it's healthy
	if leaderPod != nil {
		if s.isPodReadyForLeadership(leaderPod) {
			log.V(1).Info("Leader pod is healthy", "leader", leaderPod.Name)
			return nil // Leader exists and is healthy
		}
		log.Info("Leader pod is not healthy, promoting new leader", "unhealthyLeader", leaderPod.Name)
	}

	// No healthy leader found, promote the first ready pod
	if len(readyPods) == 0 {
		log.Info("No ready pods available for leader promotion")
		return nil // No ready pods to promote
	}

	newLeader := readyPods[0]
	if err := s.promoteToLeader(ctx, newLeader); err != nil {
		return fmt.Errorf("failed to promote pod to leader: %w", err)
	}

	log.Info("Successfully promoted new leader",
		"newLeader", newLeader.Name,
		"redis", redis.Name)

	return nil
}

// isPodReadyForLeadership checks if a pod is ready to become or remain leader
func (s *StandaloneController) isPodReadyForLeadership(pod *corev1.Pod) bool {
	// Check pod phase
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}

	// Check if pod is ready
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			// Check Redis container status
			for _, containerStatus := range pod.Status.ContainerStatuses {
				if containerStatus.Name == "redis" {
					return containerStatus.Ready && containerStatus.State.Running != nil
				}
			}
		}
	}

	return false
}

// promoteToLeader promotes a pod to leader by adding the appropriate annotation
func (s *StandaloneController) promoteToLeader(ctx context.Context, pod *corev1.Pod) error {
	log := logf.FromContext(ctx)

	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}

	pod.Annotations[leaderAnnotationField] = "leader"
	pod.Annotations["redis-operator/promoted-at"] = time.Now().Format(time.RFC3339)

	if err := s.Update(ctx, pod); err != nil {
		return fmt.Errorf("failed to update pod annotations: %w", err)
	}

	// Configure Redis as master
	if err := s.configureRedisAsMaster(ctx, pod); err != nil {
		log.Error(err, "Failed to configure Redis as master", "pod", pod.Name)
		// Don't fail the promotion if Redis config fails - the pod is still marked as leader
	}

	log.Info("Successfully promoted pod to leader", "pod", pod.Name)
	return nil
}

// configureRedisAsMaster configures a Redis instance to become the master
func (s *StandaloneController) configureRedisAsMaster(ctx context.Context, pod *corev1.Pod) error {
	log := logf.FromContext(ctx)

	// Execute Redis command to make this instance a master
	// This removes any existing replication configuration
	commands := []string{
		"REPLICAOF NO ONE",     // Stop being a replica
		"CONFIG SET save \"\"", // Disable automatic saves during transition
	}

	for _, cmd := range commands {
		if s.CommandExecutor != nil {
			if err := s.CommandExecutor.ExecuteRedisCommand(ctx, pod, cmd); err != nil {
				log.Error(err, "Failed to execute Redis command", "pod", pod.Name, "command", cmd)
				return fmt.Errorf("failed to configure Redis as master: %w", err)
			}
		} else {
			// Fallback to direct method call if CommandExecutor is not set
			if err := s.executeRedisCommand(ctx, pod, cmd); err != nil {
				log.Error(err, "Failed to execute Redis command", "pod", pod.Name, "command", cmd)
				return fmt.Errorf("failed to configure Redis as master: %w", err)
			}
		}
	}

	log.Info("Successfully configured Redis as master", "pod", pod.Name)
	return nil
}

// configureReplicationForFollowers configures all non-leader pods as replicas of the leader
func (s *StandaloneController) configureReplicationForFollowers(ctx context.Context, redis *koncachev1alpha1.Redis, leaderPod *corev1.Pod) error {
	log := logf.FromContext(ctx)

	// List all pods for this Redis instance
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(redis.Namespace),
		client.MatchingLabels{
			"app.kubernetes.io/name":     "redis",
			"app.kubernetes.io/instance": redis.Name,
		},
	}

	err := s.List(ctx, podList, listOpts...)
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	// Get the leader's IP address for replication
	leaderIP := leaderPod.Status.PodIP
	if leaderIP == "" {
		return fmt.Errorf("leader pod %s has no IP address", leaderPod.Name)
	}

	redisPort := GetRedisPort(redis)

	// Configure each non-leader pod as a replica
	for i := range podList.Items {
		pod := &podList.Items[i]

		// Skip the leader pod
		if pod.Name == leaderPod.Name {
			continue
		}

		// Skip pods that aren't ready
		if !s.isPodReadyForLeadership(pod) {
			log.Info("Skipping non-ready pod for replication configuration", "pod", pod.Name)
			continue
		}

		// Configure this pod as a replica
		if err := s.configureRedisAsReplica(ctx, pod, leaderIP, redisPort); err != nil {
			log.Error(err, "Failed to configure pod as replica", "pod", pod.Name, "leader", leaderPod.Name)
			// Continue with other pods even if one fails
		} else {
			// Update pod annotation to mark as follower
			if err := s.markPodAsFollower(ctx, pod); err != nil {
				log.Error(err, "Failed to mark pod as follower", "pod", pod.Name)
			}
		}
	}

	return nil
}

// configureRedisAsReplica configures a Redis instance to become a replica of the specified master
func (s *StandaloneController) configureRedisAsReplica(ctx context.Context, pod *corev1.Pod, masterIP string, masterPort int32) error {
	log := logf.FromContext(ctx)

	// Execute Redis command to make this instance a replica
	cmd := fmt.Sprintf("REPLICAOF %s %d", masterIP, masterPort)

	if s.CommandExecutor != nil {
		if err := s.CommandExecutor.ExecuteRedisCommand(ctx, pod, cmd); err != nil {
			return fmt.Errorf("failed to configure Redis as replica: %w", err)
		}
	} else {
		// Fallback to direct method call if CommandExecutor is not set
		if err := s.executeRedisCommand(ctx, pod, cmd); err != nil {
			return fmt.Errorf("failed to configure Redis as replica: %w", err)
		}
	}

	log.Info("Successfully configured Redis as replica",
		"pod", pod.Name,
		"master", masterIP,
		"port", masterPort)
	return nil
}

// markPodAsFollower updates pod annotations to mark it as a follower
func (s *StandaloneController) markPodAsFollower(ctx context.Context, pod *corev1.Pod) error {
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}

	pod.Annotations[leaderAnnotationField] = "follower"
	pod.Annotations["redis-operator/configured-at"] = time.Now().Format(time.RFC3339)

	return s.Update(ctx, pod)
}

// executeRedisCommand executes a Redis command on the specified pod
func (s *StandaloneController) executeRedisCommand(ctx context.Context, pod *corev1.Pod, command string) error {
	log := logf.FromContext(ctx)

	// Get Redis instance to access configuration
	redis := &koncachev1alpha1.Redis{}
	err := s.Get(ctx, types.NamespacedName{
		Name:      pod.Labels["app.kubernetes.io/instance"],
		Namespace: pod.Namespace,
	}, redis)
	if err != nil {
		return fmt.Errorf("failed to get Redis instance: %w", err)
	}

	// Create Redis client options
	addr := net.JoinHostPort(pod.Status.PodIP, fmt.Sprintf("%d", GetRedisPort(redis)))

	opts := &redisClient.Options{
		Addr:         addr,
		DialTimeout:  10 * time.Second,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Configure authentication if required
	if redis.Spec.Security.RequireAuth != nil && *redis.Spec.Security.RequireAuth {
		if redis.Spec.Security.PasswordSecret != nil {
			password, err := s.getRedisPassword(ctx, redis)
			if err != nil {
				return fmt.Errorf("failed to get Redis password: %w", err)
			}
			opts.Password = password
		}
	}

	// Configure TLS if enabled
	if redis.Spec.Security.TLS != nil && redis.Spec.Security.TLS.Enabled {
		opts.TLSConfig = &tls.Config{
			ServerName: pod.Status.PodIP,
			// In production, you might want to configure certificates properly
			InsecureSkipVerify: true,
		}
	}

	// Create Redis client
	rdb := redisClient.NewClient(opts)
	defer rdb.Close()

	// Test connection
	_, err = rdb.Ping(ctx).Result()
	if err != nil {
		return fmt.Errorf("failed to connect to Redis pod %s: %w", pod.Name, err)
	}

	// Execute the command
	log.Info("Executing Redis command", "pod", pod.Name, "command", command)

	// Parse the command and execute it
	result := rdb.Do(ctx, parseRedisCommand(command)...)
	if result.Err() != nil {
		return fmt.Errorf("failed to execute Redis command '%s' on pod %s: %w", command, pod.Name, result.Err())
	}

	log.Info("Successfully executed Redis command", "pod", pod.Name, "command", command, "result", result.Val())
	return nil
}

// getRedisPassword retrieves the Redis password from the secret
func (s *StandaloneController) getRedisPassword(ctx context.Context, redis *koncachev1alpha1.Redis) (string, error) {
	if redis.Spec.Security.PasswordSecret == nil {
		return "", nil
	}

	secret := &corev1.Secret{}
	err := s.Get(ctx, types.NamespacedName{
		Name:      redis.Spec.Security.PasswordSecret.Name,
		Namespace: redis.Namespace,
	}, secret)
	if err != nil {
		return "", fmt.Errorf("failed to get password secret: %w", err)
	}

	key := redis.Spec.Security.PasswordSecret.Key
	if key == "" {
		key = "password" // default key
	}

	password, exists := secret.Data[key]
	if !exists {
		return "", fmt.Errorf("password key '%s' not found in secret '%s'", key, redis.Spec.Security.PasswordSecret.Name)
	}

	return string(password), nil
}

// parseRedisCommand parses a Redis command string into arguments
func parseRedisCommand(command string) []interface{} {
	// Simple space-based parsing - in production you might want more sophisticated parsing
	parts := strings.Fields(command)
	args := make([]interface{}, len(parts))
	for i, part := range parts {
		args[i] = part
	}
	return args
}

// ensureReplicationConfiguration ensures all pods have correct replication configuration
func (s *StandaloneController) ensureReplicationConfiguration(ctx context.Context, redis *koncachev1alpha1.Redis) error {
	log := logf.FromContext(ctx)

	// Only configure replication for HA mode
	if redis.Spec.HighAvailability == nil || !redis.Spec.HighAvailability.Enabled {
		return nil
	}

	// Find the current leader
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(redis.Namespace),
		client.MatchingLabels{
			"app.kubernetes.io/name":     "redis",
			"app.kubernetes.io/instance": redis.Name,
		},
	}

	err := s.List(ctx, podList, listOpts...)
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	var leaderPod *corev1.Pod
	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.Annotations != nil && pod.Annotations[leaderAnnotationField] == "leader" {
			leaderPod = pod
			break
		}
	}

	if leaderPod == nil {
		log.Info("No leader pod found, skipping replication configuration")
		return nil
	}

	// Configure replication for all followers
	return s.configureReplicationForFollowers(ctx, redis, leaderPod)
}

// cleanupOrphanedStatefulSets removes StatefulSets that are no longer needed
func (s *StandaloneController) cleanupOrphanedStatefulSets(ctx context.Context, redis *koncachev1alpha1.Redis, expectedStatefulSets []*appsv1.StatefulSet) error {
	log := logf.FromContext(ctx)

	// Get all existing StatefulSets for this Redis instance
	existingStatefulSets := &appsv1.StatefulSetList{}
	listOpts := []client.ListOption{
		client.InNamespace(redis.Namespace),
		client.MatchingLabels{
			"app.kubernetes.io/name":     "redis",
			"app.kubernetes.io/instance": redis.Name,
		},
	}

	err := s.List(ctx, existingStatefulSets, listOpts...)
	if err != nil {
		return fmt.Errorf("failed to list existing StatefulSets: %w", err)
	}

	// Create a map of expected StatefulSet names for quick lookup
	expectedNames := make(map[string]bool)
	for _, ss := range expectedStatefulSets {
		expectedNames[ss.Name] = true
	}

	// Delete StatefulSets that are no longer expected
	for i := range existingStatefulSets.Items {
		ss := &existingStatefulSets.Items[i]

		// Check if this StatefulSet is owned by this Redis instance
		if !s.isOwnedByRedis(ss, redis) {
			continue
		}

		// If not in expected list, delete it
		if !expectedNames[ss.Name] {
			log.Info("Deleting orphaned StatefulSet", "StatefulSet", ss.Name)
			if err := s.Delete(ctx, ss); err != nil {
				log.Error(err, "Failed to delete orphaned StatefulSet", "StatefulSet", ss.Name)
				// Continue with other StatefulSets even if one fails
			}
		}
	}

	return nil
}

// isOwnedByRedis checks if a StatefulSet is owned by the specified Redis instance
func (s *StandaloneController) isOwnedByRedis(statefulSet *appsv1.StatefulSet, redis *koncachev1alpha1.Redis) bool {
	// Check owner references
	for _, owner := range statefulSet.OwnerReferences {
		// First try to match by UID (preferred method)
		if redis.UID != "" && owner.UID == redis.UID {
			return true
		}

		// Fallback to match by name and kind (useful for tests where UID might not be set)
		if owner.Kind == "Redis" && owner.Name == redis.Name {
			return true
		}
	}
	return false
}
