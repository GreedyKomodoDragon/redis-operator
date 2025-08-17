package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"

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

	// Create structured VirtualService object
	virtualService := im.buildVirtualService(redis, vsConfig)

	// Convert to unstructured for controller-runtime compatibility
	unstructuredVS := im.createUnstructuredVS(virtualService)

	_, err := controllerutil.CreateOrUpdate(ctx, im.Client, unstructuredVS, func() error {
		return im.updateVirtualServiceSpec(redis, virtualService, vsConfig, unstructuredVS)
	})

	if err != nil {
		istioLog.Error(err, "Failed to reconcile VirtualService", "name", unstructuredVS.GetName())
		return err
	}

	istioLog.Info("Successfully reconciled VirtualService", "name", unstructuredVS.GetName())
	return nil
}

// buildVirtualService creates a structured VirtualService object
func (im *IstioManager) buildVirtualService(redis *koncachev1alpha1.Redis, vsConfig *koncachev1alpha1.RedisIstioVirtualService) *VirtualService {
	virtualService := &VirtualService{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "networking.istio.io/v1beta1",
			Kind:       "VirtualService",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      redis.Name + "-vs",
			Namespace: redis.Namespace,
		},
		Spec: VirtualServiceSpec{
			Hosts: getVirtualServiceHosts(redis, vsConfig),
		},
	}

	// Add gateways if specified
	if len(vsConfig.Gateways) > 0 {
		virtualService.Spec.Gateways = vsConfig.Gateways
	}

	// Create TCP route for Redis port
	tcpRoute := im.buildTCPRoute(redis)
	virtualService.Spec.TCP = []TCPRoute{tcpRoute}

	return virtualService
}

// buildTCPRoute creates a TCP route for the Redis service
func (im *IstioManager) buildTCPRoute(redis *koncachev1alpha1.Redis) TCPRoute {
	port := uint32(redis.Spec.Port)
	return TCPRoute{
		Match: []L4MatchAttributes{
			{
				Port: &port,
			},
		},
		Route: []RouteDestination{
			{
				Destination: &Destination{
					Host: redis.Name,
					Port: &PortSelector{
						Number: &port,
					},
				},
			},
		},
	}
}

// createUnstructuredVS creates an unstructured VirtualService object
func (im *IstioManager) createUnstructuredVS(vs *VirtualService) *unstructured.Unstructured {
	unstructuredVS := &unstructured.Unstructured{}
	unstructuredVS.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   VirtualServiceGVR.Group,
		Version: VirtualServiceGVR.Version,
		Kind:    "VirtualService",
	})
	unstructuredVS.SetName(vs.Name)
	unstructuredVS.SetNamespace(vs.Namespace)
	return unstructuredVS
}

// updateVirtualServiceSpec updates the VirtualService spec in the unstructured object
func (im *IstioManager) updateVirtualServiceSpec(redis *koncachev1alpha1.Redis, vs *VirtualService, vsConfig *koncachev1alpha1.RedisIstioVirtualService, unstructuredVS *unstructured.Unstructured) error {
	// Set owner reference
	if err := controllerutil.SetControllerReference(redis, unstructuredVS, im.Scheme); err != nil {
		return err
	}

	// Set hosts
	if err := im.setVSHosts(unstructuredVS, vs.Spec.Hosts); err != nil {
		return err
	}

	// Set gateways if present
	if err := im.setVSGateways(unstructuredVS, vs.Spec.Gateways); err != nil {
		return err
	}

	// Set TCP routes
	return im.setVSTCPRoutes(unstructuredVS, vs.Spec.TCP, vsConfig)
}

// setVSHosts sets the hosts field in the VirtualService spec
func (im *IstioManager) setVSHosts(unstructuredVS *unstructured.Unstructured, hosts []string) error {
	hostInterfaces := make([]interface{}, len(hosts))
	for i, host := range hosts {
		hostInterfaces[i] = host
	}
	return unstructured.SetNestedField(unstructuredVS.Object, hostInterfaces, "spec", "hosts")
}

// setVSGateways sets the gateways field in the VirtualService spec
func (im *IstioManager) setVSGateways(unstructuredVS *unstructured.Unstructured, gateways []string) error {
	if len(gateways) == 0 {
		return nil
	}
	gatewayInterfaces := make([]interface{}, len(gateways))
	for i, gateway := range gateways {
		gatewayInterfaces[i] = gateway
	}
	return unstructured.SetNestedField(unstructuredVS.Object, gatewayInterfaces, "spec", "gateways")
}

// setVSTCPRoutes sets the TCP routes field in the VirtualService spec
func (im *IstioManager) setVSTCPRoutes(unstructuredVS *unstructured.Unstructured, tcpRoutes []TCPRoute, vsConfig *koncachev1alpha1.RedisIstioVirtualService) error {
	tcpRoutesInterface := make([]interface{}, len(tcpRoutes))
	for i, tcpRoute := range tcpRoutes {
		tcpRouteMap := im.buildTCPRouteMap(tcpRoute, vsConfig)
		tcpRoutesInterface[i] = tcpRouteMap
	}
	return unstructured.SetNestedField(unstructuredVS.Object, tcpRoutesInterface, "spec", "tcp")
}

// buildTCPRouteMap builds a TCP route map from a structured TCPRoute
func (im *IstioManager) buildTCPRouteMap(tcpRoute TCPRoute, vsConfig *koncachev1alpha1.RedisIstioVirtualService) map[string]interface{} {
	// Determine timeout if configured
	var timeout *string
	if vsConfig.TrafficPolicy != nil && vsConfig.Timeout != nil {
		timeoutStr := vsConfig.Timeout.Duration.String()
		timeout = &timeoutStr
	}

	// Convert structured TCPRoute to unstructured representation
	unstructuredRoute := tcpRoute.ToUnstructured(timeout)

	// Convert to map[string]interface{} using the structured approach
	return unstructuredRoute.ToMap()
}

// ToYAML converts a VirtualService to YAML representation for debugging
func (vs *VirtualService) ToYAML() (string, error) {
	yamlBytes, err := yaml.Marshal(vs)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
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

// Debugging and helper functions

// ConvertVirtualServiceToYAML converts a VirtualService object to YAML for debugging
func (im *IstioManager) ConvertVirtualServiceToYAML(redis *koncachev1alpha1.Redis, vsConfig *koncachev1alpha1.RedisIstioVirtualService) (string, error) {
	virtualService := im.buildVirtualService(redis, vsConfig)
	yamlData, err := yaml.Marshal(virtualService)
	if err != nil {
		return "", err
	}
	return string(yamlData), nil
}

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
