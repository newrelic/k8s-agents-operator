# Migrating from k8s-metadata-injection to k8s-agents-operator

This guide helps you migrate from the standalone [k8s-metadata-injection](https://github.com/newrelic/k8s-metadata-injection) webhook to the integrated metadata injection functionality in k8s-agents-operator.

## Why Migrate?

**Benefits of consolidation:**
- **Single operator** - Manage both APM agent injection and metadata injection from one place
- **Unified configuration** - One set of environment variables and CRDs
- **Simplified deployment** - One webhook instead of two
- **Reduced overhead** - Single admission webhook for all mutations
- **Better integration** - Metadata and agent injection work seamlessly together

## Prerequisites

- Existing k8s-metadata-injection deployment running in your cluster
- Kubernetes cluster with admission webhooks enabled
- Helm 3.x (if using Helm installation)
- kubectl access to your cluster

## Migration Overview

The migration process involves:
1. Deploy k8s-agents-operator with metadata injection enabled
2. Validate metadata injection works
3. Remove k8s-metadata-injection deployment
4. Clean up old webhook configuration
5. Restart affected pods (optional)

**Estimated time:** 30-45 minutes

**Downtime:** Zero downtime - both webhooks can run simultaneously during migration

## Configuration Mapping

### Environment Variables

| k8s-metadata-injection | k8s-agents-operator | Notes |
|------------------------|---------------------|-------|
| `CLUSTER_NAME` | `K8S_CLUSTER_NAME` | Different variable name |
| `TIMEOUT` | N/A | Fixed timeout in operator |
| `PORT` | N/A | Operator manages webhook port |

### Helm Chart Values

| k8s-metadata-injection | k8s-agents-operator | Notes |
|------------------------|---------------------|-------|
| `cluster` | `env.K8S_CLUSTER_NAME` | Set as environment variable |
| `ignoreNamespaces` | `env.METADATA_INJECTION_IGNORED_NAMESPACES` | Comma-separated string |
| `injectOnlyLabeledNamespaces` | `env.METADATA_INJECTION_NAMESPACE_LABEL_SELECTOR` | Label selector format |
| Always enabled | `env.METADATA_INJECTION_ENABLED=true` | Opt-in model |

### Namespace Filtering Behavior

**k8s-metadata-injection:**
- Always injects unless namespace is in ignore list
- Optional: Only inject into labeled namespaces

**k8s-agents-operator:**
- Disabled by default (must enable explicitly)
- Three-layer filtering: ignore list, opt-out annotation, label selector
- More flexible control

## Step-by-Step Migration

### Step 1: Record Current Configuration

Before starting, document your current k8s-metadata-injection configuration:

```bash
# Get current Helm values
helm get values newrelic-metadata-injection -n newrelic > old-config.yaml

# Or get environment variables from deployment
kubectl get deployment newrelic-metadata-injection -n newrelic -o yaml > old-deployment.yaml
```

**Example old configuration:**
```yaml
# old-config.yaml from k8s-metadata-injection Helm chart
cluster: "production-cluster"
ignoreNamespaces:
  - kube-system
  - kube-public
  - kube-node-lease
injectOnlyLabeledNamespaces: false
```

### Step 2: Deploy k8s-agents-operator with Metadata Injection

#### Option A: Using Helm Chart

Create a values file for k8s-agents-operator:

```yaml
# k8s-agents-operator-values.yaml
env:
  # Enable metadata injection
  - name: METADATA_INJECTION_ENABLED
    value: "true"

  # Cluster name (from old config)
  - name: K8S_CLUSTER_NAME
    value: "production-cluster"

  # Ignored namespaces (from old config)
  - name: METADATA_INJECTION_IGNORED_NAMESPACES
    value: "kube-system,kube-public,kube-node-lease"

  # Optional: Label selector (if using injectOnlyLabeledNamespaces)
  - name: METADATA_INJECTION_NAMESPACE_LABEL_SELECTOR
    value: ""  # Set to "newrelic-metadata-injection=enabled" if using labeled namespaces
```

Install or upgrade the operator:

```bash
# Install new operator
helm repo add newrelic https://helm-charts.newrelic.com
helm repo update

# Install k8s-agents-operator
helm install k8s-agents-operator newrelic/k8s-agents-operator \
  --namespace newrelic \
  --create-namespace \
  --values k8s-agents-operator-values.yaml

# Or upgrade existing installation
helm upgrade k8s-agents-operator newrelic/k8s-agents-operator \
  --namespace newrelic \
  --values k8s-agents-operator-values.yaml
```

#### Option B: Using kubectl (Direct Deployment)

If you deployed k8s-agents-operator manually, update the deployment:

```bash
kubectl edit deployment k8s-agents-operator-controller-manager -n newrelic
```

Add the environment variables to the manager container:

```yaml
spec:
  template:
    spec:
      containers:
      - name: manager
        env:
        - name: METADATA_INJECTION_ENABLED
          value: "true"
        - name: K8S_CLUSTER_NAME
          value: "production-cluster"
        - name: METADATA_INJECTION_IGNORED_NAMESPACES
          value: "kube-system,kube-public,kube-node-lease"
        - name: METADATA_INJECTION_NAMESPACE_LABEL_SELECTOR
          value: ""
```

### Step 3: Verify Operator is Running

```bash
# Check operator pod is running
kubectl get pods -n newrelic -l control-plane=controller-manager

# Check operator logs for metadata injection enabled message
kubectl logs -n newrelic deployment/k8s-agents-operator-controller-manager | grep "metadata injection"
```

**Expected output:**
```
{"level":"info","ts":"...","msg":"metadata injection enabled","clusterName":"production-cluster","ignoredNamespaces":["kube-system","kube-public","kube-node-lease"],...}
```

### Step 4: Test with a New Pod

Create a test pod to verify metadata injection works:

```yaml
# test-pod.yaml
apiVersion: v1
kind: Pod
metadata:
  name: metadata-test
  namespace: default
spec:
  containers:
  - name: test
    image: busybox:latest
    command: ["sleep", "3600"]
```

```bash
# Create test pod
kubectl apply -f test-pod.yaml

# Wait for pod to be running
kubectl wait --for=condition=ready pod/metadata-test -n default --timeout=60s

# Verify metadata env vars are present
kubectl exec metadata-test -n default -- env | grep NEW_RELIC_METADATA
```

**Expected output:**
```
NEW_RELIC_METADATA_KUBERNETES_CLUSTER_NAME=production-cluster
NEW_RELIC_METADATA_KUBERNETES_NODE_NAME=node-1
NEW_RELIC_METADATA_KUBERNETES_NAMESPACE_NAME=default
NEW_RELIC_METADATA_KUBERNETES_POD_NAME=metadata-test
NEW_RELIC_METADATA_KUBERNETES_CONTAINER_NAME=test
NEW_RELIC_METADATA_KUBERNETES_CONTAINER_IMAGE_NAME=busybox:latest
```

**If deployment name is derived:**
```
NEW_RELIC_METADATA_KUBERNETES_DEPLOYMENT_NAME=my-app
```

### Step 5: Validate Existing Pods (Optional)

Check if existing pods already have metadata (from k8s-metadata-injection):

```bash
# Check an existing pod
kubectl exec <existing-pod> -n <namespace> -- env | grep NEW_RELIC_METADATA
```

**Note:** Existing pods will NOT automatically get metadata from the new operator. You need to restart them (see Step 7).

### Step 6: Remove k8s-metadata-injection

Once you've verified the new operator works, remove the old metadata injection webhook:

```bash
# If installed via Helm
helm uninstall newrelic-metadata-injection -n newrelic

# If installed via kubectl
kubectl delete deployment newrelic-metadata-injection -n newrelic
kubectl delete service newrelic-metadata-injection -n newrelic
```

### Step 7: Clean Up Old Webhook Configuration

The old webhook configuration must be removed to prevent conflicts:

```bash
# List webhook configurations
kubectl get mutatingwebhookconfiguration

# Delete k8s-metadata-injection webhook
kubectl delete mutatingwebhookconfiguration newrelic-metadata-injection-cfg
```

### Step 8: Restart Affected Pods (Optional)

If you want existing pods to receive metadata from the new operator:

```bash
# Restart all deployments in a namespace
kubectl rollout restart deployment -n <namespace>

# Or restart a specific deployment
kubectl rollout restart deployment <deployment-name> -n <namespace>

# Or restart a specific pod
kubectl delete pod <pod-name> -n <namespace>
```

**Note:** This step is optional. New pods will automatically receive metadata. Existing pods can continue with metadata from the old webhook until their next restart.

## Verification Checklist

After migration, verify:

- [ ] k8s-agents-operator pod is running
- [ ] Operator logs show "metadata injection enabled"
- [ ] Test pod has all 7 metadata env vars
- [ ] Existing applications work correctly
- [ ] Old k8s-metadata-injection deployment removed
- [ ] Old MutatingWebhookConfiguration removed
- [ ] No webhook-related errors in operator logs

## Rollback Procedure

If you need to rollback to k8s-metadata-injection:

### 1. Disable Metadata Injection in Operator

```bash
kubectl set env deployment/k8s-agents-operator-controller-manager \
  METADATA_INJECTION_ENABLED=false \
  -n newrelic
```

### 2. Reinstall k8s-metadata-injection

```bash
helm install newrelic-metadata-injection newrelic/nri-metadata-injection \
  --namespace newrelic \
  --values old-config.yaml  # From Step 1
```

### 3. Verify Old Webhook Works

```bash
# Create test pod
kubectl run rollback-test --image=busybox --command -- sleep 3600

# Verify metadata present
kubectl exec rollback-test -- env | grep NEW_RELIC_METADATA

# Clean up
kubectl delete pod rollback-test
```

## Troubleshooting

### Issue: Metadata not appearing in new pods

**Symptoms:**
- New pods don't have `NEW_RELIC_METADATA_*` environment variables
- Operator logs show "metadata injection disabled"

**Solutions:**
1. Verify `METADATA_INJECTION_ENABLED=true` is set
2. Check `K8S_CLUSTER_NAME` is configured
3. Verify namespace is not in ignore list
4. Check namespace doesn't have opt-out annotation

### Issue: Both webhooks injecting metadata

**Symptoms:**
- Duplicate environment variables in pods
- Webhook conflicts in logs

**Solutions:**
1. Ensure old webhook is removed: `kubectl get mutatingwebhookconfiguration`
2. Delete old deployment: `helm uninstall newrelic-metadata-injection`

### Issue: Operator fails to start

**Symptoms:**
- Operator pod in CrashLoopBackOff
- Logs show: "K8S_CLUSTER_NAME must be set"

**Solutions:**
1. Set `K8S_CLUSTER_NAME` environment variable
2. Check operator deployment configuration

### Issue: Some namespaces not receiving metadata

**Symptoms:**
- Pods in certain namespaces missing metadata
- Other namespaces work fine

**Solutions:**
1. Check if namespace is in `METADATA_INJECTION_IGNORED_NAMESPACES`
2. Look for opt-out annotation: `kubectl get namespace <name> -o yaml`
3. If using label selector, verify namespace has required labels

## Advanced Scenarios

### Migrating with Custom Certificate Management

If you're using custom certificates with k8s-metadata-injection:

```bash
# k8s-agents-operator manages its own certificates
# No special configuration needed
# cert-manager integration is automatic if available
```

### Migrating with Custom Namespace Selector

Old config:
```yaml
injectOnlyLabeledNamespaces: true
# Requires namespace label: newrelic-metadata-injection=enabled
```

New config:
```yaml
env:
  - name: METADATA_INJECTION_NAMESPACE_LABEL_SELECTOR
    value: "newrelic-metadata-injection=enabled"
```

### Running Both Webhooks Temporarily

During migration, you can run both webhooks simultaneously:

- **k8s-metadata-injection** - Existing pods continue to receive metadata
- **k8s-agents-operator** - New pods receive metadata from new operator

**Note:** This is safe because metadata injection is idempotent (never overwrites existing env vars).

## Post-Migration Cleanup

After successful migration and validation:

1. **Remove old Helm values file:**
   ```bash
   rm old-config.yaml old-deployment.yaml
   ```

2. **Update documentation:**
   - Update runbooks to reference new operator
   - Update deployment procedures
   - Update monitoring dashboards if referencing old webhook

3. **Archive old configurations:**
   - Save old configurations for reference
   - Document migration date and versions

## Getting Help

If you encounter issues during migration:

- **GitHub Issues:** [k8s-agents-operator issues](https://github.com/newrelic/k8s-agents-operator/issues)
- **New Relic Support:** https://support.newrelic.com
- **Community Forum:** https://forum.newrelic.com

## Appendix: Configuration Comparison

### Complete Configuration Example

**Before (k8s-metadata-injection):**
```yaml
# values.yaml for nri-metadata-injection chart
cluster: "production-cluster"
replicas: 1
ignoreNamespaces:
  - kube-system
  - kube-public
  - kube-node-lease
injectOnlyLabeledNamespaces: false
timeoutSeconds: 28
```

**After (k8s-agents-operator):**
```yaml
# values.yaml for k8s-agents-operator chart
env:
  - name: METADATA_INJECTION_ENABLED
    value: "true"
  - name: K8S_CLUSTER_NAME
    value: "production-cluster"
  - name: METADATA_INJECTION_IGNORED_NAMESPACES
    value: "kube-system,kube-public,kube-node-lease"
  - name: METADATA_INJECTION_NAMESPACE_LABEL_SELECTOR
    value: ""
```

### Feature Parity Matrix

| Feature | k8s-metadata-injection | k8s-agents-operator |
|---------|------------------------|---------------------|
| Cluster name injection | ✅ | ✅ |
| Node name injection | ✅ | ✅ |
| Namespace name injection | ✅ | ✅ |
| Pod name injection | ✅ | ✅ |
| Deployment name injection | ✅ | ✅ |
| Container name injection | ✅ | ✅ |
| Container image injection | ✅ | ✅ |
| Namespace ignore list | ✅ | ✅ |
| Namespace label selector | ✅ | ✅ |
| Namespace opt-out annotation | ❌ | ✅ (new feature) |
| Always-on by default | ✅ | ❌ (opt-in) |
| Init container injection | ✅ | ✅ |
| Certificate management | Manual/cert-manager | Automatic |

**Legend:**
- ✅ Supported
- ❌ Not supported
