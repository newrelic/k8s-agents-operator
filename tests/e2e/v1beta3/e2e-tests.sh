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
    #minikube delete --all > /dev/null
    now=$( date "+%Y-%m-%d-%H-%M-%S" )
    CLUSTER_NAME=${now}-e2e-tests

    CLUSTER_NAME=2025-11-14-16-27-48-e2e-tests

    echo "🔄 Creating cluster ${CLUSTER_NAME}"
    #minikube start --container-runtime=containerd --kubernetes-version=${K8S_VERSION} --profile ${CLUSTER_NAME} > /dev/null

    echo "🔄 Building Docker image"
    DOCKER_BUILDKIT=1 docker build --tag e2e/k8s-agents-operator:e2e ${REPO_ROOT} --quiet > /dev/null

    echo "🔄 Loading operator image into cluster"
    minikube image load e2e/k8s-agents-operator:e2e --profile ${CLUSTER_NAME} > /dev/null

    echo "🔄 Loading health agent sidecar image into cluster"
    minikube image load --profile ${CLUSTER_NAME} newrelic/k8s-apm-agent-health-sidecar:latest

    echo "🔄 Adding Helm repositories"
    helm repo add newrelic https://helm-charts.newrelic.com > /dev/null
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
    echo "  Applying instrumentation.yml"
    kubectl apply --namespace ${OPERATOR_NAMESPACE} --filename ${SCRIPT_PATH}/instrumentation.yml

    # Wait for instrumentation to be established
    echo "🔄 Waiting for instrumentations to be established"
    sleep 3
    inst_name=$(yq '.metadata.name' ${SCRIPT_PATH}/instrumentation.yml 2>/dev/null || grep 'name:' ${SCRIPT_PATH}/instrumentation.yml | head -1 | awk '{print $2}')
    if [ -n "$inst_name" ]; then
      echo "  Verifying instrumentation: $inst_name"
      until kubectl get instrumentation -n ${OPERATOR_NAMESPACE} $inst_name 2>/dev/null; do
        echo "    Waiting for instrumentation $inst_name to be created..."
        sleep 1
      done
    fi
    echo "✅ Instrumentations are established"

    echo "🔄 Installing apps"
    kubectl apply --namespace ${APP_NAMESPACE} --filename ${SCRIPT_PATH}/deployment.yml

    echo "🔄 Waiting for app pods to be ready"
    # Wait for pod to exist first
    until kubectl get pod -n ${APP_NAMESPACE} -l="pod=php-pod" 2>/dev/null | grep -q "php-pod"; do
      echo "  Waiting for pod with label pod=php-pod to be created..."
      sleep 2
    done

    # Wait for pod to be ready (not just running)
    kubectl wait --timeout=600s --for=condition=Ready \
      --namespace ${APP_NAMESPACE} -l="pod=php-pod" pod

    echo "✅ All apps are ready and instrumented"
}

function teardown() {
    echo "🔄 Teardown"
    #minikube delete --all > /dev/null
}

main "$@"
