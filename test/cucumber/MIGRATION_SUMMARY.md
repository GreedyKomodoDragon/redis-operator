# ServiceMonitor to PodMonitor Migration - Test Updates

## Summary

Successfully migrated all Cucumber integration tests from ServiceMonitor to PodMonitor to align with the Redis Operator's migration to using PodMonitors for Prometheus monitoring.

## Files Changed

### 1. `/test/cucumber/steps.go`

**Function Renames:**
- `serviceMonitorShouldBeCreated()` → `podMonitorShouldBeCreated()`
- `serviceMonitorShouldTargetPort()` → `podMonitorShouldTargetPort()`
- `noServiceMonitorShouldBeCreated()` → `noPodMonitorShouldBeCreated()`
- `serviceMonitorShouldBeDeleted()` → `podMonitorShouldBeDeleted()`
- `createRedisResourceWithMonitoringNoServiceMonitor()` → `createRedisResourceWithMonitoringNoPodMonitor()`

**Step Definition Updates:**
- `^the ServiceMonitor should be created$` → `^the PodMonitor should be created$`
- `^the ServiceMonitor should target port (\d+)$` → `^the PodMonitor should target port (\d+)$`
- `^no ServiceMonitor should be created$` → `^no PodMonitor should be created$`
- `^the ServiceMonitor should be deleted$` → `^the PodMonitor should be deleted$`
- `^I create a Redis resource with monitoring but no ServiceMonitor:$` → `^I create a Redis resource with monitoring but no PodMonitor:$`

**Resource Changes:**
- All `schema.GroupVersionResource` definitions updated from `servicemonitors` to `podmonitors`
- Dynamic client operations updated to work with PodMonitor CRDs
- Prometheus operator validation updated to check for PodMonitor CRD instead of ServiceMonitor

**Comment Updates:**
- "Setup dynamic client for ServiceMonitor operations" → "Setup dynamic client for PodMonitor operations"
- Error messages updated to reference PodMonitor instead of ServiceMonitor
- Function documentation updated

### 2. `/test/cucumber/features/redis_monitoring.feature`

**Scenario Step Updates:**
- `And the ServiceMonitor should be created` → `And the PodMonitor should be created`
- `And the ServiceMonitor should target port 9121` → `And the PodMonitor should target port 9121`
- `When I create a Redis resource with monitoring but no ServiceMonitor:` → `When I create a Redis resource with monitoring but no PodMonitor:`
- `And no ServiceMonitor should be created` → `And no PodMonitor should be created`
- `And the ServiceMonitor should be deleted` → `And the PodMonitor should be deleted`

**Table Data Updates:**
- `serviceMonitor | false` → `podMonitor | false`

## Key Technical Changes

1. **API Resource Updates**: All Kubernetes API calls now target the `podmonitors` resource instead of `servicemonitors`

2. **CRD Validation**: The Prometheus operator installation check now validates that the `PodMonitor` CRD is available instead of `ServiceMonitor`

3. **Function Logic**: All functions that previously worked with ServiceMonitor objects now work with PodMonitor objects, maintaining the same validation and waiting logic

4. **Naming Conventions**: Updated naming to follow the `-monitor` suffix convention for PodMonitor resources (same as was used for ServiceMonitors)

## Compatibility

- All existing test scenarios continue to work with the same logic flow
- The migration is transparent to the actual Redis Operator functionality being tested
- Test timeout and polling intervals remain unchanged
- Error handling and retry logic preserved

## Validation

- All files compile successfully with `go build`
- No remaining references to ServiceMonitor in the test codebase
- All PodMonitor references are properly implemented and consistent
- Feature file scenarios maintain their original intent and coverage

The migration ensures that the integration tests accurately reflect the Redis Operator's current architecture using PodMonitors for Prometheus monitoring integration.
