package controller

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

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
		Named("redis").
		Complete(r)
}
