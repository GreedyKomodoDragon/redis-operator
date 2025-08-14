package controller

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	koncachev1alpha1 "github.com/GreedyKomodoDragon/redis-operator/api/v1alpha1"
)

const (
	// Label constants
	redisNameLabel     = "app.kubernetes.io/name"
	redisInstanceLabel = "app.kubernetes.io/instance"

	// Annotation constants
	redisRoleAnnotation       = "redis-operator/role"
	redisPromotedAtAnnotation = "redis-operator/promoted-at"

	// Role values
	redisLeaderRole = "leader"
)

// RedisReconciler reconciles a Redis object
type RedisReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Sub-controllers for different Redis modes
	standaloneController *StandaloneController
}

// +kubebuilder:rbac:groups=koncache.greedykomodo,resources=redis,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=koncache.greedykomodo,resources=redis/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=koncache.greedykomodo,resources=redis/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *RedisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	log.Info("Reconciling Redis", "name", req.Name, "namespace", req.Namespace)

	// Initialize sub-controllers if not already done
	if r.standaloneController == nil {
		log.Info("Initializing standalone controller")
		r.standaloneController = NewStandaloneController(r.Client, r.Scheme)
	}

	// Fetch the Redis instance
	redis := &koncachev1alpha1.Redis{}
	err := r.Get(ctx, req.NamespacedName, redis)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Redis resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get Redis")
		return ctrl.Result{}, err
	}

	log.Info("Processing Redis", "name", redis.Name, "mode", redis.Spec.Mode)

	// Handle different Redis modes
	switch redis.Spec.Mode {
	case koncachev1alpha1.RedisModeStandalone, "": // Default to standalone if mode is empty
		result, err := r.standaloneController.Reconcile(ctx, redis)
		// Add a small requeue delay for stable resources to reduce reconciliation frequency
		if err == nil && result.RequeueAfter == 0 {
			result.RequeueAfter = 30 * time.Second
		}
		return result, err
	case koncachev1alpha1.RedisModeCluster:
		return r.reconcileCluster(ctx, redis)
	default:
		log.Error(fmt.Errorf("unsupported Redis mode: %s", redis.Spec.Mode), "Invalid Redis mode")
		return ctrl.Result{}, fmt.Errorf("unsupported Redis mode: %s", redis.Spec.Mode)
	}
}

// reconcileCluster handles the reconciliation of a Redis cluster
func (r *RedisReconciler) reconcileCluster(ctx context.Context, _ *koncachev1alpha1.Redis) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("Cluster mode reconciliation not yet implemented")
	// TODO: Implement cluster mode reconciliation
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RedisReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&koncachev1alpha1.Redis{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Watches(
			&corev1.Pod{},
			handler.EnqueueRequestsFromMapFunc(r.handlePodEvent),
		).
		Named("redis").
		Complete(r)
}

// handlePodEvent processes pod events to detect failures
func (r *RedisReconciler) handlePodEvent(ctx context.Context, obj client.Object) []reconcile.Request {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return []reconcile.Request{}
	}

	// Only process pods that belong to Redis instances
	if pod.Labels[redisNameLabel] != "redis" {
		return []reconcile.Request{}
	}

	// Detect pod failure
	if err := r.detectPodFailure(ctx, pod); err != nil {
		log := logf.FromContext(ctx)
		log.Error(err, "Failed to process pod failure detection",
			"pod", pod.Name,
			"namespace", pod.Namespace)
	}

	// Always return empty slice since we handle the reconciliation internally
	return []reconcile.Request{}
}

// detectPodFailure checks if a pod has failed and triggers failover if needed
func (r *RedisReconciler) detectPodFailure(ctx context.Context, pod *corev1.Pod) error {
	// Check if this pod belongs to a Redis instance with HA enabled
	redis, shouldProcess := r.shouldProcessPodFailure(ctx, pod)
	if !shouldProcess {
		return nil
	}

	// Check if pod is in a failed state
	podFailed, reason := r.isPodFailed(pod)
	if !podFailed {
		return nil
	}

	// Log the failure
	r.logPodFailure(ctx, pod, redis.Name, reason)

	// Check if the failed pod was the leader and handle leader promotion
	if err := r.handleLeaderFailover(ctx, pod, redis); err != nil {
		return fmt.Errorf("failed to handle leader failover: %w", err)
	}

	// Trigger a reconciliation of the Redis instance for failover
	return r.triggerRedisReconciliation(ctx, redis)
}

// shouldProcessPodFailure determines if we should process this pod for failure detection
func (r *RedisReconciler) shouldProcessPodFailure(ctx context.Context, pod *corev1.Pod) (*koncachev1alpha1.Redis, bool) {
	// Check if this pod belongs to a Redis instance
	redisName, exists := pod.Labels[redisInstanceLabel]
	if !exists {
		return nil, false // Not a Redis pod
	}

	// Get the Redis instance
	redis := &koncachev1alpha1.Redis{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      redisName,
		Namespace: pod.Namespace,
	}, redis)
	if err != nil {
		return nil, false // Redis instance no longer exists or error
	}

	// Check if HA is enabled and auto failover is configured
	if redis.Spec.HighAvailability == nil || !redis.Spec.HighAvailability.Enabled {
		return nil, false // HA not enabled
	}

	autoFailover := true // default value
	if redis.Spec.HighAvailability.AutoFailover != nil {
		autoFailover = *redis.Spec.HighAvailability.AutoFailover
	}

	if !autoFailover {
		return nil, false // Auto failover disabled
	}

	return redis, true
}

// isPodFailed checks if a pod is in a failed state
func (r *RedisReconciler) isPodFailed(pod *corev1.Pod) (bool, string) {
	switch pod.Status.Phase {
	case corev1.PodFailed:
		return true, "Pod phase is Failed"
	case corev1.PodSucceeded:
		return true, "Pod completed unexpectedly"
	case corev1.PodRunning, corev1.PodPending:
		return r.checkContainerFailures(pod)
	}
	return false, ""
}

// checkContainerFailures checks if any containers in the pod have failed
func (r *RedisReconciler) checkContainerFailures(pod *corev1.Pod) (bool, string) {
	log := logf.FromContext(context.Background())

	for _, containerStatus := range pod.Status.ContainerStatuses {
		if containerStatus.Name != "redis" {
			continue
		}

		// Check for terminated container with error
		if containerStatus.State.Terminated != nil {
			termination := containerStatus.State.Terminated
			if termination.ExitCode != 0 {
				return true, fmt.Sprintf("Redis container terminated with exit code %d: %s",
					termination.ExitCode, termination.Reason)
			}
		}

		// Check for waiting container in error state
		if containerStatus.State.Waiting != nil {
			waiting := containerStatus.State.Waiting
			if r.isWaitingStateFailure(waiting.Reason) {
				return true, fmt.Sprintf("Redis container in waiting state: %s - %s",
					waiting.Reason, waiting.Message)
			}
		}

		// Log high restart count as warning
		if containerStatus.RestartCount > 3 {
			log.Info("Redis container has high restart count",
				"pod", pod.Name,
				"namespace", pod.Namespace,
				"restartCount", containerStatus.RestartCount)
		}
	}
	return false, ""
}

// isWaitingStateFailure checks if a waiting reason indicates failure
func (r *RedisReconciler) isWaitingStateFailure(reason string) bool {
	failureReasons := []string{
		"CrashLoopBackOff",
		"ImagePullBackOff",
		"ErrImagePull",
	}

	for _, failureReason := range failureReasons {
		if reason == failureReason {
			return true
		}
	}
	return false
}

// logPodFailure logs the pod failure detection
func (r *RedisReconciler) logPodFailure(ctx context.Context, pod *corev1.Pod, redisName, reason string) {
	log := logf.FromContext(ctx)

	log.Info("REDIS POD FAILURE DETECTED",
		"pod", pod.Name,
		"namespace", pod.Namespace,
		"redis", redisName,
		"reason", reason,
		"phase", pod.Status.Phase,
		"timestamp", time.Now().Format(time.RFC3339))

	// Log additional details about the failure
	r.logPodFailureDetails(ctx, pod)
}

// logPodFailureDetails logs detailed information about the pod failure
func (r *RedisReconciler) logPodFailureDetails(ctx context.Context, pod *corev1.Pod) {
	log := logf.FromContext(ctx)

	log.Info("Pod Failure Details",
		"pod", pod.Name,
		"namespace", pod.Namespace,
		"phase", pod.Status.Phase,
		"message", pod.Status.Message,
		"reason", pod.Status.Reason,
		"startTime", pod.Status.StartTime,
		"deletionTimestamp", pod.DeletionTimestamp)

	// Log container statuses
	for _, containerStatus := range pod.Status.ContainerStatuses {
		log.Info("Container Status",
			"container", containerStatus.Name,
			"ready", containerStatus.Ready,
			"restartCount", containerStatus.RestartCount,
			"image", containerStatus.Image)

		if containerStatus.State.Terminated != nil {
			term := containerStatus.State.Terminated
			log.Info("Container Terminated",
				"container", containerStatus.Name,
				"exitCode", term.ExitCode,
				"reason", term.Reason,
				"message", term.Message,
				"startedAt", term.StartedAt,
				"finishedAt", term.FinishedAt)
		}

		if containerStatus.State.Waiting != nil {
			waiting := containerStatus.State.Waiting
			log.Info("Container Waiting",
				"container", containerStatus.Name,
				"reason", waiting.Reason,
				"message", waiting.Message)
		}

		// Log recent events related to this container
		if containerStatus.LastTerminationState.Terminated != nil {
			lastTerm := containerStatus.LastTerminationState.Terminated
			log.Info("Last Termination State",
				"container", containerStatus.Name,
				"exitCode", lastTerm.ExitCode,
				"reason", lastTerm.Reason,
				"message", lastTerm.Message)
		}
	}
}

// triggerRedisReconciliation triggers a reconciliation of the Redis instance
func (r *RedisReconciler) triggerRedisReconciliation(ctx context.Context, redis *koncachev1alpha1.Redis) error {
	log := logf.FromContext(ctx)

	// Update the Redis status to indicate a failover is needed
	if redis.Status.Conditions == nil {
		redis.Status.Conditions = []metav1.Condition{}
	}

	// Add or update failover condition
	condition := metav1.Condition{
		Type:               "FailoverRequired",
		Status:             metav1.ConditionTrue,
		Reason:             "PodFailureDetected",
		Message:            "Pod failure detected, automatic failover triggered",
		LastTransitionTime: metav1.NewTime(time.Now()),
	}

	// Update or append the condition
	conditionUpdated := false
	for i, existingCondition := range redis.Status.Conditions {
		if existingCondition.Type == condition.Type {
			redis.Status.Conditions[i] = condition
			conditionUpdated = true
			break
		}
	}
	if !conditionUpdated {
		redis.Status.Conditions = append(redis.Status.Conditions, condition)
	}

	// Update the status
	if err := r.Status().Update(ctx, redis); err != nil {
		log.Error(err, "Failed to update Redis status with failover condition")
		return err
	}

	log.Info("TRIGGERING REDIS FAILOVER PROCESS",
		"redis", redis.Name,
		"namespace", redis.Namespace,
		"timestamp", time.Now().Format(time.RFC3339))

	return nil
}

// DetectPodFailures actively checks for failed pods and triggers reconciliation
func (r *RedisReconciler) DetectPodFailures(ctx context.Context, redis *koncachev1alpha1.Redis) {
	log := logf.FromContext(ctx)

	// Only check if HA is enabled
	if redis.Spec.HighAvailability == nil || !redis.Spec.HighAvailability.Enabled {
		return
	}

	// List pods for this Redis instance
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(redis.Namespace),
		client.MatchingLabels{
			redisNameLabel:     "redis",
			redisInstanceLabel: redis.Name,
		},
	}

	if err := r.List(ctx, podList, listOpts...); err != nil {
		log.Error(err, "Failed to list pods for failure detection")
		return
	}

	// Check each pod for failures
	for _, pod := range podList.Items {
		podFailed, reason := r.isPodFailed(&pod)
		if podFailed {
			log.Info("Active pod failure detection found failed pod",
				"pod", pod.Name,
				"namespace", pod.Namespace,
				"reason", reason)

			// Process the failure
			if err := r.detectPodFailure(ctx, &pod); err != nil {
				log.Error(err, "Failed to process detected pod failure")
			}
		}
	}
}

// handleLeaderFailover handles leader promotion when a leader pod fails
func (r *RedisReconciler) handleLeaderFailover(ctx context.Context, failedPod *corev1.Pod, redis *koncachev1alpha1.Redis) error {
	log := logf.FromContext(ctx)

	// Check if the failed pod was the leader
	isLeader := r.isPodLeader(failedPod)
	if !isLeader {
		log.Info("Failed pod was not the leader, no leader promotion needed",
			"pod", failedPod.Name,
			"namespace", failedPod.Namespace)
		return nil
	}

	log.Info("Leader pod failed, initiating leader promotion",
		"failedPod", failedPod.Name,
		"namespace", failedPod.Namespace,
		"redis", redis.Name)

	// Find candidate pods from other StatefulSets
	candidatePods, err := r.findLeaderCandidates(ctx, redis, failedPod)
	if err != nil {
		return fmt.Errorf("failed to find leader candidates: %w", err)
	}

	if len(candidatePods) == 0 {
		log.Info("No healthy candidate pods found for leader promotion")
		return nil
	}

	// Select the best candidate (first healthy pod found)
	newLeader := candidatePods[0]

	// Promote the new leader
	if err := r.promoteNewLeader(ctx, newLeader, failedPod); err != nil {
		return fmt.Errorf("failed to promote new leader: %w", err)
	}

	log.Info("Successfully promoted new leader",
		"newLeader", newLeader.Name,
		"failedPod", failedPod.Name,
		"redis", redis.Name)

	return nil
}

// isPodLeader checks if a pod is currently marked as the leader
func (r *RedisReconciler) isPodLeader(pod *corev1.Pod) bool {
	if pod.Annotations == nil {
		return false
	}
	return pod.Annotations[redisRoleAnnotation] == redisLeaderRole
}

// findLeaderCandidates finds healthy pods from other StatefulSets that can become the leader
func (r *RedisReconciler) findLeaderCandidates(ctx context.Context, redis *koncachev1alpha1.Redis, failedPod *corev1.Pod) ([]*corev1.Pod, error) {
	log := logf.FromContext(ctx)

	// List all pods belonging to this Redis instance
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(redis.Namespace),
		client.MatchingLabels{
			redisNameLabel:     "redis",
			redisInstanceLabel: redis.Name,
		},
	}

	err := r.List(ctx, podList, listOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	var candidates []*corev1.Pod
	failedStatefulSetName := r.getStatefulSetNameFromPod(failedPod)

	for i := range podList.Items {
		pod := &podList.Items[i]

		// Skip the failed pod
		if pod.Name == failedPod.Name {
			continue
		}

		// Skip pods from the same StatefulSet as the failed pod
		podStatefulSetName := r.getStatefulSetNameFromPod(pod)
		if podStatefulSetName == failedStatefulSetName {
			continue
		}

		// Check if pod is healthy and ready
		if r.isPodHealthyAndReady(pod) {
			candidates = append(candidates, pod)
			log.Info("Found leader candidate",
				"candidate", pod.Name,
				"statefulSet", podStatefulSetName,
				"phase", pod.Status.Phase)
		}
	}

	return candidates, nil
}

// getStatefulSetNameFromPod extracts the StatefulSet name from a pod's owner references
func (r *RedisReconciler) getStatefulSetNameFromPod(pod *corev1.Pod) string {
	for _, ownerRef := range pod.OwnerReferences {
		if ownerRef.Kind == "StatefulSet" {
			return ownerRef.Name
		}
	}
	return ""
}

// isPodHealthyAndReady checks if a pod is healthy and ready to become leader
func (r *RedisReconciler) isPodHealthyAndReady(pod *corev1.Pod) bool {
	// Check pod phase
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}

	// Check if pod is ready
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}

	// Check container status
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if containerStatus.Name == "redis" {
			return containerStatus.Ready && containerStatus.State.Running != nil
		}
	}

	return false
}

// promoteNewLeader promotes a pod to be the new leader
func (r *RedisReconciler) promoteNewLeader(ctx context.Context, newLeader *corev1.Pod, failedPod *corev1.Pod) error {
	log := logf.FromContext(ctx)

	// Remove leader annotation from the failed pod (if still accessible)
	if err := r.removeLeaderAnnotation(ctx, failedPod); err != nil {
		log.Error(err, "Failed to remove leader annotation from failed pod", "pod", failedPod.Name)
		// Don't fail the promotion if we can't update the failed pod
	}

	// Add leader annotation to the new leader pod
	if err := r.addLeaderAnnotation(ctx, newLeader); err != nil {
		return fmt.Errorf("failed to add leader annotation to new leader: %w", err)
	}

	log.Info("Leader promotion completed",
		"newLeader", newLeader.Name,
		"newLeaderStatefulSet", r.getStatefulSetNameFromPod(newLeader),
		"failedPod", failedPod.Name,
		"failedStatefulSet", r.getStatefulSetNameFromPod(failedPod))

	return nil
}

// addLeaderAnnotation adds the leader annotation to a pod
func (r *RedisReconciler) addLeaderAnnotation(ctx context.Context, pod *corev1.Pod) error {
	// Create a patch to add the leader annotation
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	pod.Annotations[redisRoleAnnotation] = redisLeaderRole
	pod.Annotations[redisPromotedAtAnnotation] = time.Now().Format(time.RFC3339)

	return r.Update(ctx, pod)
}

// removeLeaderAnnotation removes the leader annotation from a pod
func (r *RedisReconciler) removeLeaderAnnotation(ctx context.Context, pod *corev1.Pod) error {
	if pod.Annotations == nil {
		return nil // No annotations to remove
	}

	delete(pod.Annotations, redisRoleAnnotation)
	delete(pod.Annotations, redisPromotedAtAnnotation)

	return r.Update(ctx, pod)
}
