package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	koncachev1alpha1 "github.com/GreedyKomodoDragon/redis-operator/api/v1alpha1"
)

// serviceEntryBuilder helps build ServiceEntry resources
type serviceEntryBuilder struct {
	redis    *koncachev1alpha1.Redis
	seConfig *koncachev1alpha1.RedisIstioServiceEntry
	spec     map[string]interface{}
}

// newServiceEntryBuilder creates a new service entry builder
func newServiceEntryBuilder(redis *koncachev1alpha1.Redis, seConfig *koncachev1alpha1.RedisIstioServiceEntry) *serviceEntryBuilder {
	return &serviceEntryBuilder{
		redis:    redis,
		seConfig: seConfig,
		spec:     make(map[string]interface{}),
	}
}

// build constructs the ServiceEntry spec
func (seb *serviceEntryBuilder) build() map[string]interface{} {
	seb.spec["hosts"] = seb.getHosts()
	seb.spec["location"] = getStringValue(seb.seConfig.Location, MeshExternalLocation)
	seb.spec["resolution"] = getStringValue(seb.seConfig.Resolution, DNSResolution)
	seb.addPorts()
	return seb.spec
}

// getHosts returns the hosts for the ServiceEntry
func (seb *serviceEntryBuilder) getHosts() []string {
	if len(seb.seConfig.Hosts) > 0 {
		return seb.seConfig.Hosts
	}
	return []string{seb.redis.Name}
}

// addPorts adds port configuration to the ServiceEntry
func (seb *serviceEntryBuilder) addPorts() {
	if len(seb.seConfig.Ports) == 0 {
		return
	}

	ports := make([]map[string]interface{}, 0, len(seb.seConfig.Ports))
	for _, port := range seb.seConfig.Ports {
		portMap := map[string]interface{}{
			"number":   port.Number,
			"protocol": port.Protocol,
		}
		if port.Name != "" {
			portMap["name"] = port.Name
		}
		if port.TargetPort != 0 {
			portMap["targetPort"] = port.TargetPort
		}
		ports = append(ports, portMap)
	}
	seb.spec["ports"] = ports
}

// reconcileServiceEntry creates or updates a ServiceEntry for the Redis instance
func (im *IstioManager) reconcileServiceEntry(ctx context.Context, redis *koncachev1alpha1.Redis, seConfig *koncachev1alpha1.RedisIstioServiceEntry) error {
	serviceEntry := &unstructured.Unstructured{}
	serviceEntry.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   ServiceEntryGVR.Group,
		Version: ServiceEntryGVR.Version,
		Kind:    "ServiceEntry",
	})
	serviceEntry.SetName(redis.Name + "-se")
	serviceEntry.SetNamespace(redis.Namespace)

	_, err := controllerutil.CreateOrUpdate(ctx, im.Client, serviceEntry, func() error {
		// Set owner reference
		if err := controllerutil.SetControllerReference(redis, serviceEntry, im.Scheme); err != nil {
			return err
		}

		// Build spec using the builder
		spec := newServiceEntryBuilder(redis, seConfig).build()
		return unstructured.SetNestedMap(serviceEntry.Object, spec, "spec")
	})

	if err != nil {
		istioLog.Error(err, "Failed to reconcile ServiceEntry", "name", serviceEntry.GetName())
		return err
	}

	istioLog.Info("Successfully reconciled ServiceEntry", "name", serviceEntry.GetName())
	return nil
}
