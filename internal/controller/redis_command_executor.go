/*
Package controller provides Redis command execution interfaces and implementations.

This file contains the RedisCommandExecutor interface and its implementations for
executing Redis commands on pods in a Kubernetes cluster. It provides both real
and mock implementations to support production use and testing.
*/
package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

// RedisCommandExecutor interface for executing Redis commands on Redis pods.
// This abstraction allows for different implementations such as real Redis
// command execution and mock implementations for testing.
type RedisCommandExecutor interface {
	// ExecuteRedisCommand executes a Redis command on the specified pod.
	// It returns an error if the command execution fails.
	ExecuteRedisCommand(ctx context.Context, pod *corev1.Pod, command string) error
}

// RealRedisCommandExecutor implements actual Redis command execution using
// the go-redis client library. It delegates to the StandaloneController's
// executeRedisCommand method which handles Redis client creation, authentication,
// and TLS configuration.
type RealRedisCommandExecutor struct {
	controller *StandaloneController
}

// NewRealRedisCommandExecutor creates a new RealRedisCommandExecutor with
// the provided StandaloneController.
func NewRealRedisCommandExecutor(controller *StandaloneController) *RealRedisCommandExecutor {
	return &RealRedisCommandExecutor{
		controller: controller,
	}
}

// ExecuteRedisCommand executes a Redis command on the specified pod by
// delegating to the controller's executeRedisCommand method.
func (r *RealRedisCommandExecutor) ExecuteRedisCommand(ctx context.Context, pod *corev1.Pod, command string) error {
	return r.controller.executeRedisCommand(ctx, pod, command)
}

// MockRedisCommandExecutor implements mock Redis command execution for testing.
// It records all commands that would be executed without actually connecting
// to Redis, allowing for unit testing of Redis replication logic.
type MockRedisCommandExecutor struct {
	Commands []MockRedisCommand
}

// MockRedisCommand represents a Redis command that was recorded by the mock executor.
type MockRedisCommand struct {
	PodName      string // Name of the pod the command was executed on
	Command      string // The Redis command that was executed
	ShouldError  bool   // Whether this command should return an error
	ErrorMessage string // Error message to return if ShouldError is true
}

// NewMockRedisCommandExecutor creates a new MockRedisCommandExecutor.
func NewMockRedisCommandExecutor() *MockRedisCommandExecutor {
	return &MockRedisCommandExecutor{
		Commands: make([]MockRedisCommand, 0),
	}
}

// ExecuteRedisCommand records the command execution without actually executing it.
// This is useful for testing Redis replication configuration logic.
func (m *MockRedisCommandExecutor) ExecuteRedisCommand(ctx context.Context, pod *corev1.Pod, command string) error {
	mockCmd := MockRedisCommand{
		PodName: pod.Name,
		Command: command,
	}
	m.Commands = append(m.Commands, mockCmd)

	if mockCmd.ShouldError {
		return fmt.Errorf("mock error: %s", mockCmd.ErrorMessage)
	}
	return nil
}

// Reset clears all recorded commands. Useful for test cleanup.
func (m *MockRedisCommandExecutor) Reset() {
	m.Commands = m.Commands[:0]
}

// GetCommandsForPod returns all commands that were executed on the specified pod.
func (m *MockRedisCommandExecutor) GetCommandsForPod(podName string) []MockRedisCommand {
	var podCommands []MockRedisCommand
	for _, cmd := range m.Commands {
		if cmd.PodName == podName {
			podCommands = append(podCommands, cmd)
		}
	}
	return podCommands
}
