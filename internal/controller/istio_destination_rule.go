package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	koncachev1alpha1 "github.com/GreedyKomodoDragon/redis-operator/api/v1alpha1"
)

// destinationRuleBuilder helps build DestinationRule resources
type destinationRuleBuilder struct {
	redis    *koncachev1alpha1.Redis
	drConfig *koncachev1alpha1.RedisIstioDestinationRule
	spec     map[string]interface{}
}

// newDestinationRuleBuilder creates a new destination rule builder
func newDestinationRuleBuilder(redis *koncachev1alpha1.Redis, drConfig *koncachev1alpha1.RedisIstioDestinationRule) *destinationRuleBuilder {
	return &destinationRuleBuilder{
		redis:    redis,
		drConfig: drConfig,
		spec:     make(map[string]interface{}),
	}
}

// build constructs the DestinationRule spec
func (drb *destinationRuleBuilder) build() map[string]interface{} {
	drb.spec["host"] = drb.redis.Name
	drb.addTrafficPolicy()
	drb.addSubsets()
	return drb.spec
}

// addTrafficPolicy adds traffic policy configuration
func (drb *destinationRuleBuilder) addTrafficPolicy() {
	if drb.drConfig.TrafficPolicy != nil {
		trafficPolicy := buildTrafficPolicy(drb.drConfig.TrafficPolicy)
		if len(trafficPolicy) > 0 {
			drb.spec["trafficPolicy"] = trafficPolicy
		}
	}
}

// addSubsets adds subset configuration
func (drb *destinationRuleBuilder) addSubsets() {
	if len(drb.drConfig.Subsets) == 0 {
		return
	}

	subsets := make([]map[string]interface{}, 0, len(drb.drConfig.Subsets))
	for _, subset := range drb.drConfig.Subsets {
		subsetMap := map[string]interface{}{
			"name":   subset.Name,
			"labels": subset.Labels,
		}
		if subset.TrafficPolicy != nil {
			trafficPolicy := buildTrafficPolicy(subset.TrafficPolicy)
			if len(trafficPolicy) > 0 {
				subsetMap["trafficPolicy"] = trafficPolicy
			}
		}
		subsets = append(subsets, subsetMap)
	}
	drb.spec["subsets"] = subsets
}

// reconcileDestinationRule creates or updates a DestinationRule for the Redis instance
func (im *IstioManager) reconcileDestinationRule(ctx context.Context, redis *koncachev1alpha1.Redis, drConfig *koncachev1alpha1.RedisIstioDestinationRule) error {
	destinationRule := &unstructured.Unstructured{}
	destinationRule.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   DestinationRuleGVR.Group,
		Version: DestinationRuleGVR.Version,
		Kind:    "DestinationRule",
	})
	destinationRule.SetName(redis.Name + "-dr")
	destinationRule.SetNamespace(redis.Namespace)

	_, err := controllerutil.CreateOrUpdate(ctx, im.Client, destinationRule, func() error {
		// Set owner reference
		if err := controllerutil.SetControllerReference(redis, destinationRule, im.Scheme); err != nil {
			return err
		}

		// Set the host field directly
		if err := unstructured.SetNestedField(destinationRule.Object, redis.Name, "spec", "host"); err != nil {
			return err
		}

		// Add traffic policy if configured
		if drConfig.TrafficPolicy != nil {
			trafficPolicy := buildTrafficPolicy(drConfig.TrafficPolicy)
			if len(trafficPolicy) > 0 {
				if err := unstructured.SetNestedField(destinationRule.Object, trafficPolicy, "spec", "trafficPolicy"); err != nil {
					return err
				}
			}
		}

		// Add subsets if configured
		if len(drConfig.Subsets) > 0 {
			subsets := make([]interface{}, 0, len(drConfig.Subsets))
			for _, subset := range drConfig.Subsets {
				subsetMap := map[string]interface{}{
					"name": subset.Name,
				}

				// Convert labels map to ensure string values
				if len(subset.Labels) > 0 {
					labels := make(map[string]interface{})
					for k, v := range subset.Labels {
						labels[k] = v
					}
					subsetMap["labels"] = labels
				}

				if subset.TrafficPolicy != nil {
					trafficPolicy := buildTrafficPolicy(subset.TrafficPolicy)
					if len(trafficPolicy) > 0 {
						subsetMap["trafficPolicy"] = trafficPolicy
					}
				}
				subsets = append(subsets, subsetMap)
			}
			if err := unstructured.SetNestedField(destinationRule.Object, subsets, "spec", "subsets"); err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		istioLog.Error(err, "Failed to reconcile DestinationRule", "name", destinationRule.GetName())
		return err
	}

	istioLog.Info("Successfully reconciled DestinationRule", "name", destinationRule.GetName())
	return nil
}
