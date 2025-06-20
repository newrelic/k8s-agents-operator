#!/usr/bin/env bash

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
      --values ${SCRIPT_PATH}/e2e-values.yml \
      --set licenseKey=${LICENSE_KEY} \
      --set controllerManager.manager.logLevel=3

    echo "🔄 Waiting for operator to settle"
    sleep 15
    kubectl wait --timeout=30s --for=jsonpath='{.status.phase}'=Running -n k8s-agents-operator -l="app.kubernetes.io/instance=k8s-agents-operator" pod
    sleep 15

    echo "🔄 Creating E2E namespace"
    kubectl create namespace e2e-namespace

    #echo "🔄 Installing secret"
    #kubectl create secret generic newrelic-key-secret \
    #  --namespace k8s-agents-operator \
    #  --from-literal=new_relic_license_key=${LICENSE_KEY}

    echo "🔄 Installing instrumentation"
    for i in $(find ${SCRIPT_PATH} -maxdepth 1 -type f -name 'e2e-instrumentation-*.yml'); do
      kubectl apply --namespace k8s-agents-operator --filename $i
    done

    echo "🔄 Installing apps"
    kubectl apply --namespace e2e-namespace --filename ${SCRIPT_PATH}/apps/

    echo "🔄 Waiting for apps to settle"
    for label in $(find ${SCRIPT_PATH}/apps -type f -name '*.yaml' -exec yq '. | select(.kind == "Deployment") | .metadata.name' {} \;); do
      kubectl wait --timeout=120s --for=jsonpath='{.status.phase}'=Running --namespace e2e-namespace -l="app=$label" pod
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
    echo "✅  Success: all apps were instrumented"
}

# ensure that the init container gets resource requirements, that the health sidecar exists, that the health sidecar get resource requirements assigned
# that the imagePullPolicy gets assigned, that the securityContext gets assigned
function test_dotnet() {

}

# ensure that only the 2 out of the 4 containers get instrumented (2 init containers, 2 regular containers), and it should be the second of each type
function test_java() {

}

# ensure that the 2 containers get the php agent injected
function test_php() {

}

# ensure that 4 of the 6 containers get the python agent injected.  the agent init container should be before all the instrumented containers
function test_python() {

}

# ensure that both the ruby and python agent gets configured
function test_ruby() {

}

function teardown() {
    echo "🔄 Teardown"
   # minikube delete --all > /dev/null
}

main "$@"



