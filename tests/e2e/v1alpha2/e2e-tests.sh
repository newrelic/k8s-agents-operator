#!/usr/bin/env bash

# Test cluster
CLUSTER_NAME=""
K8S_VERSION=""

# New Relic account (production) details
LICENSE_KEY=""

# Unset if you only want to setup a test cluster with E2E specifications
# Set to true if you additionally want to run tests
RUN_TESTS=""

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
    cd ../..
    export DOCKER_BUILDKIT=1
    docker build --tag e2e/k8s-agents-operator:e2e  . --quiet > /dev/null
    cd tests/e2e

    echo "🔄 Loading image into cluster"
    minikube image load e2e/k8s-agents-operator:e2e --profile ${CLUSTER_NAME} > /dev/null

    echo "🔄 Adding Helm repositories"
    helm repo add newrelic https://helm-charts.newrelic.com > /dev/null
    helm repo update > /dev/null
    helm dependency update ../../charts/k8s-agents-operator > /dev/null

    echo "🔄 Installing operator"
    helm upgrade --install k8s-agents-operator ../../charts/k8s-agents-operator \
      --namespace k8s-agents-operator \
      --create-namespace \
      --values e2e-values.yml \
      --set licenseKey=${LICENSE_KEY}

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
    for i in $(find . -maxdepth 1 -type f -name 'e2e-instrumentation-*.yml'); do
      kubectl apply --namespace k8s-agents-operator --filename $i
    done

    echo "🔄 Installing apps"
    kubectl apply --namespace e2e-namespace --filename apps/

    echo "🔄 Waiting for apps to settle"
    for label in $(find ./apps -type f -name '*.yaml' -exec yq '. | select(.kind == "Deployment") | .metadata.name' {} \;); do
      kubectl wait --timeout=120s --for=jsonpath='{.status.phase}'=Running --namespace e2e-namespace -l="app=$label" pod
    done
}

function run_tests() {
    echo "🔄 Starting E2E tests"
    initContainers=$(kubectl get pods --namespace e2e-namespace --output yaml | yq '.items[].spec.initContainers[].name' | wc -l)
    local expected=$(ls apps | wc -l)
    if [[ ${initContainers} -lt $expected ]]; then
      echo "❌ Error: not all apps were instrumented. Expected $expected, got ${initContainers}"
      exit 1
    fi
    echo "✅ Success: all apps were instrumented"
}

function teardown() {
    echo "🔄 Teardown"
    minikube delete --all > /dev/null
}

main "$@"
