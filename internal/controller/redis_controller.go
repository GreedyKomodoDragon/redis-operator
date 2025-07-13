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
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *RedisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Initialize sub-controllers if not already done
	if r.standaloneController == nil {
		r.standaloneController = NewStandaloneController(r.Client, r.Scheme)
	}

	// Fetch the Redis instance
	redis := &koncachev1alpha1.Redis{}
	err := r.Get(ctx, req.NamespacedName, redis)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			log.Info("Redis resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get Redis")
		return ctrl.Result{}, err
	}

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
func (r *RedisReconciler) reconcileCluster(ctx context.Context, redis *koncachev1alpha1.Redis) (ctrl.Result, error) {
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
	if pod.Labels["app.kubernetes.io/name"] != "redis" {
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

	// Trigger a reconciliation of the Redis instance for failover
	return r.triggerRedisReconciliation(ctx, redis)
}

// shouldProcessPodFailure determines if we should process this pod for failure detection
func (r *RedisReconciler) shouldProcessPodFailure(ctx context.Context, pod *corev1.Pod) (*koncachev1alpha1.Redis, bool) {
	// Check if this pod belongs to a Redis instance
	redisName, exists := pod.Labels["app.kubernetes.io/instance"]
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

	log.Info("🚨 REDIS POD FAILURE DETECTED 🚨",
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

	log.Info("🔄 TRIGGERING REDIS FAILOVER PROCESS 🔄",
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
			"app.kubernetes.io/name":     "redis",
			"app.kubernetes.io/instance": redis.Name,
		},
	}

	err := r.List(ctx, podList, listOpts...)
	if err != nil {
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
