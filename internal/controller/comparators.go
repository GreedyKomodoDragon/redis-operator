package controller

import (
	"crypto/sha256"
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

// EqualResourceLists compares two ResourceList objects
func EqualResourceLists(a, b corev1.ResourceList) bool {
	if len(a) != len(b) {
		return false
	}

	for key, aVal := range a {
		bVal, exists := b[key]
		if !exists || !aVal.Equal(bVal) {
			return false
		}
	}

	return true
}

// EqualStringMaps compares two string maps for equality
func EqualStringMaps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// EqualTolerations compares two toleration slices for equality
func EqualTolerations(a, b []corev1.Toleration) bool {
	if len(a) != len(b) {
		return false
	}

	// Simple comparison - check if each toleration in a exists in b
	for _, aTol := range a {
		found := false
		for _, bTol := range b {
			if TolerationsEqual(aTol, bTol) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// TolerationsEqual compares two individual tolerations
func TolerationsEqual(a, b corev1.Toleration) bool {
	if a.Key != b.Key || a.Operator != b.Operator || a.Effect != b.Effect || a.Value != b.Value {
		return false
	}

	// Compare TolerationSeconds
	if a.TolerationSeconds == nil && b.TolerationSeconds == nil {
		return true
	}
	if a.TolerationSeconds != nil && b.TolerationSeconds != nil {
		return *a.TolerationSeconds == *b.TolerationSeconds
	}

	return false
}

// ComputeStringHash computes a SHA256 hash of a string
// This is a more efficient way to hash single configuration strings
func ComputeStringHash(content string) string {
	if content == "" {
		return ""
	}

	hasher := sha256.New()
	hasher.Write([]byte(content))
	return fmt.Sprintf("%x", hasher.Sum(nil))[:16] // Use first 16 chars for brevity
}
