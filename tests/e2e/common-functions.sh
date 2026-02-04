#!/usr/bin/env bash
# Common functions for e2e tests
# Source this file from individual test scripts

# Wait for operator to be fully ready
function wait_for_operator_ready() {
    local namespace=$1

    echo "🔄 Waiting for operator pod to be ready"
    # Wait for pod to be running
    kubectl wait --timeout=60s --for=jsonpath='{.status.phase}'=Running \
      -n ${namespace} -l="app.kubernetes.io/instance=k8s-agents-operator" pod

    # Wait for pod to be fully ready (all containers ready)
    kubectl wait --timeout=120s --for=condition=Ready \
      -n ${namespace} -l="app.kubernetes.io/instance=k8s-agents-operator" pod

    echo "🔄 Waiting for webhook service endpoints to be ready"
    # Wait for endpointslices to exist
    until kubectl get endpointslices -n ${namespace} \
      -l kubernetes.io/service-name=k8s-agents-operator-webhook-service 2>/dev/null | grep -q k8s-agents-operator; do
      echo "  Waiting for endpointslices to be created..."
      sleep 2
    done

    # Wait for endpoints to be ready
    kubectl wait -n ${namespace} \
      --selector=kubernetes.io/service-name=k8s-agents-operator-webhook-service \
      endpointslices --timeout=60s --for=jsonpath='{.endpoints[].conditions.ready}=true'

    # Wait for endpoints to be serving
    kubectl wait -n ${namespace} \
      --selector=kubernetes.io/service-name=k8s-agents-operator-webhook-service \
      endpointslices --timeout=60s --for=jsonpath='{.endpoints[].conditions.serving}=true'

    # Verify webhook configurations are properly configured with CA bundles
    echo "🔄 Verifying webhook configurations"
    until kubectl get validatingwebhookconfigurations k8s-agents-operator-validating-webhook-configuration \
      -o jsonpath='{.webhooks[0].clientConfig.caBundle}' 2>/dev/null | grep -q '.'; do
      echo "  Waiting for validating webhook CA bundle to be injected..."
      sleep 2
    done

    until kubectl get mutatingwebhookconfigurations k8s-agents-operator-mutating-webhook-configuration \
      -o jsonpath='{.webhooks[0].clientConfig.caBundle}' 2>/dev/null | grep -q '.'; do
      echo "  Waiting for mutating webhook CA bundle to be injected..."
      sleep 2
    done

    echo "✅ Operator is ready"
}

# Wait for instrumentations to be created and established
function wait_for_instrumentations() {
    local operator_namespace=$1
    local instrumentation_files_pattern=$2

    echo "🔄 Waiting for instrumentations to be established"
    # Give the operator time to process the CRDs and set up watches
    sleep 3

    # Verify instrumentations were created successfully
    for i in $(find $(dirname "$instrumentation_files_pattern") -maxdepth 1 -type f -name "$(basename "$instrumentation_files_pattern")"); do
      inst_name=$(yq '.metadata.name' $i 2>/dev/null || grep 'name:' $i | head -1 | awk '{print $2}')
      if [ -n "$inst_name" ]; then
        echo "  Verifying instrumentation: $inst_name"
        until kubectl get instrumentation -n ${operator_namespace} $inst_name 2>/dev/null; do
          echo "    Waiting for instrumentation $inst_name to be created..."
          sleep 1
        done
      fi
    done

    echo "✅ Instrumentations are established"
}

# Wait for deployments and pods to be ready
function wait_for_apps_ready() {
    local app_namespace=$1
    local apps_path=$2

    echo "🔄 Waiting for app deployments to be available"
    for deployment in $(find ${apps_path} -type f -name '*.yaml' -exec yq '. | select(.kind == "Deployment") | .metadata.name' {} \; 2>/dev/null); do
      if [ -n "$deployment" ]; then
        echo "  Waiting for deployment: $deployment"
        kubectl wait --timeout=300s --for=condition=Available \
          --namespace ${app_namespace} deployment/$deployment
      fi
    done

    echo "🔄 Waiting for all app pods to be ready"
    for label in $(find ${apps_path} -type f -name '*.yaml' -exec yq '. | select(.kind == "Deployment") | .metadata.name' {} \; 2>/dev/null); do
      if [ -n "$label" ]; then
        echo "  Waiting for pods with label app=$label"
        # Wait for pod to exist first
        local retries=30
        until kubectl get pod -n ${app_namespace} -l="app=$label" 2>/dev/null | grep -q "$label"; do
          echo "    Waiting for pod with label app=$label to be created..."
          sleep 2
          retries=$((retries-1))
          if [ $retries -le 0 ]; then
            echo "    ❌ Timeout waiting for pod with label app=$label"
            return 1
          fi
        done

        # Wait for pod to be ready (not just running, but all containers ready)
        kubectl wait --timeout=600s --for=condition=Ready \
          --namespace ${app_namespace} -l="app=$label" pod

        # Verify pod has init containers (instrumentation was applied)
        local pod_name=$(kubectl get pod -n ${app_namespace} -l="app=$label" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
        if [ -n "$pod_name" ]; then
          local init_count=$(kubectl get pod -n ${app_namespace} $pod_name -o jsonpath='{.spec.initContainers}' 2>/dev/null | grep -o 'name' | wc -l | tr -d ' ')
          echo "    Pod $pod_name has $init_count init container(s)"
        fi
      fi
    done

    echo "✅ All apps are ready and instrumented"
}
