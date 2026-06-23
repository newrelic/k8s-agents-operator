# Kubernetes Metadata Injection Examples

This directory contains example configurations for enabling and using Kubernetes metadata injection with the k8s-agents-operator.

## Examples

### Basic Configuration

1. **[basic-config.yaml](basic-config.yaml)** - Minimal configuration to enable metadata injection
2. **[namespace-filtering.yaml](namespace-filtering.yaml)** - Examples of namespace filtering (ignore list, label selector, opt-out)
3. **[combined-with-agents.yaml](combined-with-agents.yaml)** - Using metadata injection with APM agent auto-instrumentation
4. **[helm-values.yaml](helm-values.yaml)** - Helm chart values for metadata injection

## Quick Start

### Enable Metadata Injection

The simplest way to enable metadata injection:

```bash
kubectl set env deployment/k8s-agents-operator-controller-manager \
  METADATA_INJECTION_ENABLED=true \
  K8S_CLUSTER_NAME=my-cluster \
  -n <operator-namespace>
```

### Verify Configuration

```bash
# Check operator logs
kubectl logs -n <operator-namespace> deployment/k8s-agents-operator-controller-manager | grep "metadata injection"

# Create a test pod
kubectl run test-pod --image=busybox --command -- sleep 3600

# Verify metadata env vars
kubectl exec test-pod -- env | grep NEW_RELIC_METADATA

# Clean up
kubectl delete pod test-pod
```

## Environment Variables Reference

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `METADATA_INJECTION_ENABLED` | No | `false` | Enable/disable metadata injection |
| `K8S_CLUSTER_NAME` | Yes* | - | Kubernetes cluster name (*required when enabled) |
| `METADATA_INJECTION_IGNORED_NAMESPACES` | No | `kube-system,kube-public,kube-node-lease` | Comma-separated list of ignored namespaces |
| `METADATA_INJECTION_NAMESPACE_LABEL_SELECTOR` | No | `""` | Label selector for namespace filtering |

## Injected Environment Variables

When metadata injection is enabled, these environment variables are added to all containers:

| Variable | Source | Example Value |
|----------|--------|---------------|
| `NEW_RELIC_METADATA_KUBERNETES_CLUSTER_NAME` | Config | `production-cluster` |
| `NEW_RELIC_METADATA_KUBERNETES_NODE_NAME` | Downward API | `node-1` |
| `NEW_RELIC_METADATA_KUBERNETES_NAMESPACE_NAME` | Downward API | `default` |
| `NEW_RELIC_METADATA_KUBERNETES_POD_NAME` | Downward API | `my-app-7f9c4d-xyz` |
| `NEW_RELIC_METADATA_KUBERNETES_DEPLOYMENT_NAME` | Derived | `my-app` |
| `NEW_RELIC_METADATA_KUBERNETES_CONTAINER_NAME` | Static | `app` |
| `NEW_RELIC_METADATA_KUBERNETES_CONTAINER_IMAGE_NAME` | Static | `nginx:latest` |

## Use Cases

### Use Case 1: Enable for All Namespaces (Except System Namespaces)

```yaml
env:
  - name: METADATA_INJECTION_ENABLED
    value: "true"
  - name: K8S_CLUSTER_NAME
    value: "production"
  - name: METADATA_INJECTION_IGNORED_NAMESPACES
    value: "kube-system,kube-public,kube-node-lease"
```

### Use Case 2: Enable Only for Labeled Namespaces

```yaml
env:
  - name: METADATA_INJECTION_ENABLED
    value: "true"
  - name: K8S_CLUSTER_NAME
    value: "production"
  - name: METADATA_INJECTION_NAMESPACE_LABEL_SELECTOR
    value: "newrelic-metadata=enabled"
```

Then label your namespaces:
```bash
kubectl label namespace my-app newrelic-metadata=enabled
```

### Use Case 3: Opt-Out Specific Namespace

Enable globally, but opt-out specific namespaces:

```yaml
# Namespace annotation
apiVersion: v1
kind: Namespace
metadata:
  name: sensitive-namespace
  annotations:
    newrelic.com/metadata-injection: "false"
```

## Troubleshooting

### No metadata in pods

1. Check operator configuration:
   ```bash
   kubectl get deployment -n <operator-namespace> k8s-agents-operator-controller-manager \
     -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="METADATA_INJECTION_ENABLED")].value}'
   ```

2. Check operator logs:
   ```bash
   kubectl logs -n <operator-namespace> deployment/k8s-agents-operator-controller-manager
   ```

3. Verify namespace is not filtered:
   ```bash
   # Check namespace annotations
   kubectl get namespace <namespace-name> -o yaml

   # Check namespace labels (if using label selector)
   kubectl get namespace <namespace-name> --show-labels
   ```

### Partial metadata in pods

If only some metadata env vars are present, check:
- `K8S_CLUSTER_NAME` is set (required for cluster name var)
- Pod has proper owner references (required for deployment name)

## More Information

- [Main README](../../README.md#kubernetes-metadata-injection)
- [Migration Guide](../../docs/metadata-injection-migration.md)
- [New Relic Documentation](https://docs.newrelic.com/docs/kubernetes-pixie/kubernetes-integration/advanced-configuration/link-apm-applications-kubernetes/)
