#!/usr/bin/env bash
set -euo pipefail

# Source common functions
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/../common-functions.sh"

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


OPERATOR_NAMESPACE=k8s-agents-operator
APP_NAMESPACE=e2e-namespace


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
            --k8s_version)
            shift
            K8S_VERSION="$1"
            ;;
            --license_key)
            shift
            LICENSE_KEY="$1"
            ;;
            --run_tests)
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

    echo "🔄 Waiting for nodes"
    kubectl wait --for=condition=Ready --all nodes

    echo "🔄 Waiting for default service account"
    until_ready 15 "kubectl get sa/default"

    echo "🔄 Building Docker image"
    DOCKER_BUILDKIT=1 docker build --tag e2e/k8s-agents-operator:e2e ${REPO_ROOT} --quiet > /dev/null

    echo "🔄 Loading image into cluster"
    minikube image load e2e/k8s-agents-operator:e2e --profile ${CLUSTER_NAME} > /dev/null

    echo "🔄 Adding Helm repositories"
    helm repo add newrelic https://newrelic.github.io/helm-charts/ > /dev/null
    helm repo update > /dev/null
    helm dependency update ${REPO_ROOT}/charts/k8s-agents-operator > /dev/null

    echo "🔄 Installing operator"
    helm upgrade --install k8s-agents-operator ${REPO_ROOT}/charts/k8s-agents-operator \
      --namespace ${OPERATOR_NAMESPACE} \
      --create-namespace \
      --set controllerManager.manager.image.version=e2e,controllerManager.manager.image.pullPolicy=Never,controllerManager.manager.image.repository=e2e/k8s-agents-operator \
      --set licenseKey=${LICENSE_KEY}

    # Use common wait function
    wait_for_operator_ready ${OPERATOR_NAMESPACE}

    echo "🔄 Creating E2E namespace"
    if ! kubectl get ns ${APP_NAMESPACE} > /dev/null 2>&1; then
      kubectl create namespace ${APP_NAMESPACE}
    fi

    echo "🔄 Installing instrumentations"
    for i in $(find ${SCRIPT_PATH} -maxdepth 1 -type f -name 'e2e-instrumentation-*.yml'); do
      echo "  Applying $(basename $i)"
      kubectl apply --namespace ${OPERATOR_NAMESPACE} --filename $i
    done

    # Use common wait function for instrumentations
    wait_for_instrumentations ${OPERATOR_NAMESPACE} "${SCRIPT_PATH}/e2e-instrumentation-*.yml"

    echo "🔄 Installing apps"
    kubectl apply --namespace ${APP_NAMESPACE} --filename ${SCRIPT_PATH}/apps/

    # Use common wait function for apps
    wait_for_apps_ready ${APP_NAMESPACE} "${SCRIPT_PATH}/apps"
}

function run_tests() {
    echo "🔄 Starting E2E tests"

    initContainers=$(kubectl get pods --namespace ${APP_NAMESPACE} --output yaml | yq '.items[].spec.initContainers[].name' | wc -l)
    local expected=$(ls ${SCRIPT_PATH}/apps | wc -l)
    if [[ ${initContainers} -lt $expected ]]; then
      echo "❌ Error: not all apps were instrumented. Expected $expected, got ${initContainers}"
      exit 1
    fi

    if command -v yq > /dev/null 2> /dev/null; then
      test_dotnet
      test_java
    else
      echo "⚠️ test skipped because 'yq' is missing"
    fi


    echo "✅  Success: all apps were instrumented"
}

# ensure that the resources get assigned to the pod init container
function test_dotnet() {
  local dotnet_resources_mem=$(kubectl get pods -l app=dotnetapp -n ${APP_NAMESPACE} -o yaml | yq '.items[] | .spec.initContainers[] | select(.name == "nri-dotnet--dotnetapp") | .resources.requests.memory')
  if test "$dotnet_resources_mem" != "512Mi" ; then
    echo "❌ Error: expected resource with request.memory"
    exit 1
  fi

  # verify that the health sidecar exists
}

# ensure that the configmap gets mounted
function test_java() {
  local java_cfg_vol_name=$(kubectl get pods -l app=javaapp -n ${APP_NAMESPACE} -o yaml | yq '.items[] | .spec.volumes[] | select(.name == "nri-cfg--javaapp") | .name')
  local java_cfg_mount_name=$(kubectl get pods -l app=javaapp -n ${APP_NAMESPACE} -o yaml | yq '.items[] | .spec.containers[].volumeMounts[] | select(.name == "nri-cfg--javaapp") | .name')
  if test "$java_cfg_vol_name" != "nri-cfg--javaapp" || test "$java_cfg_mount_name" != "nri-cfg--javaapp"; then
    echo "❌ Error: expected volume and volume mount with agent config map"
    exit 1
  fi
}

function teardown() {
    echo "🔄 Teardown"
    minikube delete --all > /dev/null
}

function until_ready() {
  local to=$1
  to=$(($to*10))
  shift
  local cmd="$1"
  local c=0
  while ! (eval "$cmd") > /dev/null 2> /dev/null; do
    if test "$c" == "$to"; then
      printf "timeout.\n"
      return 1
    fi
    printf "."
    sleep 0.1
    c=$(($c+1))
  done
  printf "\n"
  return 0
}

main "$@"

