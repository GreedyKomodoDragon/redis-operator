package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	koncachev1alpha1 "github.com/GreedyKomodoDragon/redis-operator/api/v1alpha1"
)

var istioLog = logf.Log.WithName("istio-manager")

// Constants for Istio resource management
const (
	IstioNetworkingGroup = "networking.istio.io"
	IstioSecurityGroup   = "security.istio.io"
	IstioBetaVersion     = "v1beta1"

	SidecarInjectAnnotation   = "sidecar.istio.io/inject"
	SidecarTemplateAnnotation = "sidecar.istio.io/template"

	MeshExternalLocation = "MESH_EXTERNAL"
	DNSResolution        = "DNS"
	StrictMTLSMode       = "STRICT"
)

// IstioManager handles Istio resource management for Redis instances
type IstioManager struct {
	client.Client
	Scheme *runtime.Scheme
}

// NewIstioManager creates a new IstioManager
func NewIstioManager(client client.Client, scheme *runtime.Scheme) *IstioManager {
	return &IstioManager{
		Client: client,
		Scheme: scheme,
	}
}

// Istio GroupVersionResource definitions
var (
	VirtualServiceGVR = schema.GroupVersionResource{
		Group:    IstioNetworkingGroup,
		Version:  IstioBetaVersion,
		Resource: "virtualservices",
	}
	DestinationRuleGVR = schema.GroupVersionResource{
		Group:    IstioNetworkingGroup,
		Version:  IstioBetaVersion,
		Resource: "destinationrules",
	}
	ServiceEntryGVR = schema.GroupVersionResource{
		Group:    IstioNetworkingGroup,
		Version:  IstioBetaVersion,
		Resource: "serviceentries",
	}
	PeerAuthenticationGVR = schema.GroupVersionResource{
		Group:    IstioSecurityGroup,
		Version:  IstioBetaVersion,
		Resource: "peerauthentications",
	}
)

// ReconcileIstioResources reconciles all Istio resources for a Redis instance
func (im *IstioManager) ReconcileIstioResources(ctx context.Context, redis *koncachev1alpha1.Redis) error {
	log := logf.FromContext(ctx)
	log.Info("Starting Istio resource reconciliation", "redis", redis.Name, "namespace", redis.Namespace)

	// Debug logging
	if redis.Spec.Networking == nil {
		log.Info("No networking configuration found", "redis", redis.Name)
		return im.cleanupIstioResources(ctx, redis)
	}

	if redis.Spec.Networking.Istio == nil {
		log.Info("No Istio configuration found", "redis", redis.Name)
		return im.cleanupIstioResources(ctx, redis)
	}

	if !redis.Spec.Networking.Istio.Enabled {
		log.Info("Istio is disabled", "redis", redis.Name, "enabled", redis.Spec.Networking.Istio.Enabled)
		return im.cleanupIstioResources(ctx, redis)
	}

	log.Info("Istio is enabled, proceeding with reconciliation", "redis", redis.Name)

	istioConfig := redis.Spec.Networking.Istio

	// Reconcile each Istio resource type
	reconcileFuncs := []func() error{
		func() error { return im.reconcileVirtualServiceIfEnabled(ctx, redis, istioConfig.VirtualService) },
		func() error { return im.reconcileDestinationRuleIfEnabled(ctx, redis, istioConfig.DestinationRule) },
		func() error { return im.reconcileServiceEntryIfEnabled(ctx, redis, istioConfig.ServiceEntry) },
		func() error {
			return im.reconcilePeerAuthenticationIfEnabled(ctx, redis, istioConfig.PeerAuthentication)
		},
	}

	for _, reconcileFunc := range reconcileFuncs {
		if err := reconcileFunc(); err != nil {
			return err
		}
	}

	return nil
}

func (im *IstioManager) reconcileVirtualServiceIfEnabled(ctx context.Context, redis *koncachev1alpha1.Redis, vsConfig *koncachev1alpha1.RedisIstioVirtualService) error {
	log := logf.FromContext(ctx)

	if vsConfig == nil {
		log.Info("VirtualService config is nil", "redis", redis.Name)
		return nil
	}

	enabled := getBoolValue(vsConfig.Enabled, true)
	log.Info("VirtualService enabled check", "redis", redis.Name, "enabled", enabled, "config", vsConfig.Enabled)

	if enabled {
		log.Info("Reconciling VirtualService", "redis", redis.Name)
		return im.reconcileVirtualService(ctx, redis, vsConfig)
	}
	return nil
}

func (im *IstioManager) reconcileDestinationRuleIfEnabled(ctx context.Context, redis *koncachev1alpha1.Redis, drConfig *koncachev1alpha1.RedisIstioDestinationRule) error {
	if drConfig != nil && getBoolValue(drConfig.Enabled, true) {
		return im.reconcileDestinationRule(ctx, redis, drConfig)
	}
	return nil
}

func (im *IstioManager) reconcileServiceEntryIfEnabled(ctx context.Context, redis *koncachev1alpha1.Redis, seConfig *koncachev1alpha1.RedisIstioServiceEntry) error {
	if seConfig != nil && getBoolValue(seConfig.Enabled, false) {
		return im.reconcileServiceEntry(ctx, redis, seConfig)
	}
	return nil
}

func (im *IstioManager) reconcilePeerAuthenticationIfEnabled(ctx context.Context, redis *koncachev1alpha1.Redis, paConfig *koncachev1alpha1.RedisIstioPeerAuthentication) error {
	if paConfig != nil && getBoolValue(paConfig.Enabled, false) {
		return im.reconcilePeerAuthentication(ctx, redis, paConfig)
	}
	return nil
}

// GetSidecarAnnotations returns the annotations needed for Istio sidecar injection
func (im *IstioManager) GetSidecarAnnotations(redis *koncachev1alpha1.Redis) map[string]string {
	annotations := make(map[string]string)

	if redis.Spec.Networking == nil || redis.Spec.Networking.Istio == nil || !redis.Spec.Networking.Istio.Enabled {
		return annotations
	}

	istioConfig := redis.Spec.Networking.Istio
	sidecarConfig := istioConfig.SidecarInjection

	if sidecarConfig == nil {
		// Default to enabled if not specified
		annotations["sidecar.istio.io/inject"] = "true"
		return annotations
	}

	// Set sidecar injection
	if getBoolValue(sidecarConfig.Enabled, true) {
		annotations[SidecarInjectAnnotation] = "true"
	} else {
		annotations[SidecarInjectAnnotation] = "false"
	}

	// Set custom template if specified
	if sidecarConfig.Template != "" {
		annotations[SidecarTemplateAnnotation] = sidecarConfig.Template
	}

	// Set proxy metadata (simplified example)
	for metaKey, metaValue := range sidecarConfig.ProxyMetadata {
		annotations[fmt.Sprintf("sidecar.istio.io/%s", metaKey)] = metaValue
	}

	return annotations
}

// reconcileVirtualService creates or updates a VirtualService for the Redis instance
func (im *IstioManager) reconcileVirtualService(ctx context.Context, redis *koncachev1alpha1.Redis, vsConfig *koncachev1alpha1.RedisIstioVirtualService) error {
	log := logf.FromContext(ctx)
	log.Info("Reconciling VirtualService", "redis", redis.Name, "namespace", redis.Namespace)

	virtualService := &unstructured.Unstructured{}
	virtualService.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   VirtualServiceGVR.Group,
		Version: VirtualServiceGVR.Version,
		Kind:    "VirtualService",
	})
	virtualService.SetName(redis.Name + "-vs")
	virtualService.SetNamespace(redis.Namespace)

	_, err := controllerutil.CreateOrUpdate(ctx, im.Client, virtualService, func() error {
		// Set owner reference
		if err := controllerutil.SetControllerReference(redis, virtualService, im.Scheme); err != nil {
			return err
		}

		// Build spec using SetNestedField to avoid deep copy issues
		hosts := getVirtualServiceHosts(redis, vsConfig)
		hostInterfaces := make([]interface{}, len(hosts))
		for i, host := range hosts {
			hostInterfaces[i] = host
		}

		// Set hosts
		if err := unstructured.SetNestedField(virtualService.Object, hostInterfaces, "spec", "hosts"); err != nil {
			return err
		}

		// Add gateways if specified
		if len(vsConfig.Gateways) > 0 {
			gatewayInterfaces := make([]interface{}, len(vsConfig.Gateways))
			for i, gateway := range vsConfig.Gateways {
				gatewayInterfaces[i] = gateway
			}
			if err := unstructured.SetNestedField(virtualService.Object, gatewayInterfaces, "spec", "gateways"); err != nil {
				return err
			}
		}

		// Build TCP routing structure step by step
		tcpRoutes := []interface{}{
			map[string]interface{}{
				"match": []interface{}{
					map[string]interface{}{
						"port": int64(redis.Spec.Port),
					},
				},
				"route": []interface{}{
					map[string]interface{}{
						"destination": map[string]interface{}{
							"host": redis.Name,
							"port": map[string]interface{}{
								"number": int64(redis.Spec.Port),
							},
						},
					},
				},
			},
		}

		// Add traffic policies if configured
		if vsConfig.TrafficPolicy != nil && vsConfig.Timeout != nil {
			if tcpRoute, ok := tcpRoutes[0].(map[string]interface{}); ok {
				tcpRoute["timeout"] = vsConfig.Timeout.Duration.String()
			}
		}

		// Set TCP routes
		return unstructured.SetNestedField(virtualService.Object, tcpRoutes, "spec", "tcp")
	})

	if err != nil {
		istioLog.Error(err, "Failed to reconcile VirtualService", "name", virtualService.GetName())
		return err
	}

	istioLog.Info("Successfully reconciled VirtualService", "name", virtualService.GetName())
	return nil
}

// reconcilePeerAuthentication creates or updates a PeerAuthentication for the Redis instance
func (im *IstioManager) reconcilePeerAuthentication(ctx context.Context, redis *koncachev1alpha1.Redis, paConfig *koncachev1alpha1.RedisIstioPeerAuthentication) error {
	peerAuth := &unstructured.Unstructured{}
	peerAuth.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   PeerAuthenticationGVR.Group,
		Version: PeerAuthenticationGVR.Version,
		Kind:    "PeerAuthentication",
	})
	peerAuth.SetName(redis.Name + "-pa")
	peerAuth.SetNamespace(redis.Namespace)

	_, err := controllerutil.CreateOrUpdate(ctx, im.Client, peerAuth, func() error {
		// Set owner reference
		if err := controllerutil.SetControllerReference(redis, peerAuth, im.Scheme); err != nil {
			return err
		}

		// Build spec
		spec := map[string]interface{}{
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{
					"app": redis.Name,
				},
			},
		}

		// Add mutual TLS configuration
		if paConfig.MutualTLS != nil {
			mtls := map[string]interface{}{
				"mode": getStringValue(paConfig.MutualTLS.Mode, "STRICT"),
			}
			spec["mtls"] = mtls
		}

		return unstructured.SetNestedMap(peerAuth.Object, spec, "spec")
	})

	if err != nil {
		istioLog.Error(err, "Failed to reconcile PeerAuthentication", "name", peerAuth.GetName())
		return err
	}

	istioLog.Info("Successfully reconciled PeerAuthentication", "name", peerAuth.GetName())
	return nil
}

// cleanupIstioResources removes all Istio resources for a Redis instance
func (im *IstioManager) cleanupIstioResources(ctx context.Context, redis *koncachev1alpha1.Redis) error {
	resources := []struct {
		name string
		gvk  schema.GroupVersionKind
	}{
		{redis.Name + "-vs", schema.GroupVersionKind{Group: VirtualServiceGVR.Group, Version: VirtualServiceGVR.Version, Kind: "VirtualService"}},
		{redis.Name + "-dr", schema.GroupVersionKind{Group: DestinationRuleGVR.Group, Version: DestinationRuleGVR.Version, Kind: "DestinationRule"}},
		{redis.Name + "-se", schema.GroupVersionKind{Group: ServiceEntryGVR.Group, Version: ServiceEntryGVR.Version, Kind: "ServiceEntry"}},
		{redis.Name + "-pa", schema.GroupVersionKind{Group: PeerAuthenticationGVR.Group, Version: PeerAuthenticationGVR.Version, Kind: "PeerAuthentication"}},
	}

	for _, resource := range resources {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(resource.gvk)
		obj.SetName(resource.name)
		obj.SetNamespace(redis.Namespace)

		err := im.Delete(ctx, obj)
		if err != nil && !errors.IsNotFound(err) {
			istioLog.Error(err, "Failed to delete Istio resource", "kind", resource.gvk.Kind, "name", resource.name)
			return err
		}
	}

	return nil
}

// Helper functions

func getBoolValue(ptr *bool, defaultValue bool) bool {
	if ptr != nil {
		return *ptr
	}
	return defaultValue
}

func getStringValue(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

func getVirtualServiceHosts(redis *koncachev1alpha1.Redis, vsConfig *koncachev1alpha1.RedisIstioVirtualService) []string {
	if len(vsConfig.Hosts) > 0 {
		return vsConfig.Hosts
	}
	// Default to service name
	return []string{redis.Name}
}

func getServiceEntryHosts(redis *koncachev1alpha1.Redis, seConfig *koncachev1alpha1.RedisIstioServiceEntry) []string {
	if len(seConfig.Hosts) > 0 {
		return seConfig.Hosts
	}
	// Default to service name
	return []string{redis.Name}
}
