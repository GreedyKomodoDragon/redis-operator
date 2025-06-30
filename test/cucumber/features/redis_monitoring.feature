Feature: Redis Monitoring
  As an operations engineer
  I want to monitor Redis instances
  So that I can observe Redis performance and health

  @minikube @integration @monitoring
  Scenario: Deploy Redis with monitoring enabled
    Given minikube is running
    And the Redis operator is deployed
    And Prometheus operator is installed
    When I create a Redis resource with monitoring:
      | name             | test-redis-monitoring |
      | namespace        | default               |
      | mode             | standalone            |
      | monitoringEnabled| true                  |
      | exporterImage    | oliver006/redis_exporter:latest |
    Then the Redis StatefulSet should be created
    And the Redis StatefulSet should have a monitoring sidecar
    And the ServiceMonitor should be created
    And the monitoring sidecar should export Redis metrics
    And Prometheus should scrape Redis metrics

  @minikube @integration @monitoring  
  Scenario: Redis monitoring with custom metrics port
    Given minikube is running
    And the Redis operator is deployed
    And Prometheus operator is installed
    When I create a Redis resource with custom monitoring port:
      | name             | test-redis-metrics-port |
      | namespace        | default                 |
      | mode             | standalone              |
      | monitoringEnabled| true                    |
      | metricsPort      | 9121                    |
    Then the Redis StatefulSet should be created
    And the monitoring sidecar should use port 9121
    And the ServiceMonitor should target port 9121
    And metrics should be available on the custom port

  @minikube @integration @monitoring
  Scenario: Redis monitoring without Prometheus operator
    Given minikube is running
    And the Redis operator is deployed
    When I create a Redis resource with monitoring but no ServiceMonitor:
      | name             | test-redis-monitoring-no-sm |
      | namespace        | default                     |
      | mode             | standalone                  |
      | monitoringEnabled| true                        |
      | serviceMonitor   | false                       |
    Then the Redis StatefulSet should be created
    And the Redis StatefulSet should have a monitoring sidecar
    And the monitoring sidecar should export Redis metrics
    And no ServiceMonitor should be created

  @minikube @integration @monitoring
  Scenario: Disable monitoring for existing Redis instance
    Given minikube is running
    And the Redis operator is deployed
    And a Redis instance with monitoring enabled is running
    When I disable monitoring for the Redis instance
    Then the monitoring sidecar should be removed
    And the ServiceMonitor should be deleted
    And the Redis StatefulSet should be updated
