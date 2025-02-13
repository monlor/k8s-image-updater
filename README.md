# Kubernetes Image Updater

A powerful and flexible Kubernetes controller that automatically updates container images in your cluster. It supports both API-driven updates and annotation-based automatic updates, making it easy to keep your applications up to date.

## Features

🚀 **Two Update Methods**
- API-driven manual updates for controlled deployments
- Annotation-based automatic updates for hands-free operation (can be disabled)

🔄 **Resource Support**
- Deployments
- StatefulSets
- DaemonSets

🎯 **Smart Update Strategies**
- Semantic version-based updates (e.g., 1.2.3 -> 1.2.4)
- Digest-based updates for immutable tags
- Prioritizes clean version tags over suffixed ones (e.g., prefers 1.2.3 over 1.2.3-alpine)

🔐 **Security Features**
- API key authentication
- Registry authentication support
- Minimal RBAC configuration

⚙️ **Flexible Configuration**
- Enable/disable auto-updater globally
- Configurable update interval
- Container-specific updates
- Multiple registry support

🔍 **Monitoring & Control**
- Detailed update logs
- Dry-run mode available
- Automatic restart based on image pull policy

## Installation

1. Create API key Secret:

```bash
kubectl create secret generic k8s-image-updater \
  --namespace=kube-system \
  --from-literal=api-key=your-secure-api-key
```

2. Apply RBAC configuration:

```bash
kubectl apply -f deploy/rbac.yaml
```

3. Deploy the service:

```bash
kubectl apply -f deploy/deployment.yaml
```

## Auto-Update Configuration

The auto-update feature can be configured using annotations on your Kubernetes resources:

```yaml
annotations:
  image-updater.k8s.io/enabled: "true"           # Enable auto-update for this resource
  image-updater.k8s.io/mode: "release"          # Update mode: "release", "digest" or "latest"
  image-updater.k8s.io/container: "app"         # Optional: specify container name
```

### Update Modes

1. **Release Mode** (`mode: "release"`)
   - Updates to the latest version based on semantic versioning
   - Supports both `v` prefixed (v1.2.3) and non-prefixed (1.2.3) versions
   - Example: `nginx:1.21.0` -> `nginx:1.22.0`

2. **Digest Mode** (`mode: "digest"`)
   - Updates when the image digest changes
   - Useful for `latest` tags or when you want to update on any change
   - Example: `nginx@sha256:abc...` -> `nginx@sha256:xyz...`

3. **Latest Mode** (`mode: "latest"`)
   - Monitors digest changes for images with fixed tags (including latest tag)
   - Requires `imagePullPolicy: Always` to be set
   - Restarts the pod when a new image is detected with the same tag
   - Example: When `nginx:latest` has a new digest, the pod will be restarted

### Example Configuration

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  annotations:
    image-updater.k8s.io/enabled: "true"
    image-updater.k8s.io/mode: "release"
    image-updater.k8s.io/container: "app"
spec:
  template:
    spec:
      containers:
      - name: app
        image: my-registry/my-app:1.0.0
        imagePullPolicy: Always
```

## API Usage

### Update Image

**Request**:

```bash
# Full request
curl -X GET "http://k8s-image-updater:8080/api/v1/update?namespace=default&service=my-app&container=app&kind=deployment&image=my-app:v1.0.0" \
  -H "X-API-Key: your-secure-api-key"

# Simplified request (using default kind=deployment)
curl -X GET "http://k8s-image-updater:8080/api/v1/update?namespace=default&service=my-app&container=app&image=my-app:v1.0.0" \
  -H "X-API-Key: your-secure-api-key"
```

**Parameters**:

- `namespace`: (required) Kubernetes namespace
- `service`: (required) Service name
- `container`: (optional) Container name, defaults to first container
- `kind`: (optional) Resource type (deployment, statefulset, or daemonset), defaults to deployment
- `image`: (required) New image address and tag

**Response Example**:

```json
{
  "details":"Image nginx:latest is already up to date for deployment default/nginx-deployment (container: nginx)",
  "ok":true
}
```

## Using in GitHub Actions

Example workflow:

```yaml
name: Update K8s Image

on:
  workflow_dispatch:
    inputs:
      namespace:
        description: 'Kubernetes namespace'
        required: true
      name:
        description: 'Resource name'
        required: true
      kind:
        description: 'Resource kind (deployment/statefulset/daemonset)'
        required: false
        default: 'deployment'
      image:
        description: 'New image with tag'
        required: true

jobs:
  update-image:
    runs-on: ubuntu-latest
    steps:
    - name: Update K8s Image
      run: |
        curl -X GET "${{ secrets.K8S_IMAGE_UPDATER_URL }}/api/v1/update?namespace=${{ github.event.inputs.namespace }}&name=${{ github.event.inputs.name }}&kind=${{ github.event.inputs.kind }}&image=${{ github.event.inputs.image }}" \
          -H "X-API-Key: ${{ secrets.K8S_IMAGE_UPDATER_API_KEY }}"
```

## Configuration

Environment variables:

- `API_PORT`: API service port (default: 8080)
- `API_KEY`: API access key
- `KUBECONFIG`: Path to kubeconfig file
- `UPDATER_ENABLED`: Enable/disable auto-updater (default: true)
- `IMAGE_UPDATE_INTERVAL`: Interval for checking image updates (default: 5m)
- `LOG_LEVEL`: Logging level (default: info)
- `ALLOWED_NAMESPACES`: Comma-separated list of namespaces that the API can operate on

### Auto-Updater Configuration

The auto-updater can be:
1. Disabled globally using `UPDATER_ENABLED=false`
2. Enabled/disabled per resource using annotations

Example deployment with auto-updater disabled globally:
```yaml
env:
- name: UPDATER_ENABLED
  value: "false"
```

## Build and Run

1. Build image:

```bash
docker build -t k8s-image-updater:latest .
```

2. Deploy to Kubernetes:

```bash
# Create API key
kubectl create secret generic k8s-image-updater \
  --namespace=kube-system \
  --from-literal=api-key=your-secure-api-key

# Apply RBAC configuration
kubectl apply -f deploy/rbac.yaml

# Deploy service
kubectl apply -f deploy/deployment.yaml
``` 