package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	koncachev1alpha1 "github.com/GreedyKomodoDragon/redis-operator/api/v1alpha1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
)

const (
	testRedisImage       = "redis:7.2-alpine"
	testConfigHashKey    = "redis-operator/config-hash"
	testNamespace        = "default"
	testStorageClassName = "standard"
	testMaxMemoryPolicy  = "allkeys-lru"
	configMapSuffix      = "-config"
	redisConfigKey       = "redis.conf"
)

func getTestStorageClassNamePtr() *string {
	sc := testStorageClassName
	return &sc
}

func TestRedisControllerReconcileNonExistentResource(t *testing.T) {
	// Setup test scheme
	testScheme := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(testScheme))
	require.NoError(t, koncachev1alpha1.AddToScheme(testScheme))

	ctx := context.Background()

	// Create fake client without any Redis resources
	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		Build()

	// Create controller
	reconciler := &RedisReconciler{
		Client: fakeClient,
		Scheme: testScheme,
	}

	// Test reconciliation of non-existent resource
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "non-existent-redis",
			Namespace: testNamespace,
		},
	}

	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, int64(0), result.RequeueAfter.Nanoseconds())
}

func TestRedisControllerReconcileStandaloneMode(t *testing.T) {
	// Setup test scheme
	testScheme := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(testScheme))
	require.NoError(t, koncachev1alpha1.AddToScheme(testScheme))
	require.NoError(t, monitoringv1.AddToScheme(testScheme))

	ctx := context.Background()

	// Create a basic Redis resource
	redis := &koncachev1alpha1.Redis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-redis",
			Namespace: testNamespace,
		},
		Spec: koncachev1alpha1.RedisSpec{
			Mode:    koncachev1alpha1.RedisModeStandalone,
			Version: "7.2",
			Image:   testRedisImage,
			Port:    6379,
			Storage: koncachev1alpha1.RedisStorage{
				Size:             resource.MustParse("1Gi"),
				StorageClassName: getTestStorageClassNamePtr(),
				AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			},
			Config: koncachev1alpha1.RedisConfig{
				MaxMemory:       "256mb",
				MaxMemoryPolicy: testMaxMemoryPolicy,
			},
			ServiceType: corev1.ServiceTypeClusterIP,
		},
	}

	// Create fake client
	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(redis).
		Build()

	// Create controller
	reconciler := &RedisReconciler{
		Client: fakeClient,
		Scheme: testScheme,
	}

	// Test reconciliation
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      redis.Name,
			Namespace: redis.Namespace,
		},
	}

	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	// Should have a requeue delay for stable resources
	assert.Equal(t, int64(30000000000), result.RequeueAfter.Nanoseconds())

	// Verify ConfigMap was created
	configMap := &corev1.ConfigMap{}
	err = fakeClient.Get(ctx, types.NamespacedName{
		Name:      redis.Name + configMapSuffix,
		Namespace: redis.Namespace,
	}, configMap)
	require.NoError(t, err)
	assert.Equal(t, redis.Name+configMapSuffix, configMap.Name)
	assert.Contains(t, configMap.Data, redisConfigKey)

	// Verify Service was created
	service := &corev1.Service{}
	err = fakeClient.Get(ctx, types.NamespacedName{
		Name:      redis.Name,
		Namespace: redis.Namespace,
	}, service)
	require.NoError(t, err)
	assert.Equal(t, redis.Name, service.Name)
	assert.Equal(t, corev1.ServiceTypeClusterIP, service.Spec.Type)
}

func TestRedisControllerModeSelection(t *testing.T) {
	testScheme := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(testScheme))
	require.NoError(t, koncachev1alpha1.AddToScheme(testScheme))
	require.NoError(t, monitoringv1.AddToScheme(testScheme))

	ctx := context.Background()

	tests := []struct {
		name        string
		mode        koncachev1alpha1.RedisMode
		expectError bool
		expectReque bool
	}{
		{
			name:        "standalone mode",
			mode:        koncachev1alpha1.RedisModeStandalone,
			expectError: false,
		},
		{
			name:        "empty mode defaults to standalone",
			mode:        "",
			expectError: false,
		},
		{
			name:        "cluster mode not implemented",
			mode:        koncachev1alpha1.RedisModeCluster,
			expectError: false, // Should not error, just not implemented
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redis := &koncachev1alpha1.Redis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-redis-" + tt.name,
					Namespace: testNamespace,
				},
				Spec: koncachev1alpha1.RedisSpec{
					Mode:    tt.mode,
					Version: "7.2",
					Image:   testRedisImage,
					Port:    6379,
					Storage: koncachev1alpha1.RedisStorage{
						Size:             resource.MustParse("1Gi"),
						StorageClassName: getTestStorageClassNamePtr(),
						AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					},
					ServiceType: corev1.ServiceTypeClusterIP,
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(testScheme).
				WithObjects(redis).
				Build()

			reconciler := &RedisReconciler{
				Client: fakeClient,
				Scheme: testScheme,
			}

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      redis.Name,
					Namespace: redis.Namespace,
				},
			}

			_, err := reconciler.Reconcile(ctx, req)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			// Check for errors
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				// If no error occurred, the mode was successfully routed
				// We don't check specific requeue behavior as it depends on
				// the implementation details of each sub-controller
			}
		})
	}
}

func TestRedisControllerConfigMapCreation(t *testing.T) {
	testScheme := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(testScheme))
	require.NoError(t, koncachev1alpha1.AddToScheme(testScheme))
	require.NoError(t, monitoringv1.AddToScheme(testScheme))

	ctx := context.Background()

	redis := &koncachev1alpha1.Redis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-redis-config",
			Namespace: testNamespace,
		},
		Spec: koncachev1alpha1.RedisSpec{
			Mode:    "standalone",
			Version: "7.2",
			Image:   testRedisImage,
			Port:    6379,
			Config: koncachev1alpha1.RedisConfig{
				MaxMemory:       "256mb",
				MaxMemoryPolicy: "allkeys-lru",
				Databases:       16,
			},
			Storage: koncachev1alpha1.RedisStorage{
				Size:             resource.MustParse("1Gi"),
				StorageClassName: getTestStorageClassNamePtr(),
				AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			},
			ServiceType: corev1.ServiceTypeClusterIP,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(redis).
		Build()

	controller := NewStandaloneController(fakeClient, testScheme)

	// Generate Redis config
	redisConfig := BuildRedisConfig(redis)
	require.NotEmpty(t, redisConfig)

	// Test ConfigMap creation
	err := controller.reconcileConfigMap(ctx, redis, redisConfig)
	require.NoError(t, err)

	// Verify ConfigMap content
	configMap := &corev1.ConfigMap{}
	err = fakeClient.Get(ctx, types.NamespacedName{
		Name:      redis.Name + configMapSuffix,
		Namespace: redis.Namespace,
	}, configMap)
	require.NoError(t, err)

	assert.Equal(t, redis.Name+configMapSuffix, configMap.Name)
	assert.Contains(t, configMap.Data, redisConfigKey)
	configData := configMap.Data[redisConfigKey]
	assert.Contains(t, configData, "maxmemory 256mb")
	assert.Contains(t, configData, "maxmemory-policy allkeys-lru")
	assert.Contains(t, configData, "databases 16")
}

func TestRedisControllerWithTLS(t *testing.T) {
	testScheme := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(testScheme))
	require.NoError(t, koncachev1alpha1.AddToScheme(testScheme))
	require.NoError(t, monitoringv1.AddToScheme(testScheme))

	ctx := context.Background()

	redis := &koncachev1alpha1.Redis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-redis-tls",
			Namespace: testNamespace,
		},
		Spec: koncachev1alpha1.RedisSpec{
			Mode:    koncachev1alpha1.RedisModeStandalone,
			Version: "7.2",
			Image:   testRedisImage,
			Port:    6379,
			Security: koncachev1alpha1.RedisSecurity{
				TLS: &koncachev1alpha1.RedisTLS{
					Enabled:    true,
					CertSecret: "redis-tls-secret",
				},
			},
			Storage: koncachev1alpha1.RedisStorage{
				Size:             resource.MustParse("1Gi"),
				StorageClassName: getTestStorageClassNamePtr(),
				AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			},
			ServiceType: corev1.ServiceTypeClusterIP,
		},
	}

	// Create a TLS secret
	tlsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "redis-tls-secret",
			Namespace: testNamespace,
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": []byte("fake-cert"),
			"tls.key": []byte("fake-key"),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(redis, tlsSecret).
		Build()

	reconciler := &RedisReconciler{
		Client: fakeClient,
		Scheme: testScheme,
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      redis.Name,
			Namespace: redis.Namespace,
		},
	}

	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	// Should have a requeue delay for stable resources
	assert.Equal(t, int64(30000000000), result.RequeueAfter.Nanoseconds())

	// Verify ConfigMap contains TLS configuration
	configMap := &corev1.ConfigMap{}
	err = fakeClient.Get(ctx, types.NamespacedName{
		Name:      redis.Name + configMapSuffix,
		Namespace: redis.Namespace,
	}, configMap)
	require.NoError(t, err)

	configData := configMap.Data[redisConfigKey]
	assert.Contains(t, configData, "tls-port")
	assert.Contains(t, configData, "port 0") // Regular port should be disabled when TLS is enabled
}

func TestStandaloneControllerBackupInitContainer(t *testing.T) {
	// Setup test scheme
	testScheme := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(testScheme))
	require.NoError(t, koncachev1alpha1.AddToScheme(testScheme))

	tests := []struct {
		name                      string
		backupInitEnabled         bool
		backupInitImage           string
		expectInitContainer       bool
		expectedInitContainerName string
	}{
		{
			name:                "backup init disabled",
			backupInitEnabled:   false,
			expectInitContainer: false,
		},
		{
			name:                      "backup init enabled with default image",
			backupInitEnabled:         true,
			expectInitContainer:       true,
			expectedInitContainerName: "backup-init",
		},
		{
			name:                      "backup init enabled with custom image",
			backupInitEnabled:         true,
			backupInitImage:           "custom-backup-init:v1.0.0",
			expectInitContainer:       true,
			expectedInitContainerName: "backup-init",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redis := &koncachev1alpha1.Redis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-redis-init",
					Namespace: "default",
				},
				Spec: koncachev1alpha1.RedisSpec{
					Image:           "redis:7.2-alpine",
					ImagePullPolicy: corev1.PullIfNotPresent,
					Storage: koncachev1alpha1.RedisStorage{
						Size:             resource.MustParse("1Gi"),
						StorageClassName: getTestStorageClassNamePtr(),
						AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					},
					ServiceType: corev1.ServiceTypeClusterIP,
					Backup: koncachev1alpha1.RedisBackup{
						Enabled: true,
						Image:   "backup:latest",
						BackUpInitConfig: koncachev1alpha1.BackupInitConfig{
							Enabled: tt.backupInitEnabled,
							Image:   tt.backupInitImage,
						},
						Storage: koncachev1alpha1.RedisBackupStorage{
							Type: "s3",
							S3: &koncachev1alpha1.RedisS3Storage{
								Bucket:     "test-bucket",
								Region:     "us-west-2",
								SecretName: "s3-credentials",
							},
						},
					},
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(testScheme).
				WithObjects(redis).
				Build()

			controller := NewStandaloneController(fakeClient, testScheme)

			// Generate Redis config and config hash
			redisConfig := BuildRedisConfig(redis)
			configHash := ComputeStringHash(redisConfig)

			// Create StatefulSet using the controller's method
			statefulSet := controller.statefulSetForRedis(redis, redisConfig, configHash)

			// Verify init containers
			if tt.expectInitContainer {
				require.Len(t, statefulSet.Spec.Template.Spec.InitContainers, 1, "StatefulSet should have one init container")
				initContainer := statefulSet.Spec.Template.Spec.InitContainers[0]
				assert.Equal(t, tt.expectedInitContainerName, initContainer.Name)
				assert.Equal(t, []string{"/init-backup"}, initContainer.Command)

				// Check image
				expectedImage := redis.Spec.Backup.Image
				if tt.backupInitImage != "" {
					expectedImage = tt.backupInitImage
				}
				assert.Equal(t, expectedImage, initContainer.Image)

				// Check volume mounts
				require.Len(t, initContainer.VolumeMounts, 1, "Init container should have one volume mount")
				assert.Equal(t, "redis-data", initContainer.VolumeMounts[0].Name)
				assert.Equal(t, "/data", initContainer.VolumeMounts[0].MountPath)

				// Check environment variables
				envVarMap := make(map[string]corev1.EnvVar)
				for _, env := range initContainer.Env {
					envVarMap[env.Name] = env
				}
				assert.Equal(t, "/data", envVarMap["DATA_DIR"].Value)
				assert.Equal(t, "test-bucket", envVarMap["S3_BUCKET"].Value)
				assert.Equal(t, "us-west-2", envVarMap["S3_REGION"].Value)
			} else {
				assert.Len(t, statefulSet.Spec.Template.Spec.InitContainers, 0, "StatefulSet should have no init containers")
			}
		})
	}
}
