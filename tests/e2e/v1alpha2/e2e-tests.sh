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
    echo "✅  Success: all apps were instrumented"
}

function teardown() {
    echo "🔄 Teardown"
    minikube delete --all > /dev/null
}

main "$@"
