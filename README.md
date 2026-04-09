<a href="https://opensource.newrelic.com/oss-category/#community-plus"><picture><source media="(prefers-color-scheme: dark)" srcset="https://github.com/newrelic/opensource-website/raw/main/src/images/categories/dark/Community_Plus.png"><source media="(prefers-color-scheme: light)" srcset="https://github.com/newrelic/opensource-website/raw/main/src/images/categories/Community_Plus.png"><img alt="New Relic Open Source community plus project banner." src="https://github.com/newrelic/opensource-website/raw/main/src/images/categories/Community_Plus.png"></picture></a>

# K8s Agents Operator [![codecov](https://codecov.io/gh/newrelic/k8s-agents-operator/graph/badge.svg?token=YUSEXVY3WF)](https://codecov.io/gh/newrelic/k8s-agents-operator)

This project auto-instruments containerized workloads in Kubernetes with New Relic agents.

## Table Of Contents

- [Installation](#installation)
- [Instrumentation](instrumentation.md)
- [Kubernetes Metadata Injection](#kubernetes-metadata-injection)
- [Development](#development)
- [Compatibility](#compatibility)
- [Support](#support)
- [Contribute](#contribute)
- [License](#license)

## Installation

For instructions on how to install the Helm chart, read the [chart's README](./charts/k8s-agents-operator/README.md)

## Kubernetes Metadata Injection

The K8s Agents Operator can automatically inject Kubernetes metadata as environment variables into your pod containers, enabling APM agents to correlate application traces with infrastructure monitoring.

### What is Metadata Injection?

Metadata injection adds the following environment variables to all containers in your pods:

- `NEW_RELIC_METADATA_KUBERNETES_CLUSTER_NAME` - Your Kubernetes cluster name
- `NEW_RELIC_METADATA_KUBERNETES_NODE_NAME` - The node where the pod is running
- `NEW_RELIC_METADATA_KUBERNETES_NAMESPACE_NAME` - The namespace of the pod
- `NEW_RELIC_METADATA_KUBERNETES_POD_NAME` - The name of the pod
- `NEW_RELIC_METADATA_KUBERNETES_DEPLOYMENT_NAME` - The deployment name (when applicable)
- `NEW_RELIC_METADATA_KUBERNETES_CONTAINER_NAME` - The name of the container
- `NEW_RELIC_METADATA_KUBERNETES_CONTAINER_IMAGE_NAME` - The container image

This metadata enables New Relic to automatically link your APM data with Kubernetes infrastructure data, providing a unified view of your applications and their underlying infrastructure.

### Enabling Metadata Injection

Metadata injection is **disabled by default** and must be explicitly enabled. Set the following environment variables in the operator deployment:

```yaml
env:
  # Enable metadata injection
  - name: METADATA_INJECTION_ENABLED
    value: "true"

  # Set your cluster name (required)
  - name: K8S_CLUSTER_NAME
    value: "production-cluster"

  # Optional: Customize ignored namespaces (default shown)
  - name: METADATA_INJECTION_IGNORED_NAMESPACES
    value: "kube-system,kube-public,kube-node-lease"

  # Optional: Only inject into namespaces with specific labels
  - name: METADATA_INJECTION_NAMESPACE_LABEL_SELECTOR
    value: ""
```

### Configuration Options

#### `METADATA_INJECTION_ENABLED`
- **Type:** Boolean (`"true"` or `"false"`)
- **Default:** `"false"`
- **Description:** Master switch to enable/disable metadata injection

#### `K8S_CLUSTER_NAME`
- **Type:** String
- **Required:** Yes (when metadata injection is enabled)
- **Description:** The name of your Kubernetes cluster. This value appears in New Relic's infrastructure monitoring.

#### `METADATA_INJECTION_IGNORED_NAMESPACES`
- **Type:** Comma-separated list of namespace names
- **Default:** `"kube-system,kube-public,kube-node-lease"`
- **Description:** Namespaces where metadata injection should be skipped. System namespaces are ignored by default to avoid unnecessary overhead.

#### `METADATA_INJECTION_NAMESPACE_LABEL_SELECTOR`
- **Type:** Label selector string (e.g., `"inject=enabled"`)
- **Default:** `""` (empty - inject into all non-ignored namespaces)
- **Description:** When set, only namespaces matching this label selector will receive metadata injection. This allows fine-grained control over which namespaces are instrumented.

### Namespace Filtering

Metadata injection uses a three-layer filtering mechanism (evaluated in order):

1. **Ignore List** - Namespaces in `METADATA_INJECTION_IGNORED_NAMESPACES` are always skipped
2. **Opt-Out Annotation** - Namespaces with annotation `newrelic.com/metadata-injection: "false"` are skipped
3. **Label Selector** - If `METADATA_INJECTION_NAMESPACE_LABEL_SELECTOR` is set, only matching namespaces receive injection

#### Example: Opt-Out a Specific Namespace

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: my-namespace
  annotations:
    newrelic.com/metadata-injection: "false"  # Disable metadata injection
```

#### Example: Label-Based Filtering

```yaml
# Operator configuration
env:
  - name: METADATA_INJECTION_NAMESPACE_LABEL_SELECTOR
    value: "newrelic-metadata=enabled"

---
# Only this namespace will receive metadata injection
apiVersion: v1
kind: Namespace
metadata:
  name: production
  labels:
    newrelic-metadata: enabled
```

### Using with APM Agent Injection

Metadata injection works independently of APM agent injection. You can:

- **Use both together** - Automatically inject both metadata and APM agents
- **Use metadata only** - Inject metadata into pods that already have APM agents installed
- **Use agent injection only** - Inject APM agents without metadata (not recommended)

When both are enabled, the operator injects metadata first, then applies APM agent instrumentation.

### Migrating from k8s-metadata-injection

If you're currently using the standalone [k8s-metadata-injection](https://github.com/newrelic/k8s-metadata-injection) webhook, you can migrate to this operator for consolidated management. See the [migration guide](docs/metadata-injection-migration.md) for detailed steps.

**Key differences:**
- Opt-in model (disabled by default vs. always-on)
- Environment variable `K8S_CLUSTER_NAME` instead of `CLUSTER_NAME`
- Consolidated deployment (one operator instead of two webhooks)

### Examples

See [examples/metadata-injection/](examples/metadata-injection/) for complete configuration examples.

### Troubleshooting

#### Metadata not appearing in pods

1. **Check operator logs for configuration errors:**
   ```bash
   kubectl logs -n <operator-namespace> deployment/k8s-agents-operator-controller-manager
   ```

2. **Verify metadata injection is enabled:**
   ```bash
   kubectl get deployment -n <operator-namespace> k8s-agents-operator-controller-manager -o yaml | grep METADATA_INJECTION_ENABLED
   ```

3. **Check namespace filtering:**
   - Verify namespace is not in ignore list
   - Check for opt-out annotation on namespace
   - If using label selector, verify namespace has matching labels

4. **Verify env vars in pod:**
   ```bash
   kubectl get pod <pod-name> -o jsonpath='{.spec.containers[*].env[?(@.name=="NEW_RELIC_METADATA_KUBERNETES_CLUSTER_NAME")].name}'
   ```

#### Operator fails to start with metadata injection enabled

**Error:** `K8S_CLUSTER_NAME must be set when METADATA_INJECTION_ENABLED is true`

**Solution:** Set the `K8S_CLUSTER_NAME` environment variable in the operator deployment.

#### Metadata injection conflicts with existing env vars

Metadata injection is **idempotent** - it never overwrites existing environment variables. If your pods already have any of the metadata environment variables set, those values are preserved.

## Development

We use Minikube and Tilt to spawn a local environment that it will reload after any changes inside the charts or the integration code.

Make sure you have these tools or install them:

* [Install minikube](https://minikube.sigs.k8s.io/docs/start/)
* [Install Tilt](https://docs.tilt.dev/install.html)
* [Install Helm](https://helm.sh/docs/intro/install/)

Start the local environment:

```bash
ctlptl create registry ctlptl-registry --port=5005
ctlptl create cluster minikube --registry=ctlptl-registry
tilt up
```
## Compatibility
* This project has been tested on standard Kubernetes clusters.
* **EKS Fargate has not been tested and not officially supported**

## Support

New Relic hosts and moderates an online forum where you can interact with New Relic employees as well as other customers to get help and share best practices. Like all official New Relic open source projects, there's a related Community topic in the New Relic Explorers Hub. You can find this project's topic/threads here:

* [New Relic Documentation](https://docs.newrelic.com): Comprehensive guidance for using our platform
* [New Relic Community](https://forum.newrelic.com/t/new-relic-kubernetes-open-source-integration/109093): The best place to engage in troubleshooting questions
* [New Relic Developer](https://developer.newrelic.com/): Resources for building a custom observability applications
* [New Relic University](https://learn.newrelic.com/): A range of online training for New Relic users of every level
* [New Relic Technical Support](https://support.newrelic.com/) 24/7/365 ticketed support. Read more about our [Technical Support Offerings](https://docs.newrelic.com/docs/licenses/license-information/general-usage-licenses/support-plan)

## Contribute

We encourage your contributions to improve *K8s Agents Operator*! Keep in mind that when you submit your pull request, you'll need to sign the CLA via the click-through using CLA-Assistant. You only have to sign the CLA one time per project.

If you have any questions, or to execute our corporate CLA (which is required if your contribution is on behalf of a company), drop us an email at opensource@newrelic.com.

**A note about vulnerabilities**

As noted in our [security policy](../../security/policy), New Relic is committed to the privacy and security of our customers and their data. We believe that providing coordinated disclosure by security researchers and engaging with the security community are important means to achieve our security goals.

If you believe you have found a security vulnerability in this project or any of New Relic's products or websites, we welcome and greatly appreciate you reporting it to New Relic through [HackerOne](https://hackerone.com/newrelic).

If you would like to contribute to this project, review [these guidelines](./CONTRIBUTING.md).

To all contributors, we thank you!  Without your contribution, this project would not be what it is today.

## License
*K8s Agents Operator* is licensed under the [Apache 2.0](http://apache.org/licenses/LICENSE-2.0.txt) License.

