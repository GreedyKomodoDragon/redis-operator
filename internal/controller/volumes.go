package controller

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	koncachev1alpha1 "github.com/GreedyKomodoDragon/redis-operator/api/v1alpha1"
)

// BuildVolumeClaimTemplate builds the volume claim template for StatefulSets
func BuildVolumeClaimTemplate(redis *koncachev1alpha1.Redis) corev1.PersistentVolumeClaim {
	storageSize := redis.Spec.Storage.Size
	if storageSize.IsZero() {
		storageSize = resource.MustParse("1Gi")
	}

	accessModes := redis.Spec.Storage.AccessModes
	if len(accessModes) == 0 {
		accessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
	}

	return corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: RedisDataVolumeName,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      accessModes,
			StorageClassName: redis.Spec.Storage.StorageClassName,
			VolumeMode:       redis.Spec.Storage.VolumeMode,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: storageSize,
				},
			},
		},
	}
}

// BuildConfigMapVolume creates a ConfigMap volume for Redis configuration
func BuildConfigMapVolume(redisName string) corev1.Volume {
	return corev1.Volume{
		Name: RedisConfigVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: redisName + "-config",
				},
			},
		},
	}
}

// BuildTLSVolumeMounts builds volume mounts for TLS certificates
func BuildTLSVolumeMounts(redis *koncachev1alpha1.Redis) []corev1.VolumeMount {
	var volumeMounts []corev1.VolumeMount

	if IsTLSEnabled(redis) {
		// TLS certificate volume mount
		if redis.Spec.Security.TLS.CertSecret != "" {
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      TLSCertsVolumeName,
				MountPath: "/etc/redis/tls",
				ReadOnly:  true,
			})
		}

		// CA certificate volume mount (if different from cert secret)
		if redis.Spec.Security.TLS.CASecret != "" &&
			redis.Spec.Security.TLS.CASecret != redis.Spec.Security.TLS.CertSecret {
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      TLSCAVolumeName,
				MountPath: "/etc/redis/tls-ca",
				ReadOnly:  true,
			})
		}
	}

	return volumeMounts
}

// BuildTLSVolumes builds volumes for TLS certificates
func BuildTLSVolumes(redis *koncachev1alpha1.Redis) []corev1.Volume {
	var volumes []corev1.Volume

	if IsTLSEnabled(redis) {
		// TLS certificate volume
		if redis.Spec.Security.TLS.CertSecret != "" {
			items := []corev1.KeyToPath{
				{
					Key:  TLSCertKey,
					Path: TLSCertKey,
				},
				{
					Key:  TLSKeyKey,
					Path: TLSKeyKey,
				},
			}

			// If CA secret is the same as cert secret, include ca.crt in the same volume
			if redis.Spec.Security.TLS.CASecret == redis.Spec.Security.TLS.CertSecret {
				items = append(items, corev1.KeyToPath{
					Key:  TLSCAKey,
					Path: TLSCAKey,
				})
			}

			volumes = append(volumes, corev1.Volume{
				Name: TLSCertsVolumeName,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: redis.Spec.Security.TLS.CertSecret,
						Items:      items,
					},
				},
			})
		}

		// CA certificate volume (if different from cert secret)
		if redis.Spec.Security.TLS.CASecret != "" &&
			redis.Spec.Security.TLS.CASecret != redis.Spec.Security.TLS.CertSecret {
			volumes = append(volumes, corev1.Volume{
				Name: TLSCAVolumeName,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: redis.Spec.Security.TLS.CASecret,
						Items: []corev1.KeyToPath{
							{
								Key:  TLSCAKey,
								Path: TLSCAKey,
							},
						},
					},
				},
			})
		}
	}

	return volumes
}

// buildVolumes builds all volumes needed for the Redis pod including config and TLS
func buildVolumes(redis *koncachev1alpha1.Redis) []corev1.Volume {
	volumes := []corev1.Volume{
		{
			Name: RedisConfigVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: redis.Name + "-config",
					},
				},
			},
		},
	}

	// Add TLS volumes if TLS is enabled
	tlsVolumes := BuildTLSVolumes(redis)
	if len(tlsVolumes) > 0 {
		volumes = append(volumes, tlsVolumes...)
	}

	return volumes
}

// BuildBackupVolumes builds volumes for Redis backup jobs
func BuildBackupVolumes(redis *koncachev1alpha1.Redis) []corev1.Volume {
	// For StatefulSets, the backup volume is created automatically from VolumeClaimTemplate
	// We only need to include other volumes like TLS volumes
	var volumes []corev1.Volume

	// Add TLS volumes if TLS is enabled
	tlsVolumes := BuildTLSVolumes(redis)
	if len(tlsVolumes) > 0 {
		volumes = append(volumes, tlsVolumes...)
	}

	return volumes
}
