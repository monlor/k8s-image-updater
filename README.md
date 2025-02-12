# Kubernetes Image Updater

A simple API service for updating container images in a Kubernetes cluster. Supports Deployment, StatefulSet, and DaemonSet resource types.

## Features

- Support for updating container images in Deployments, StatefulSets, and DaemonSets
- Automatic restart based on image pull policy
- API key authentication
- Minimal RBAC configuration
- Kubernetes API access using ServiceAccount

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

## API Usage

### Update Image

**Request**:

```bash
# Full request
curl -X GET "http://k8s-image-updater:8080/api/v1/update?namespace=default&name=my-app&kind=deployment&image=my-app:v1.0.0" \
  -H "X-API-Key: your-secure-api-key"

# Simplified request (using default kind=deployment)
curl -X GET "http://k8s-image-updater:8080/api/v1/update?namespace=default&name=my-app&image=my-app:v1.0.0" \
  -H "X-API-Key: your-secure-api-key"
```

**Parameters**:

- `namespace`: (required) Kubernetes namespace
- `name`: (required) Resource name
- `kind`: (optional) Resource type (deployment, statefulset, or daemonset), defaults to deployment
- `image`: (required) New image address and tag

**Response Example**:

```json
{
  "message": "Image updated",
  "details": "Updated deployment default/my-app with image my-app:v1.0.0"
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