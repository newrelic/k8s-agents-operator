# Directories
BIN_DIR = ./bin
TMP_DIR = $(shell pwd)/tmp

LICENSE_KEY     ?= fake-abc123
E2E_K8S_VERSION ?= v1.31.1
ALL_E2E_K8S_VERSIONS ?= v1.31.1 v1.30.5 v1.29.9 v1.28.14 v1.27.16 v1.26.15

K8S_AGENTS_OPERATOR_VERSION = ""

.DEFAULT_GOAL := help

# Go packages to test
TEST_PACKAGES = ./api/v1beta1 \
	./api/v1alpha2 \
	./internal/apm \
	./internal/autodetect \
	./internal/config \
	./internal/controller \
	./internal/instrumentation \
	./internal/instrumentation/util/ticker \
	./internal/instrumentation/util/worker \
	./internal/migrate/upgrade \
	./internal/version \
	./internal/webhook

## Tool Versions
SETUP_ENVTEST            ?= $(LOCALBIN)/setup-envtest
KUSTOMIZE                ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN           ?= $(LOCALBIN)/controller-gen
HELMIFY                  ?= $(LOCALBIN)/helmify
GOLANGCI_LINT            ?= $(LOCALBIN)/golangci-lint
HELM                     ?= $(LOCALBIN)/helm
HELM_DOCS                ?= $(LOCALBIN)/helm-docs
CT                       ?= $(LOCALBIN)/ct
HELM_UNITTEST            ?= $(LOCALBIN)/helm-unittest
GOIMPORTS                ?= $(LOCALBIN)/goimports

# Kubebuilder variables
SETUP_ENVTEST_K8S_VERSION ?= 1.30.0
ALL_SETUP_ENVTEST_K8S_VERSIONS ?= 1.30.0 1.29.3 1.28.3 1.27.1 1.26.1 #https://storage.googleapis.com/kubebuilder-tools

# controller-gen crd options
CRD_OPTIONS ?= "crd:generateEmbeddedObjectMeta=true"

LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

# Temp location to install dependencies
$(TMP_DIR):
	mkdir $(TMP_DIR)

##@ Targets

.PHONY: help
help:  ## Show help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z0-9_-]+:.*?##/ { printf "  \033[36m%-17s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } END{printf "\n"}' $(MAKEFILE_LIST)

.PHONY: all
all: clean format modules test build ## clean, format, modules, test, build

.PHONY: clean
clean: ## cleanup temp files
	rm -rf $(BIN_DIR) $(TMP_DIR)

.PHONY: modules
modules: ## Download go dependencies
	@# Add any missing modules and remove unused modules in go.mod and go.sum
	go mod tidy
	@# Verify dependencies have not been modified since being downloaded
	go mod verify

##@ Testing

$(TMP_DIR)/cover.out: test

.PHONY: coverprofile
coverprofile: $(TMP_DIR)/cover.out ## Generate coverage report
	go tool cover -html=$(TMP_DIR)/cover.out
	go tool cover -func=$(TMP_DIR)/cover.out

.PHONY: go-test
go-test: $(SETUP_ENVTEST) $(TMP_DIR) ## Run Go tests with k8s version specified by $SETUP_ENVTEST_K8S_VERSION
	@chmod -R 755 $(LOCALBIN)/k8s
	KUBEBUILDER_ASSETS="$(shell $(SETUP_ENVTEST) use $(SETUP_ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" \
		go test -v -cover -covermode=count -coverprofile=$(TMP_DIR)/cover.out -coverpkg=./cmd/...,./api/...,./internal/... $(TEST_PACKAGES)

.PHONY: go-test-race
go-test-race: $(SETUP_ENVTEST) $(TMP_DIR) ## Run Go tests with k8s version specified by $SETUP_ENVTEST_K8S_VERSION with race detector
	@chmod -R 755 $(LOCALBIN)/k8s
	KUBEBUILDER_ASSETS="$(shell $(SETUP_ENVTEST) use $(SETUP_ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" \
		go test -v -race -cover -covermode=atomic -coverprofile=$(TMP_DIR)/cover.out -coverpkg=./cmd/...,./api/...,./internal/... $(TEST_PACKAGES)

.PHONY: all-go-tests
all-go-tests: ## Run go tests with all k8s versions specified by $ALL_SETUP_ENVTEST_K8S_VERSIONS
	@for k8s_version in $(ALL_SETUP_ENVTEST_K8S_VERSIONS); do \
	  env SETUP_ENVTEST_K8S_VERSION=$$k8s_version $(MAKE) -f $(MAKEFILE_LIST) go-test; \
	done

.PHONY: e2e-tests
e2e-tests: ## Run e2e tests with k8s version specified by $E2E_K8S_VERSION
	@for cmd in docker minikube helm kubectl yq; do \
	  if ! command -v $$cmd > /dev/null; then \
	    echo "$$cmd required" >&2; \
	    exit 1; \
	  fi; \
	done
	cd tests/e2e && ./e2e-tests.sh --k8s_version $(E2E_K8S_VERSION) --license_key $(LICENSE_KEY) --run_tests

.PHONY: all-e2e-tests
all-e2e-tests: ## Run e2e tests with all k8s versions specified by $ALL_E2E_K8S_VERSIONS
	@for k8s_version in $(ALL_E2E_K8S_VERSIONS); do \
	  env E2E_K8S_VERSION=$$k8s_version $(MAKE) -f $(MAKEFILE_LIST) e2e-tests; \
	done

.PHONY: run-helm-unittest
run-helm-unittest: $(CT) ## Run helm unit tests based on changes
	@if ! test -f ./.github/ct-lint.yaml; then echo "missing .github/ct-lint.yaml" >&2; exit 1; fi
	@for chart in $$($(CT) list-changed --config ./.github/ct-lint.yaml); do \
	  if test -d "$$chart/tests/"; then \
	    $(HELM_UNITTEST) $$chart; \
	  else \
	    echo "No unit tests found for $$chart"; \
	  fi; \
	done;

.PHONY: test
test: go-test # run-helm-unittest ## Run go tests (just an alias)

##@ Linting

.PHONY: go-lint
go-lint: golangci-lint ## Lint all go files
	$(GOLANGCI_LINT) run --config=./.github/.golangci.yml

.PHONY: lint
lint: go-lint run-helm-lint ## Lint everything

.PHONY: run-helm-lint
run-helm-lint: ## Lint all the helm charts
	@helm lint charts/**

##@ Formatting

.PHONY: format
format: go-format ## Format all files

.PHONY: go-format
go-format: ## Format all go files
	go fmt ./...
	go vet ./...

.PHONY: fix-goimports
fix-goimports: $(GOIMPORTS)
	@for i in ./api ./internal ./cmd; do \
	  find $$i -type f -name '*.go' -exec $(GOIMPORTS) -d -local github.com/newrelic/ -w {} +; \
	done

##@ Builds

.PHONY: build
build: ## Build the go binary
	CGO_ENABLED=0 go build -ldflags="-X 'github.com/newrelic/k8s-agents-operator/internal/version.version=$(K8S_AGENTS_OPERATOR_VERSION)' -X 'github.com/newrelic/k8s-agents-operator/internal/version.buildDate=$(shell date)'" -o $(BIN_DIR)/operator ./cmd/main.go

.PHONY: docker-build
docker-build: ## Build the docker image
	DOCKER_BUILDKIT=1 docker build -t k8s-agent-operator:latest \
	  --platform=linux/amd64,linux/arm64,linux/arm \
      .

##@ Tools

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen
$(CONTROLLER_GEN): $(LOCALBIN)
	@test -s $(CONTROLLER_GEN) || GOBIN=$(LOCALBIN) go -C ./tools install sigs.k8s.io/controller-tools/cmd/controller-gen

.PHONY: ct
ct: $(CT) ## Download ct (Chart Testing)
$(CT): $(LOCALBIN)
	@test -s $(CT) || GOBIN=$(LOCALBIN) go -C ./tools install github.com/helm/chart-testing/v3/ct

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint
$(GOLANGCI_LINT): $(LOCALBIN)
	@test -s $(GOLANGCI_LINT) || GOBIN=$(LOCALBIN) go -C ./tools install github.com/golangci/golangci-lint/v2/cmd/golangci-lint

.PHONY: helm
helm: $(HELM) ## Download helmo
$(HELM): $(LOCALBIN)
	@test -s $(HELM) || GOBIN=$(LOCALBIN) go -C ./tools install helm.sh/helm/v3/cmd/helm

.PHONY: helm-docs
helm-docs: $(HELM_DOCS) ## Download helm-docs
$(HELM_DOCS): $(LOCALBIN)
	@test -s $(HELM_DOCS) || GOBIN=$(LOCALBIN) go -C ./tools install github.com/norwoodj/helm-docs/cmd/helm-docs

.PHONY: helm-unittest
helm-unittest: $(HELM_UNITTEST) ## Download helm-unittest
$(HELM_UNITTEST): $(LOCALBIN)
	@test -s $(HELM_UNITTEST) || GOBIN=$(LOCALBIN) go -C ./tools install github.com/helm-unittest/helm-unittest/cmd/helm-unittest

.PHONY: helmify
helmify: $(HELMIFY) ## Download helmify
$(HELMIFY): $(LOCALBIN)
	@test -s $(HELMIFY) || GOBIN=$(LOCALBIN) go -C ./tools install github.com/arttor/helmify/cmd/helmify

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize
$(KUSTOMIZE): $(LOCALBIN)
	@test -s $(KUSTOMIZE) || GOBIN=$(LOCALBIN) go -C ./tools install sigs.k8s.io/kustomize/kustomize/v5

.PHONY: setup-envtest
setup-envtest: $(SETUP_ENVTEST) ## Download setup-envtest
$(SETUP_ENVTEST): $(LOCALBIN)
	@test -s $(SETUP_ENVTEST) || GOBIN=$(LOCALBIN) go -C ./tools install sigs.k8s.io/controller-runtime/tools/setup-envtest

.PHONY: goimports
goimports: $(GOIMPORTS) ## Download goimports
$(GOIMPORTS): $(LOCALBIN)
	@test -s $(GOIMPORTS) || GOBIN=$(LOCALBIN) go -C ./tools install golang.org/x/tools/cmd/goimports

.PHONY: tools
tools: tools-download controller-gen ct golangci-lint helm helm-docs helm-unittest helmify kustomize setup-envtest goimports ## Download all tools used

.PHONY: tools-download
tools-download: tools/go.mod tools/go.sum
	@go -C ./tools mod download

##@ Generate manifests e.g. CRD, RBAC etc.

.PHONY: gen-helm-docs
gen-helm-docs: helm-docs ## Generate Helm Docs from templates
	$(HELM_DOCS)

.PHONY: generate
generate: controller-gen ## Generate stuff
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt"  paths='{"./api/...","./internal/...","./cmd/..."}'

.PHONY: manifests
manifests: generate controller-gen ## Generate manifests
	$(CONTROLLER_GEN) paths='{"./api/...","./internal/...","./cmd/..."}' \
	  rbac:roleName=manager-role output:rbac:artifacts:config=config/rbac \
	  webhook output:webhook:artifacts:config=config/webhook \
	  $(CRD_OPTIONS) output:crd:artifacts:config=config/crd/bases \
	  output:stdout

.PHONY: run-helmify
run-helmify: manifests helmify kustomize ## Generate the CRD with kustomize and helmify from the manifests
	@# could we do more here?
	$(KUSTOMIZE) build config/default | $(HELMIFY) tmp/k8s-agents-operator
	printf "\nIMPORTANT: The generated chart needs to be transformed!\n- deployment.yaml is split into deployment.yaml and service-account.yaml\n- mutating-webhook-configuration.yaml, validating-webhook-configuration.yaml, and instrumentation-crd.yaml are merged into instrumentation-crd.yaml\n- Documents generated are missing several config options (i.e. labels)\n"
