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


operator_ns=k8s-agents-operator
operator_name=k8s-agents-operator
app_ns=e2e-namespace


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
    helm repo add newrelic https://helm-charts.newrelic.com > /dev/null
    helm repo update > /dev/null
    helm dependency update ${REPO_ROOT}/charts/k8s-agents-operator > /dev/null

    echo "🔄 Installing operator"
    helm upgrade --install k8s-agents-operator ${REPO_ROOT}/charts/k8s-agents-operator \
      --namespace $operator_ns \
      --create-namespace \
      --set controllerManager.manager.image.version=e2e,controllerManager.manager.image.pullPolicy=Never,controllerManager.manager.image.repository=e2e/k8s-agents-operator \
      --set licenseKey=${LICENSE_KEY}

    echo "🔄 Waiting for operator namespace"
    until_ready 15 "kubectl get ns/$operator_ns"

    echo "🔄 Waiting for operator deployment"
    until_ready 15 "kubectl get deploy/$operator_name -n $operator_ns"

    echo "🔄 Waiting for operator to settle"
    kubectl wait --timeout=30s --for=jsonpath='{.status.phase}'=Running -n $operator_ns -l="app.kubernetes.io/instance=k8s-agents-operator" pod

    echo "🔄 Waiting for operator to obtain lease"
    until_ready 30 "kubectl logs -n $operator_ns deploy/$operator_name | grep 'successfully acquired lease'"

    echo "🔄 Waiting for operator to handle admission webhooks"
    until_ready 30 "kubectl apply -f ${SCRIPT_PATH}/e2e-instrumentation-test.yml -n $operator_ns"

    echo "🔄 Creating E2E namespace"
    kubectl create namespace $app_ns

    echo "🔄 Installing instrumentation"
    for i in $(find ${SCRIPT_PATH} -maxdepth 1 -type f -name 'e2e-instrumentation-*.yml'); do
      kubectl apply --namespace $operator_ns --filename $i
    done

    echo "🔄 Installing apps"
    kubectl apply --namespace $app_ns --filename ${SCRIPT_PATH}/apps/

    echo "🔄 Waiting for apps to settle"
    for label in $(find ${SCRIPT_PATH}/apps -type f -name '*.yaml' -exec yq '. | select(.kind == "Deployment") | .metadata.name' {} \;); do
      kubectl wait --timeout=600s --for=jsonpath='{.status.phase}'=Running --namespace $app_ns -l="app=$label" pod
    done
}

function run_tests() {
    echo "🔄 Starting E2E tests"

    initContainers=$(kubectl get pods --namespace $app_ns --output yaml | yq '.items[].spec.initContainers[].name' | wc -l)
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
  local dotnet_resources_mem=$(kubectl get pods -l app=dotnetapp -n $app_ns -o yaml | yq '.items[] | .spec.initContainers[] | select(.name == "nri-dotnet--dotnetapp") | .resources.requests.memory')
  if test "$dotnet_resources_mem" != "512Mi" ; then
    echo "❌ Error: expected resource with request.memory"
    exit 1
  fi

  # verify that the health sidecar exists
}

# ensure that the configmap gets mounted
function test_java() {
  local java_cfg_vol_name=$(kubectl get pods -l app=javaapp -n $app_ns -o yaml | yq '.items[] | .spec.volumes[] | select(.name == "nri-cfg--javaapp") | .name')
  local java_cfg_mount_name=$(kubectl get pods -l app=javaapp -n $app_ns -o yaml | yq '.items[] | .spec.containers[].volumeMounts[] | select(.name == "nri-cfg--javaapp") | .name')
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

