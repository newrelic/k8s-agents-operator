#!/usr/bin/env bash
set -euo pipefail

# Test cluster
CLUSTER_NAME=""
K8S_VERSION=""

# New Relic account (production) details
LICENSE_KEY=""

# Unset if you only want to setup a test cluster with E2E specifications
# Set to true if you additionally want to run tests
RUN_TESTS=""

SCRIPT_PATH=$(dirname $0)
REPO_ROOT=$(realpath $SCRIPT_PATH/../../..)

function main() {
    parse_args "$@"

    if test $(echo $K8S_VERSION | awk -F. '{print $1}') == "v1" && test $(echo $K8S_VERSION | awk -F. '{print $2}') -le "28"; then
      echo "⚠️ cluster skipped because the version $K8S_VERSION does not have native sidecars support, which is required; introduced as a feature flag in v1.28.0 disabled by default, and enabled by default in v1.29.0 or greater"
      exit 0
    fi

    create_cluster
    if [[ "$RUN_TESTS" == "true" ]]; then
        run_tests
        teardown
    fi
}

function parse_args() {
    totalArgs=$#

    # Arguments are passed by value, so other functions
    # are not affected by destructive processing
    while [[ $# -gt 0 ]]; do
        case $1 in
            --help)
            help
            exit 0
            ;;

            --k8s_version|--k8s-version)
            shift
            K8S_VERSION="$1"
            ;;
            --k8s_version=*|--k8s-version=*)
            K8S_VERSION="${1#*=}"
            ;;

            --license_key|--license-key)
            shift
            LICENSE_KEY="$1"
            ;;
            --license_key=*|--license-key=*)
            LICENSE_KEY="${1#*=}"
            ;;

            --run_tests|--run-tests)
            RUN_TESTS="true"
            ;;
            -*|--*|*)
            echo "Unknown field: $1"
            exit 1
            ;;
        esac
        shift
    done

    if [[ totalArgs -lt 4 ]]; then
        help
        exit 0
    fi
}

function help() {
    cat <<END
 Usage:
 ${0##*/}    --k8s_version <cluster_version>
             --license_key <license_key>
             [--run_tests]

 --k8s_version:  valid Kubernetes cluster version. It is highly recommended to use same versions as E2E tests
 --license_key:  key type 'INGEST - LICENSE'
 --run_tests:    if unset, create a cluster with specifications matching E2E tests
                 otherwise run tests in addition to setting up cluster
END
}

function create_cluster() {
    echo "🔄 Setup"
    minikube delete --all > /dev/null
    now=$( date "+%Y-%m-%d-%H-%M-%S" )
    CLUSTER_NAME=${now}-e2e-tests

    echo "🔄 Creating cluster ${CLUSTER_NAME}"
    minikube start --container-runtime=containerd --kubernetes-version=${K8S_VERSION} --profile ${CLUSTER_NAME} > /dev/null

    echo "🔄 Building Docker image"
    DOCKER_BUILDKIT=1 docker build --tag e2e/k8s-agents-operator:e2e ${REPO_ROOT} --quiet > /dev/null

    echo "🔄 Loading operator image into cluster"
    minikube image load e2e/k8s-agents-operator:e2e --profile ${CLUSTER_NAME} > /dev/null

    echo "🔄 Loading dotnet init image into cluster"
    minikube image load --profile ${CLUSTER_NAME} newrelic/newrelic-dotnet-init:latest

    echo "🔄 Loading health agent sidecar image into cluster"
    minikube image load --profile ${CLUSTER_NAME} newrelic/k8s-apm-agent-health-sidecar:latest

    echo "🔄 Adding Helm repositories"
    helm repo add newrelic https://helm-charts.newrelic.com > /dev/null
    helm repo update > /dev/null
    helm dependency update ${REPO_ROOT}/charts/k8s-agents-operator > /dev/null

    echo "🔄 Installing operator"
    helm upgrade --install k8s-agents-operator ${REPO_ROOT}/charts/k8s-agents-operator \
      --namespace k8s-agents-operator \
      --create-namespace \
      --set controllerManager.manager.image.version=e2e,controllerManager.manager.image.pullPolicy=Never,controllerManager.manager.image.repository=e2e/k8s-agents-operator \
      --set licenseKey=${LICENSE_KEY}

    echo "🔄 Waiting for operator to settle"
    sleep 15
    kubectl wait --timeout=30s --for=jsonpath='{.status.phase}'=Running -n k8s-agents-operator -l="app.kubernetes.io/instance=k8s-agents-operator" pod
    sleep 15

    echo "🔄 Creating E2E namespace"
    kubectl create namespace e2e-namespace

    echo "🔄 Installing instrumentation"
    for i in $(find ${SCRIPT_PATH} -maxdepth 1 -type f -name 'e2e-instrumentation-*.yml'); do
      kubectl apply --namespace k8s-agents-operator --filename $i
    done

    echo "🔄 Installing apps"
    kubectl apply --namespace e2e-namespace --filename ${SCRIPT_PATH}/apps/

    echo "🔄 Waiting for apps to settle"
    for label in $(find ${SCRIPT_PATH}/apps -type f -name '*.yaml' -exec yq '. | select(.kind == "Deployment") | .metadata.name' {} \;); do
      kubectl wait --timeout=300s --for=jsonpath='{.status.phase}'=Running --namespace e2e-namespace -l="app=$label" pod
    done
}

function run_tests() {
    echo "🔄 Starting E2E tests"
    initContainers=$(kubectl get pods --namespace e2e-namespace --output yaml | yq '.items[].spec.initContainers[].name' | wc -l)
    local expected=$(ls ${SCRIPT_PATH}/apps | wc -l)
    if [[ ${initContainers} -lt $expected ]]; then
      echo "❌ Error: not all apps were instrumented. Expected $expected, got ${initContainers}"
      exit 1
    fi

    if command -v yq > /dev/null 2> /dev/null; then
      test_dotnet
      test_java
      test_nodejs
      test_php
      test_python
      test_ruby
    else
      echo "⚠️ test skipped because 'yq' is missing"
    fi

    echo "✅  Success: all apps were instrumented"
}

# ensure that the init container gets resource requirements, that the health sidecar exists, that the health sidecar get resource requirements assigned
# that the imagePullPolicy gets assigned, that the securityContext gets assigned
function test_dotnet() {
  if ! test $(kubectl get pods -l app=dotnetapp -n e2e-namespace -o yaml | yq '.items[0].spec | .initContainers + .containers | .[]| select(.name == "nri-dotnet--dotnetapp") | .resources' -I0 -o json) == '{"limits":{"cpu":"1","memory":"32Mi"},"requests":{"cpu":"500m","memory":"16Mi"}}'; then
    echo "unexpected resources for container nri-dotnet--dotnetapp"
    exit 1
  fi
  if ! test $(kubectl get pods -l app=dotnetapp -n e2e-namespace -o yaml | yq '.items[0].spec | .initContainers + .containers | .[]| select(.name == "newrelic-apm-health-sidecar") | .resources' -I0 -o json) == '{"limits":{"cpu":"1500m","memory":"48Mi"},"requests":{"cpu":"750m","memory":"24Mi"}}'; then
    echo "unexpected resources for container newrelic-apm-health-sidecar"
    exit 1
  fi
  if ! test $(kubectl get pods -l app=dotnetapp -n e2e-namespace -o yaml | yq '.items[0].spec | .initContainers + .containers | .[]| select(.name == "alpine") | .resources' -I0 -o json) == '{}'; then
    echo "unexpected resources for container alpine"
    exit 1
  fi
  if ! test $(kubectl get pods -l app=dotnetapp -n e2e-namespace -o yaml | yq '.items[0].spec | .initContainers + .containers | .[]| select(.name == "dotnetapp") | .resources' -I0 -o json) == '{}'; then
    echo "unexpected resources for container dotnetapp"
    exit 1
  fi

  if ! test $(kubectl get pods -l app=dotnetapp -n e2e-namespace -o yaml | yq '.items[0].spec | .initContainers + .containers | .[]| select(.name == "nri-dotnet--dotnetapp") | .securityContext' -I0 -o json) == '{"allowPrivilegeEscalation":false,"readOnlyRootFilesystem":true,"runAsUser":1234}'; then
    echo "unexpected securityContext for container nri-dotnet--dotnetapp"
    exit 1
  fi
  if ! test $(kubectl get pods -l app=dotnetapp -n e2e-namespace -o yaml | yq '.items[0].spec | .initContainers + .containers | .[]| select(.name == "newrelic-apm-health-sidecar") | .securityContext' -I0 -o json) == '{"allowPrivilegeEscalation":false,"readOnlyRootFilesystem":true,"runAsUser":5678}'; then
    echo "unexpected securityContext for container newrelic-apm-health-sidecar"
    exit 1
  fi
  if ! test $(kubectl get pods -l app=dotnetapp -n e2e-namespace -o yaml | yq '.items[0].spec | .initContainers + .containers | .[]| select(.name == "alpine") | .securityContext' -I0 -o json) == 'null'; then
    echo "unexpected securityContext for container alpine"
    exit 1
  fi
  if ! test $(kubectl get pods -l app=dotnetapp -n e2e-namespace -o yaml | yq '.items[0].spec | .initContainers + .containers | .[]| select(.name == "dotnetapp") | .securityContext' -I0 -o json) == 'null'; then
    echo "unexpected securityContext for container dotnetapp"
    exit 1
  fi

  if ! test $(kubectl get pods -l app=dotnetapp -n e2e-namespace -o yaml | yq '.items[0].spec | .initContainers + .containers | .[]| select(.name == "nri-dotnet--dotnetapp") | .imagePullPolicy' -I0 -r) == 'Never'; then
    echo "unexpected imagePullPolicy for container nri-dotnet--dotnetapp"
    exit 1
  fi
  if ! test $(kubectl get pods -l app=dotnetapp -n e2e-namespace -o yaml | yq '.items[0].spec | .initContainers + .containers | .[]| select(.name == "newrelic-apm-health-sidecar") | .imagePullPolicy' -I0 -r) == 'Always'; then
    echo "unexpected imagePullPolicy for container newrelic-apm-health-sidecar"
    exit 1
  fi
  if ! test $(kubectl get pods -l app=dotnetapp -n e2e-namespace -o yaml | yq '.items[0].spec | .initContainers + .containers | .[]| select(.name == "alpine") | .imagePullPolicy' -I0 -r) == 'Always'; then
    echo "unexpected imagePullPolicy for container alpine"
    exit 1
  fi
  if ! test $(kubectl get pods -l app=dotnetapp -n e2e-namespace -o yaml | yq '.items[0].spec | .initContainers + .containers | .[]| select(.name == "dotnetapp") | .imagePullPolicy' -I0 -r) == 'IfNotPresent'; then
    echo "unexpected imagePullPolicy for container dotnetapp"
    exit 1
  fi
}

# ensure that the app container gets a volume mounted with the configmap
function test_java() {
  if ! test $(kubectl get pods -l app=javaapp -n e2e-namespace -o yaml | yq '.items[0].spec | .initContainers + .containers | .[] | select(.name == "init-zero") | [(.volumeMounts[] | select(.name | contains("nri-")))]' -I0 -o json) == '[]'; then
    echo "unexpected volumeMounts for container init-zero"
    exit 1
  fi
  if ! test $(kubectl get pods -l app=javaapp -n e2e-namespace -o yaml | yq '.items[0].spec | .initContainers + .containers | .[] | select(.name == "nri-java--init-java") | [(.volumeMounts[] | select(.name | contains("nri-")))]' -I0 -o json) == '[{"mountPath":"/nri-java--init-java","name":"nri-java--init-java"}]'; then
    echo "unexpected volumeMounts for container nri-java--init-java"
    exit 1
  fi
  if ! test $(kubectl get pods -l app=javaapp -n e2e-namespace -o yaml | yq '.items[0].spec | .initContainers + .containers | .[] | select(.name == "init-java") | [(.volumeMounts[] | select(.name | contains("nri-")))]' -I0 -o json) == '[{"mountPath":"/nri-cfg--init-java","name":"nri-cfg--init-java"},{"mountPath":"/nri-java--init-java","name":"nri-java--init-java"}]'; then
    echo "unexpected volumeMounts for container init-java"
    exit 1
  fi
  if ! test $(kubectl get pods -l app=javaapp -n e2e-namespace -o yaml | yq '.items[0].spec | .initContainers + .containers | .[] | select(.name == "nri-java--javaapp") | [(.volumeMounts[] | select(.name | contains("nri-")))]' -I0 -o json) == '[{"mountPath":"/nri-java--javaapp","name":"nri-java--javaapp"}]'; then
    echo "unexpected volumeMounts for container nri-java--javaapp"
    exit 1
  fi
  if ! test $(kubectl get pods -l app=javaapp -n e2e-namespace -o yaml | yq '.items[0].spec | .initContainers + .containers | .[] | select(.name == "zero") | [(.volumeMounts[] | select(.name | contains("nri-")))]' -I0 -o json) == '[]'; then
    echo "unexpected volumeMounts for container zero"
    exit 1
  fi
  if ! test $(kubectl get pods -l app=javaapp -n e2e-namespace -o yaml | yq '.items[0].spec | .initContainers + .containers | .[] | select(.name == "javaapp") | [(.volumeMounts[] | select(.name | contains("nri-")))]' -I0 -o json) == '[{"mountPath":"/nri-cfg--javaapp","name":"nri-cfg--javaapp"},{"mountPath":"/nri-java--javaapp","name":"nri-java--javaapp"}]'; then
    echo "unexpected volumeMounts for container javaapp"
    exit 1
  fi

  if ! test $(kubectl get pods -l app=javaapp -n e2e-namespace -o yaml | yq '.items[0].spec | [(.volumes[] | select(.name|contains("nri-")))]' -I0 -o json) == '[{"configMap":{"defaultMode":420,"name":"java-cm"},"name":"nri-cfg--init-java"},{"emptyDir":{},"name":"nri-java--init-java"},{"configMap":{"defaultMode":420,"name":"java-cm"},"name":"nri-cfg--javaapp"},{"emptyDir":{},"name":"nri-java--javaapp"}]'; then
    echo "unexpected volumeMounts for pod javaapp"
    exit 1
  fi
}

# ensure that the targeted container was instrumented
function test_nodejs() {
  if ! test $(kubectl get pods -l app=nodejsapp -n e2e-namespace -o yaml | yq '[(.items[0].spec | .initContainers + .containers | .[] | select(.env[].name == "NEW_RELIC_APP_NAME") | .name)]' -I0 -o json) == '["nodejsapp2"]'; then
    echo "unexpected instrumented containers for pod nodejsapp"
    exit 1
  fi
}

# ensure that the 2 containers get the php agent injected
function test_php() {
  if ! test $(kubectl get pods -l app=phpapp -n e2e-namespace -o yaml | yq '[(.items[0].spec | .initContainers + .containers | .[] | select(.env[].name == "PHP_INI_SCAN_DIR") | .name)]' -I0 -o json) == '["phpapp","phpapp2"]'; then
    echo "unexpected instrumented containers for pod phpapp"
    exit 1
  fi
}

# ensure that 4 of the 6 containers get the python agent injected.  the agent init container should be before all the instrumented containers
function test_python() {
  if ! test $(kubectl get pods -l app=pythonapp -n e2e-namespace -o yaml | yq '[(.items[0].spec | .initContainers + .containers | .[] | select(.env[].name == "NEW_RELIC_APP_NAME") | .name)]' -I0 -o json) == '["init-python","any-python1","any-python2","python"]'; then
    echo "unexpected instrumented containers for pod pythonapp"
    exit 1
  fi
}

# ensure that both the ruby and python agent gets configured
function test_ruby() {
  if ! test $(kubectl get pods -l app=rubyapp -n e2e-namespace -o yaml | yq '[(.items[0].spec | .initContainers + .containers | .[] | select(.env[].name == "NEW_RELIC_APP_NAME") | select(.env[].name == "RUBYOPT") | select(.env[].name == "PYTHONPATH") | .name)]' -I0 -o json) == '["rubyapp"]'; then
    echo "unexpected instrumented containers for pod rubyapp"
    exit 1
  fi
}

function teardown() {
    echo "🔄 Teardown"
    minikube delete --all > /dev/null
}

main "$@"
