# Instrumentation (v1beta2)

The instrumentation is the specification to match specific pods and containers, instrumenting them with the newrelic
agent by mutating the pod via admission webhooks.  Typically, it means adding an init container to the pod (before the
selected container) with contains the agent, which is then copied to a temporary volume, and the selected container will
get a mounted volume (the temp volume) and the environment altered so that it loads the agent.  If something needs to be
changed in the instrumentation, the pod will need to be deleted before it can be mutated again.

- [Instrumentation (v1beta2)](#instrumentation-v1beta2)
  - [namespaceSelector](#namespaceselector)
  - [podSelector](#podselector)
  - [containerSelector](#containerselector)
      - [Targeting containers using the classic approach](#targeting-containers-using-the-classic-approach)
  - [envSelector](#envselector)
    - [Targeting containers by container environment variables](#targeting-containers-by-container-environment-variables)
  - [imageSelector](#imageselector)
    - [Targeting containers by container image](#targeting-containers-by-container-image)
  - [nameSelector](#nameselector)
    - [Targeting containers by name](#targeting-containers-by-name)
  - [namesFromPodAnnotation](#namesfrompodannotation)
    - [Targeting container by an annotation on the pod and instrumentation](#targeting-container-by-an-annotation-on-the-pod-and-instrumentation)
  - [agent](#agent)
  - [agent language](#agent-language)
  - [agent image](#agent-image)
  - [agent imagePullPolicy](#agent-imagepullpolicy)
  - [agent env](#agent-env)
  - [agent resourceRequirements](#agent-resourcerequirements)
  - [agent securityContext](#agent-securitycontext)
  - [healthAgent](#healthagent)
  - [healthAgent image](#healthagent-image)
  - [healthAgent imagePullPolicy](#healthagent-imagepullpolicy)
  - [healthAgent env](#healthagent-env)
  - [healthAgent resourceRequirements](#healthagent-resourcerequirements)
  - [healthAgent securityContext](#healthagent-securitycontext)
  - [licenseKeySecret](#licensekeysecret)
  - [agentConfigMap](#agentconfigmap)

```yaml
apiVersion: newrelic.com/v1beta2
kind: Instrumentation
metadata:
  name: example
spec:
  namespaceLabelSelector:
    matchLabels:
      some-label: some-value
    matchExpressions:
      - key: "kubernetes.io/metadata.name"
        operator: "In"
        values: ["name-of-my-namespace"]
  podLabelSelector:
    matchLabels:
      some-label: some-value
    matchExpressions:
      - key: "app.kubernetes.io/name"
        operator: "In"
        values: ["name-of-my-app"]
  containerSelector:
    envSelector:
      matchEnvs:
        SOME_ENV: value
      matchExpressions:
      - key: SOME_ENV
        operator: Exists
    imageSelector:
      matchImages:
        url: ruby:latest
      matchExpressions:
        - key: url
          operator: StartsWith
          values: ["ghcr.io/"]
    nameSelector:
      matchNames:
        anyContainer: my-app
      matchExpressions:
        - key: initContainer
          operator: In
          values: ["my-app1","my-app2"]
    namesFromPodAnnotation: some-annotation
  agent:
    env:
      - name: NEW_RELIC_SOME_VAR
        value: some-value
    language: java
    image: newrelic/newrelic-java-init:latest
    imagePullPolicy: IfNotPresent
    resourceRequirements:
      limits:
        cpu: "1"
        memory: "32Mi"
      requests:
        cpu: "0.5"
        memory: "16Mi"
    securityContext:
      allowPrivilegeEscalation: false
      readOnlyRootFilesystem: true
      runAsUser: 5678
  healthAgent:
    env:
      - name: NEW_RELIC_SOME_VAR
        value: some-value
    image: newrelic/k8s-apm-agent-health-sidecar:latest
    imagePullPolicy: Never
    resourceRequirements:
      limits:
        cpu: "1"
        memory: "32Mi"
      requests:
        cpu: "0.5"
        memory: "16Mi"
    securityContext:
      allowPrivilegeEscalation: false
      readOnlyRootFilesystem: true
      runAsUser: 1234
  licenseKeySecret: secret-name
  agentConfigMap: configmap-name
```

## namespaceSelector

Used for selecting pods based on the namespace labels.  See https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/

## podSelector

Used for selecting pods based on the pod labels.  See https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/

## containerSelector

Supports different ways of selecting containers for automatic instrumentation

#### Targeting containers using the classic approach

Without a `containerSelector` defined, the first (non-init) container defined in the pod will be instrumented.

- [envSelector](#envSelector) - select containers based on the container environment variables
- [imageSelector](#imageSelector) - select containers based on the container image url
- [nameSelector](#nameSelector) - select containers based on the container names, and if they are init containers or regular containers
- [namesFromPodAnnotation](#namesFromPodAnnotation) - select containers based on an annotation key and value in the pod and instrumentation

## envSelector


### Targeting containers by container environment variables

Containers can be selected based on the environment variables, limited to `.env` in the pod spec, and does not resolve `valueFrom` of any kind.  It also does not resolve any runtime environment variables that may have been set in the container image.
The supported keys could be any environment key that a container may have.
The supported values could be any environment variable value that it may have.

| supported operators |
|---------------------|
| ==                  |
| !=                  |
| In                  |
| NotIn               |
| Exists              |
| NotExists           |

<details>
<summary>example instrumentation using envSelector and matchEnvs</summary>

```yaml
apiVersion: newrelic.com/v1beta2
kind: Instrumentation
metadata:
  name: select-by-match-envs
spec:
  containerSelector:
    envSelector:
      matchEnvs:
        DEBUG: 'true'
#...
```

</details>



<details>
<summary>example instrumentation using envSelector and matchExpressions</summary>

```yaml
apiVersion: newrelic.com/v1beta2
kind: Instrumentation
metadata:
  name: select-by-any-container
spec:
  containerSelector:
    envSelector:
      matchExpressions:
        - key: SOMETHING
          operator: ==
          values: here
#...
```

</details>


<details>
<summary>example instrumentation using envSelector and matchExpressions, to instrument anything that does not opt out</summary>

```yaml
apiVersion: newrelic.com/v1beta2
kind: Instrumentation
metadata:
  name: select-by-any-container
spec:
  containerSelector:
    envSelector:
      matchExpressions:
        - key: NEW_RELIC_DONT_INSTRUMENT
          operator: NotExists
#...
```

</details>


## imageSelector

### Targeting containers by container image

Containers can be selected based on the url of the container image.

| supported keys |
|----------------|
| url            |

| supported operators |
|---------------------|
| ==                  |
| !=                  |
| In                  |
| NotIn               |
| StartsWith          |
| NotStartsWith       |
| EndsWith            |
| NotEndsWith         |
| Contains            |
| NotContains         |

<details>
<summary>example instrumentation using imageSelector and matchImages</summary>

```yaml
apiVersion: newrelic.com/v1beta2
kind: Instrumentation
metadata:
  name: select-by-any-container
spec:
  containerSelector:
    imageSelector:
      matchImages:
        url: php:8.4-alpine
#...
```

</details>

<details>
<summary>example instrumentation using imageSelector and matchExpressions</summary>

```yaml
apiVersion: newrelic.com/v1beta2
kind: Instrumentation
metadata:
  name: select-by-any-container
spec:
  containerSelector:
    imageSelector:
      matchExpressions:
        - key: url
          operator: StartsWith
          values: ["docker.io/"]
#...
```

</details>

## nameSelector

### Targeting containers by name

Containers can be selected based on if it's an init container, regular container, or either.
The values would be the names of the containers to select ot not.

| supports keys |
|---------------|
| initContainer |
| container     |
| anyContainer  |

| supported operators |
|---------------------|
| ==                  |
| !=                  |
| In                  |
| NotIn               |

<details>
<summary>example instrumentation using matchNames and anyContainer</summary>

```yaml
apiVersion: newrelic.com/v1beta2
kind: Instrumentation
metadata:
  name: select-by-match-names-and-any-container
spec:
  containerSelector:
    nameSelector:
      matchNames:
        anyContainer: app
#...
```

> This would match only the container named `app`, as the `app2` container has a different name

```yaml
#... pod spec
spec:
  initContainers:
    - name: app
      #...
  containers:
    - name: app2
#...
```

</details>



<details>
<summary>example instrumentation using matchExpressions and initContainer</summary>

```yaml
apiVersion: newrelic.com/v1beta2
kind: Instrumentation
metadata:
  name: select-by-init-container
spec:
  containerSelector:
    nameSelector:
      matchNames:
        type_of_container: value
      matchExpressions:
        - key: initContainer
          operator: In
          values: ["first", "second"]
#...
```

> This would match only the container named `first`, as the `second` container is a regular container

```yaml
#... pod spec
spec:
  initContainers:
    - name: first
      #...
  containers:
    - name: second
#...
```

</details>


<details>
<summary>example instrumentation using matchExpressions and container</summary>

```yaml
apiVersion: newrelic.com/v1beta2
kind: Instrumentation
metadata:
  name: select-by-container
spec:
  containerSelector:
    nameSelector:
      matchExpressions:
        - key: container
          operator: In
          values: ["alpha", "beta"]
#...
```

> This would match only the container named `beta`, as the `alpha` container is an init container
```yaml
#...
spec:
  initContainers:
    - name: alpha
      #...
  containers:
    - name: beta
      #...
```
</details>


<details>
<summary>example instrumentation using matchExpressions and anyContainer</summary>

```yaml
apiVersion: newrelic.com/v1beta2
kind: Instrumentation
metadata:
  name: select-by-any-container
spec:
  containerSelector:
    nameSelector:
      matchExpressions:
        - key: anyContainer
          operator: In
          values: ["one", "two"]
```

> This would match both containers `one` and `two`.
>
```yaml
apiVersion: newrelic.com/v1beta2
kind: Instrumentation
metadata:
  name: select-by-any-container
spec:
  initContainers:
    - name: one
      #...
  containers:
    - name: two
      #...
    - name: three
      #...
```
</details>

## namesFromPodAnnotation

### Targeting container by an annotation on the pod and instrumentation

Containers can be selected based on an annotation defined in the pod, the name/key of the annotation to be used would be defined in the instrumentation.
Multiple containers can be selected by separating the names with a comma (,).

<details>
<summary>example instrumentation using namesFromPodAnnotation</summary>

```yaml
apiVersion: newrelic.com/v1beta2
kind: Instrumentation
metadata:
  name: select-by-names-from-pod-annotation
spec:
  containerSelector:
    namesFromPodAnnotation: use-these-for-newrelic
#...
```

> This would match the containers named `a` and `c`.

```yaml
#... pod spec
metadata:
  annotations:
    use-these-for-newrelic: a,c
spec:
  initContainers:
    - name: a
      #.. more container spec ..
  containers:
    - name: b
      # ...
    - name: c
#...
```

</details>

## agent

- [language](#agent-language)
- [image](#agent-image)
- [imagePullPolicy](#agent-imagePullPolicy)
- [env](#agent-env)
- [resourceRequirements](#agent-resourceRequirements)
- [securityContext](#agent-securityContext)

## agent language

The supported language agents.  PHP being an exception, and currently requires specifying a version suffix.

| languages           |
|---------------------|
| dotnet              |
| dotnet-windows2022  |
| dotnet-windows2025  |
| java                |
| nodejs              |
| php-7.2             |
| php-7.3             |
| php-7.4             |
| php-8.0             |
| php-8.1             |
| php-8.2             |
| php-8.3             |
| php-8.4             |
| python              |
| ruby                |

## agent image

The supported images.  Each image requires at least `cp` (Linux) or `xcopy` (Windows) to copy the agent to a temporary volume; php being an exception and requires a shell.
Custom-built images can be used and specified.

| language            | official image                                   | arch        | os      | libc  |
|---------------------|--------------------------------------------------|-------------|---------|-------|
| dotnet              | newrelic/newrelic-dotnet-init:latest             |             | linux   |       |
| dotnet-windows2022  | newrelic/newrelic-dotnet-windows2022-init:latest | amd64       | windows |       |
| dotnet-windows2025  | newrelic/newrelic-dotnet-windows2025-init:latest | amd64       | windows |       |
| java                | newrelic/newrelic-java-init:latest               |             | linux   |       |
| nodejs              | newrelic/newrelic-node-init:latest               |             | linux   | glibc |
| php-7.2             | newrelic/newrelic-php-init:latest                | amd64       | linux   | glibc |
| php-7.3             | newrelic/newrelic-php-init:latest                | amd64       | linux   | glibc |
| php-7.4             | newrelic/newrelic-php-init:latest                | amd64       | linux   | glibc |
| php-8.0             | newrelic/newrelic-php-init:latest                | amd64,arm64 | linux   | glibc |
| php-8.1             | newrelic/newrelic-php-init:latest                | amd64,arm64 | linux   | glibc |
| php-8.2             | newrelic/newrelic-php-init:latest                | amd64,arm64 | linux   | glibc |
| php-8.3             | newrelic/newrelic-php-init:latest                | amd64,arm64 | linux   | glibc |
| php-8.4             | newrelic/newrelic-php-init:latest                | amd64,arm64 | linux   | glibc |
| php-7.2             | newrelic/newrelic-php-init:musl                  | amd64       | linux   | musl  |
| php-7.3             | newrelic/newrelic-php-init:musl                  | amd64       | linux   | musl  |
| php-7.4             | newrelic/newrelic-php-init:musl                  | amd64       | linux   | musl  |
| php-8.0             | newrelic/newrelic-php-init:musl                  | amd64,arm64 | linux   | musl  |
| php-8.1             | newrelic/newrelic-php-init:musl                  | amd64,arm64 | linux   | musl  |
| php-8.2             | newrelic/newrelic-php-init:musl                  | amd64,arm64 | linux   | musl  |
| php-8.3             | newrelic/newrelic-php-init:musl                  | amd64,arm64 | linux   | musl  |
| php-8.4             | newrelic/newrelic-php-init:musl                  | amd64,arm64 | linux   | musl  |
| python              | newrelic/newrelic-python-init:latest             |             | linux   |       |
| ruby                | newrelic/newrelic-ruby-init:latest               |             | linux   |       |

## agent imagePullPolicy

Sets the image pull policy for the init container that will be added to the pod during mutation. See https://kubernetes.io/docs/concepts/containers/images/

## agent env

Sets the env for the init container that will be added to the pod during mutation. See https://kubernetes.io/docs/tasks/inject-data-application/define-environment-variable-container/

> Each `name` must start with `NEWRELIC_` or `NEW_RELIC_`

## agent resourceRequirements

Sets the resources for the init container that will be added to the pod during mutation.  See https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/

## agent securityContext

Sets the security context for the init container that will be added to the pod during mutation.  See https://kubernetes.io/docs/tasks/configure-pod-container/security-context/

## healthAgent

- [image](#healthAgent-image)
- [imagePullPolicy](#healthAgent-imagePullPolicy)
- [env](#healthAgent-env)
- [resourceRequirements](#healthAgent-resourceRequirements)
- [securityContext](#healthAgent-securityContext)

## healthAgent image

The health agent is used to report the health of the agent that was injected into the target container.  Only a one
health agent can be added to a pod, and therefor only a single container can report the agent health.

The operator will collect the health from the pods periodically, when will update the instrumentation status with some
health stats.

The supported images.

| official image                               | arch        | os    |
|----------------------------------------------|-------------|-------|
| newrelic/k8s-apm-agent-health-sidecar:latest | amd64,arm64 | linux |

## healthAgent imagePullPolicy

Sets the image pull policy for the sidecar container that will be added to the pod during mutation. See https://kubernetes.io/docs/concepts/containers/images/

## healthAgent env

Sets the env for the sidecar container that will be added to the pod during mutation. See https://kubernetes.io/docs/tasks/inject-data-application/define-environment-variable-container/

> Each `name` must start with `NEWRELIC_` or `NEW_RELIC_`

## healthAgent resourceRequirements

Sets the resources for the sidecar container that will be added to the pod during mutation.  See https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/

## healthAgent securityContext

Sets the security context for the sidecar container that will be added to the pod during mutation.  See https://kubernetes.io/docs/tasks/configure-pod-container/security-context/

## licenseKeySecret

Not required.  When blank, it will use the secret `newrelic-key-secret` in the operator's namespace.  It uses the field 
`new_relic_license_key`.  The license key will be copied from the operator namespace to the pods namespace during pod 
startup.  License keys are not updated, and would require all the cloned license key secrets to be removed if they needed to be updated.

## agentConfigMap

This is used to set properties for the agent, without setting them through the environment.

> Only supported with the agent language `java`.

<details>
<summary>example configmap</summary>

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: java-cm
data:
  newrelic.yaml: |
    production:
      x: y
```

</details>
