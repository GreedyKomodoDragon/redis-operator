Feature: Redis Standalone Deployment
  As a Kubernetes operator user
  I want to deploy standalone Redis instances
  So that I can use Redis in my applications

  @minikube @integration
  Scenario: Deploy a basic standalone Redis instance
    Given minikube is running
    And the Redis operator is deployed
    When I create a Redis resource with the following configuration:
      | name      | test-redis    |
      | namespace | default       |
      | mode      | standalone    |
      | version   | 7.2           |
      | image     | redis:7.2-alpine |
      | port      | 6379          |
    Then the Redis StatefulSet should be created
    And the Redis Service should be created
    And the Redis ConfigMap should be created
    And the Redis pod should be running
    And I should be able to connect to Redis

  @minikube @integration
  Scenario: Deploy Redis with custom configuration
    Given minikube is running
    And the Redis operator is deployed
    When I create a Redis resource with custom config:
      | name           | test-redis-config |
      | namespace      | default           |
      | mode           | standalone        |
      | maxMemory      | 256mb             |
      | maxMemoryPolicy| allkeys-lru       |
      | databases      | 8                 |
    Then the Redis StatefulSet should be created
    And the Redis ConfigMap should contain the custom configuration
    And the Redis pod should be running
    And Redis should have the custom configuration applied

  @minikube @integration
  Scenario: Deploy Redis with persistent storage
    Given minikube is running
    And the Redis operator is deployed
    When I create a Redis resource with persistent storage:
      | name         | test-redis-storage |
      | namespace    | default            |
      | mode         | standalone         |
      | storageSize  | 1Gi                |
      | storageClass | standard           |
    Then the Redis StatefulSet should be created
    And the Redis PVC should be created
    And the Redis pod should be running
    And Redis data should persist after pod restart

  @minikube @integration
  Scenario: Update Redis configuration
    Given minikube is running
    And the Redis operator is deployed
    And a Redis instance "test-redis-update" is running
    When I update the Redis maxMemory from "256mb" to "512mb"
    Then the Redis ConfigMap should be updated
    And the Redis pod should be restarted
    And Redis should have the new configuration applied

  @minikube @integration
  Scenario: Delete Redis instance
    Given minikube is running
    And the Redis operator is deployed
    And a Redis instance "test-redis-delete" is running
    When I delete the Redis resource
    Then the Redis StatefulSet should be deleted
    And the Redis Service should be deleted
    And the Redis ConfigMap should be deleted
    And the Redis PVC should remain (for data safety)
