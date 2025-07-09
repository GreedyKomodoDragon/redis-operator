# redis-operator
// TODO(user): Add simple overview of use/purpose

## Description
// TODO(user): An in-depth paragraph about your project and overview of use

## Getting Started

### Prerequisites
- go version v1.23.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/redis-operator:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/redis-operator:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following the options to release and provide this solution to the users.

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/redis-operator:tag
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/redis-operator/<tag or branch>/dist/install.yaml
```

### By providing a Helm Chart

1. Build the chart using the optional helm plugin

```sh
operator-sdk edit --plugins=helm/v1-alpha
```

2. See that a chart was generated under 'dist/chart', and users
can obtain this solution from there.

**NOTE:** If you change the project, you need to update the Helm Chart
using the same command above to sync the latest changes. Furthermore,
if you create webhooks, you need to use the above command with
the '--force' flag and manually ensure that any custom configuration
previously added to 'dist/chart/values.yaml' or 'dist/chart/manager/manager.yaml'
is manually re-applied afterwards.

## Testing

The Redis Operator includes comprehensive testing at multiple levels:

### Unit Tests

Run the standard unit tests:

```sh
make test
```

### Cucumber Integration Tests

The project includes Cucumber/Gherkin-based integration tests for end-to-end testing scenarios.

**Quick unit tests only (CI-friendly):**
```sh
make test-cucumber-short
```

**Full integration tests (requires minikube):**
```sh
# Ensure minikube is running
minikube start

# Run integration tests
make test-cucumber

# Or use the helper script with auto-setup
./scripts/run-cucumber-tests.sh
```

**Run all tests:**
```sh
make test-all
```

The cucumber tests cover:
- Standalone Redis deployment scenarios
- TLS and authentication security features  
- Monitoring and metrics collection
- Configuration updates and persistence
- Resource lifecycle management

See [test/cucumber/README.md](test/cucumber/README.md) for detailed documentation.

## S3 Backup Configuration

The Redis Operator supports automated backups to S3-compatible storage with optional backup restoration on pod startup.

### Creating S3 Credentials Secret

Create a Kubernetes secret with your S3 credentials:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: s3-credentials
  namespace: default
type: Opaque
data:
  # Base64 encoded AWS credentials
  aws-access-key-id: <base64-encoded-access-key>
  aws-secret-access-key: <base64-encoded-secret-key>
```

Or create it using kubectl:

```bash
kubectl create secret generic s3-credentials \
  --from-literal=aws-access-key-id=YOUR_ACCESS_KEY \
  --from-literal=aws-secret-access-key=YOUR_SECRET_KEY
```

### Backup Configuration Example

```yaml
apiVersion: koncache.greedykomodo/v1alpha1
kind: Redis
metadata:
  name: redis-with-backup
spec:
  # ... other Redis configuration ...
  backup:
    enabled: true
    image: "greedykomodo/redis-backup:v0.0.20"
    schedule: "0 3 * * *"  # Daily at 3 AM
    retention: 7
    storage:
      type: "s3"
      s3:
        bucket: "redis-backups"
        region: "us-east-1"
        prefix: "my-redis-backups"
        secretName: "s3-credentials"
    # Optional: Enable backup restoration on pod startup
    backupInitConfig:
      enabled: true
      image: "greedykomodo/redis-backup-init:v0.0.20"
```

### Backup Features

- **Automated Scheduling**: Backups run according to cron schedule
- **S3 Storage**: Supports AWS S3 and S3-compatible storage (MinIO, etc.)
- **Backup Restoration**: Optional init container restores latest backup on pod startup
- **Retention Policy**: Configurable backup retention
- **Streaming Uploads**: Direct streaming to S3 without local disk usage

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

