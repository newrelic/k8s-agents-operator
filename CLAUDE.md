# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

K8s Agents Operator is a Kubebuilder-based Kubernetes operator that auto-instruments containerized workloads with New Relic APM agents. It uses admission webhooks to inject language-specific agents (Java, Python, Ruby, Node.js, PHP, .NET) into pods at runtime, enabling zero-code APM instrumentation.

**Key Technologies:**
- Go 1.25.5 with Kubebuilder v4 framework (controller-runtime v0.22.4)
- CRD: `Instrumentation` (newrelic.com domain)
- Supports 4 API versions: v1alpha2 (legacy), v1beta1 (legacy), v1beta2, v1beta3 (current)

## Common Commands

### Building and Testing

```bash
# Run tests (default k8s version)
make test
# or explicitly
make go-test

# Run tests with race detector
make go-test-race

# Run tests across all k8s versions
make all-go-tests

# Run e2e tests
make e2e-tests
# Run e2e tests across all k8s versions
make all-e2e-tests

# Build the operator binary
make build

# Build docker image
make docker-build
```

### Development Workflow

```bash
# Format code
make format

# Lint code
make lint
# or just Go lint
make go-lint

# Fix import formatting
make fix-goimports

# Download/update dependencies
make modules

# Generate code (after modifying types)
make generate

# Generate manifests (CRDs, RBAC, webhooks)
make manifests

# Complete workflow: clean, format, modules, test, build
make all
```

### Local Development Environment

```bash
# Start local environment with Minikube and Tilt
ctlptl create registry ctlptl-registry --port=5005
ctlptl create cluster minikube --registry=ctlptl-registry
tilt up
```

### Tool Management

```bash
# Download all development tools
make tools

# Individual tools
make controller-gen    # Generate CRDs and webhooks
make golangci-lint     # Linter
make helm              # Helm CLI
make setup-envtest     # Test environment setup
make goimports         # Import formatter
```

## Architecture Overview

### Component Hierarchy

The operator consists of three main layers:

1. **Admission Webhooks** (entry point for pod instrumentation)
   - `internal/webhook/PodMutationHandler` - intercepts pod creation at `/mutate-v1-pod`
   - API validation webhooks for `Instrumentation` CRDs (all versions)
   - Failure policy: `ignore` (never blocks pod creation, even on errors)

2. **Controllers** (reconciliation loops for health monitoring)
   - `InstrumentationReconciler` - watches Instrumentation CRs in operator namespace
   - `PodReconciler` - watches pods across all namespaces (except system namespaces)
   - `NamespaceReconciler` - watches namespace lifecycle
   - All feed events to central `HealthMonitor`

3. **Instrumentation Pipeline** (mutation logic)
   - `internal/instrumentation/mutator.go` - orchestrates instrumentation discovery and validation
   - `internal/instrumentation/sdk.go` - delegates to language-specific injectors
   - `internal/apm/` - language-specific injector implementations

### How Pod Instrumentation Works

```
Pod Creation → Webhook Intercepts → Match Instrumentation → Validate → Replicate Secrets/ConfigMaps
→ Language Injector Adds Init Container + Volume → Mutated Pod Spec Returned
```

**Key Mechanism:** Init containers copy agent artifacts to shared emptyDir volume, target container mounts volume and loads agent via environment variables.

### Health Monitoring Architecture

Event-driven system that updates Instrumentation status with pod health:

```
Controllers → Emit Events → HealthMonitor (worker pools) → Periodic Health Checks (15s)
→ Call Health Sidecar /healthz → Update Instrumentation.Status
```

- 50 concurrent workers for pod events
- 50 concurrent workers for instrumentation events
- Ticker-based health checks every 15 seconds
- Non-blocking status updates via `InstrumentationStatusUpdater`

### API Version Strategy

Multiple versions coexist with `api/current/current.go` aliasing v1beta3:

- **Internal code** uses `api/current` types (version-agnostic)
- **Conversion** happens at webhook/API boundaries
- **Add new versions** by copying existing API package and updating current alias

### Language Support Plugin System

Registry-based design in `internal/apm/injector.go`:

```go
// Each language registers its injector in init()
DefaultInjectorRegistry.MustRegister("java", NewJavaInjector())
```

**To add a new language:**
1. Create `internal/apm/<language>.go` implementing `ContainerInjector` interface
2. Register in `init()` function
3. Add language to validation in instrumentation webhooks
4. Update documentation

### Container Selection Logic

Four-layer matching system:

1. **Namespace selector** - `spec.namespaceLabelSelector` (label selector on namespace)
2. **Pod selector** - `spec.podLabelSelector` (label selector on pod)
3. **Container selector** - `spec.containerSelector` (one or more of):
   - `envSelector` - match by environment variable keys/values
   - `imageSelector` - match by image URL patterns (StartsWith, Contains, etc.)
   - `nameSelector` - match by container name (init vs regular vs any)
   - `namesFromPodAnnotation` - match by comma-separated annotation value
4. **Validation** - ensures no conflicts (single language per container, single license key, etc.)

### Important Packages

| Package | Purpose |
|---------|---------|
| `cmd/` | Main entry point, manager setup |
| `api/v1beta3/` | Current Instrumentation CRD types |
| `api/current/` | Version-agnostic alias to v1beta3 |
| `internal/webhook/` | Pod mutation webhook handler |
| `internal/instrumentation/` | Core mutation, health monitoring, status updates |
| `internal/apm/` | Language-specific agent injectors |
| `internal/controller/` | Pod, Namespace, Instrumentation reconcilers |
| `internal/selector/` | Generic selector matching logic |
| `internal/autodetect/` | Platform capability detection (OpenShift, HPA versions) |
| `internal/migrate/upgrade/` | One-time upgrade mechanism for managed instances |

## Development Guidelines

### Adding New Language Support

1. Create `internal/apm/<language>.go` with struct implementing `ContainerInjector`:
   ```go
   type LanguageInjector struct{}

   func (i *LanguageInjector) Language() string { return "language-name" }
   func (i *LanguageInjector) AcceptsContainerImage(image string) bool { /* logic */ }
   func (i *LanguageInjector) InjectContainer(ctx context.Context, req InjectContainerRequest) (*InjectContainerResponse, error) { /* mutation logic */ }
   ```

2. Register in `init()`:
   ```go
   func init() {
       DefaultInjectorRegistry.MustRegister("language-name", NewLanguageInjector())
   }
   ```

3. Update validation in `api/v1beta3/instrumentation_webhook.go` (and other versions if needed)

4. Add default image mapping in relevant code

### Modifying CRD Types

After changing types in `api/v1beta3/`:

```bash
make generate    # Generate DeepCopy methods
make manifests   # Generate CRD YAML
```

The generated CRDs go to `config/crd/bases/`. Helm chart CRDs must be manually updated from there.

### Testing Patterns

- Unit tests use `envtest` (setup-envtest tool) for fake Kubernetes API server
- Tests are in packages alongside source files
- E2E tests in `tests/e2e/` use real Minikube clusters
- Run specific package tests: `go test ./internal/apm -v`

### Webhook Development

Pod mutation webhook critical rules:

1. **Never block pod creation** - return `Allowed: true` even on errors (status 500)
2. **Validate early** - check all constraints before modifying pod spec
3. **Log failures clearly** - operators need visibility into why instrumentation failed
4. **Handle edge cases** - missing namespaces, deleted secrets, etc.

The webhook path is `/mutate-v1-pod` and registered in manager setup.

### Health Monitoring Development

When modifying health monitoring (`internal/instrumentation/health.go`):

- Events are asynchronous (pushed to buffered channels)
- Worker pools process events concurrently (50 workers per type)
- Ticker triggers periodic checks (15s interval)
- Status updates must be non-blocking
- Handle race conditions (pods/instrumentations deleted during check)

### Secret and ConfigMap Replication

License keys and agent configs are replicated from operator namespace to target namespace:

- Naming convention: `newrelic-instrumentation-<instrumentation-name>-<original-name>`
- Deletion: not automatic (operator doesn't clean up on Instrumentation deletion)
- Updates: not propagated (requires manual cleanup and recreation)

## Important Architectural Decisions

### Failure-Safe Webhook Design

The pod mutation webhook has `failurePolicy=ignore` and returns `Allowed=true` even on internal errors. This ensures operator bugs never prevent application deployments. Trade-off: silent failures require log monitoring.

### Event-Driven vs Polling

Health monitoring uses event-driven architecture (controllers push events) rather than polling all pods continuously. This reduces API server load and enables real-time status updates.

### Init Container Pattern

Agents are injected via init containers rather than modifying application images. This enables:
- Immutable application containers
- Agent version updates without redeploying apps
- Support for arbitrary base images

### Operator Namespace Isolation

Instrumentation CRs live in operator namespace only, but select pods across cluster. This prevents:
- Users creating Instrumentation resources in arbitrary namespaces
- Operator instrumenting itself or system components
- RBAC complexity (operator has limited write permissions outside its namespace)

### Multi-Version API Support

Four API versions coexist without conversion webhooks. Versions are structurally compatible, and `api/current` package provides facade. This simplifies internal code while maintaining backward compatibility.

## Code Formatting and Style

- Use `goimports` with `-local github.com/newrelic/` for import grouping
- Run `make format` before committing
- Follow standard Go conventions (exported names documented, errors checked)
- Use structured logging via `logr` interface (provided by controller-runtime)

## Debugging Tips

### Local Development

Tilt provides live reload for code changes. View logs:
```bash
tilt up  # Opens browser UI showing all resources
tilt logs operator  # View operator logs directly
```

### Webhook Debugging

Webhook failures are logged in operator pod logs. Look for:
- `"PodMutationHandler failed"` - webhook handler errors
- `"failed to mutate pod"` - instrumentation mutation errors
- `"validation failed"` - Instrumentation CR validation errors

Get webhook configuration:
```bash
kubectl get mutatingwebhookconfiguration
kubectl get validatingwebhookconfiguration
```

### Health Monitoring Debugging

Health status visible in Instrumentation CR:
```bash
kubectl get instrumentation -n <operator-namespace> <name> -o yaml
# Check .status.podsMatching, .status.podsInjected, .status.unhealthyPods
```

### Common Issues

**Pods not being instrumented:**
- Check Instrumentation selectors match pod labels
- Verify license key secret exists in operator namespace
- Check operator logs for webhook errors
- Verify webhook service is running: `kubectl get svc -n <operator-namespace>`

**Agent not loading in container:**
- Check init container ran successfully: `kubectl describe pod <pod-name>`
- Verify volume mount exists: `kubectl get pod <pod-name> -o yaml`
- Check environment variables were injected
- Review language-specific requirements (e.g., Java needs JAVA_TOOL_OPTIONS)

## Testing Requirements

When making changes:

1. Run `make test` locally before pushing
2. Ensure `make lint` passes
3. For API changes, test across all supported k8s versions: `make all-go-tests`
4. For webhook changes, run e2e tests: `make e2e-tests`
5. Update both unit tests and e2e tests if adding new features

Coverage reports: `make coverprofile`

## Release Process

The operator version is set via build flag:
```bash
K8S_AGENTS_OPERATOR_VERSION=v1.2.3 make build
```

Version is embedded in binary via ldflags in Makefile. CI/CD pipeline handles Helm chart versioning automatically.
