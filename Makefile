# Directories
GO_DIR = ./src
BIN_DIR = ./bin
TMP_DIR = $(shell pwd)/tmp

LICENSE_KEY     ?= fake-abc123
E2E_K8S_VERSION ?= v1.31.1
ALL_E2E_K8S_VERSIONS ?= v1.31.1 v1.30.5 v1.29.9 v1.28.14 v1.27.16 v1.26.15

.DEFAULT_GOAL := help

# Go packages to test
TEST_PACKAGES = ./src/internal/config \
                ./src/internal/version \
                ./src/internal/webhookhandler \
				./src/api/v1alpha2 \
				./src/autodetect \
				./src/instrumentation/ \
				./src/instrumentation/upgrade \
                ./src/apm

# Kubebuilder variables
SETUP_ENVTEST             = $(LOCALBIN)/setup-envtest
SETUP_ENVTEST_VERSION     ?= release-0.19
SETUP_ENVTEST_K8S_VERSION ?= 1.29.0
ALL_SETUP_ENVTEST_K8S_VERSIONS ?= 1.30.0 1.29.3 1.28.3 1.27.1 1.26.1 #https://storage.googleapis.com/kubebuilder-tools

## Tool Versions
KUSTOMIZE                ?= $(LOCALBIN)/kustomize
KUSTOMIZE_VERSION        ?= v5.4.3
CONTROLLER_GEN           ?= $(LOCALBIN)/controller-gen
CONTROLLER_TOOLS_VERSION ?= v0.14.0
HELMIFY                  ?= $(LOCALBIN)/helmify
HELMIFY_VERSION          ?= v0.3.34
GOLANGCI_LINT            ?= $(LOCALBIN)/golangci-lint
GOLANGCI_LINT_VERSION    ?= v1.61.0
HELM                     ?= $(LOCALBIN)/helm
HELM_VERSION             ?= v3.16.1
HELM_DOCS                ?= $(LOCALBIN)/helm-docs
HELM_DOCS_VERSION        ?= v1.14.2
HELM_DOCS_VERSION_ST     ?= $(subst v,,$(HELM_DOCS_VERSION))
CT                       ?= $(LOCALBIN)/ct
CT_VERSION               ?= v3.11.0
HELM_UNITTEST            ?= $(LOCALBIN)/helm-unittest
HELM_UNITTEST_VERSION    ?= v0.6.2

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
		go test -v -cover -covermode=count -coverprofile=$(TMP_DIR)/cover.out $(TEST_PACKAGES)

.PHONY: go-test-race
go-test-race: $(SETUP_ENVTEST) $(TMP_DIR) ## Run Go tests with k8s version specified by $SETUP_ENVTEST_K8S_VERSION with race detector
	@chmod -R 755 $(LOCALBIN)/k8s
	KUBEBUILDER_ASSETS="$(shell $(SETUP_ENVTEST) use $(SETUP_ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" \
		go test -v -race -cover -covermode=atomic -coverprofile=$(TMP_DIR)/cover.out $(TEST_PACKAGES)

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

##@ Builds

.PHONY: build
build: ## Build the go binary
	CGO_ENABLED=0 go build -o $(BIN_DIR)/operator $(GO_DIR)

.PHONY: dockerbuild
dockerbuild: ## Build the docker image
	DOCKER_BUILDKIT=1 docker build -t k8s-agent-operator:latest \
	  --platform=linux/amd64,linux/arm64,linux/arm \
      .

##@ Tools

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen
$(CONTROLLER_GEN): $(LOCALBIN)
	test -s $(CONTROLLER_GEN) || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

.PHONY: ct
ct: $(CT) ## Download ct (Chart Testing)
$(CT): $(LOCALBIN)
	test -s $(CT) || GOBIN=$(LOCALBIN) go install github.com/helm/chart-testing/v3/ct@$(CT_VERSION)

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint
$(GOLANGCI_LINT): $(LOCALBIN)
	test -s $(GOLANGCI_LINT) || GOBIN=$(LOCALBIN) go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

.PHONY: helm
helm: $(HELM) ## Download helmo
$(HELM): $(LOCALBIN)
	test -s $(HELM) || GOBIN=$(LOCALBIN) go install helm.sh/helm/v3/cmd/helm@$(HELM_VERSION)

.PHONY: helm-docs
helm-docs: $(HELM_DOCS) ## Download helm-docs
$(HELM_DOCS): $(LOCALBIN)
	test -s $(HELM_DOCS) || GOBIN=$(LOCALBIN) go install -ldflags "-X 'main.version=$(HELM_DOCS_VERSION_ST)'" github.com/norwoodj/helm-docs/cmd/helm-docs@$(HELM_DOCS_VERSION)

.PHONY: helm-unittest
helm-unittest: $(HELM_UNITTEST) ## Download helm-unittest
$(HELM_UNITTEST): $(LOCALBIN)
	test -s $(HELM_UNITTEST) || GOBIN=$(LOCALBIN) go install github.com/helm-unittest/helm-unittest/cmd/helm-unittest@$(HELM_UNITTEST_VERSION)

.PHONY: helmify
helmify: $(HELMIFY) ## Download helmify
$(HELMIFY): $(LOCALBIN)
	test -s $(HELMIFY) || GOBIN=$(LOCALBIN) go install github.com/arttor/helmify/cmd/helmify@$(HELMIFY_VERSION)

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize
$(KUSTOMIZE): $(LOCALBIN)
	test -s $(KUSTOMIZE) || GOBIN=$(LOCALBIN) go install sigs.k8s.io/kustomize/kustomize/v5@$(KUSTOMIZE_VERSION)

.PHONY: setup-envtest
setup-envtest: $(SETUP_ENVTEST) ## Download setup-envtest
$(SETUP_ENVTEST): $(LOCALBIN)
	test -s $(SETUP_ENVTEST) || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@$(SETUP_ENVTEST_VERSION)

##@ Generate manifests e.g. CRD, RBAC etc.

.PHONY: gen-helm-docs
gen-helm-docs: helm-docs ## Generate Helm Docs from templates
	$(HELM_DOCS)

.PHONY: generate
generate: controller-gen ## Generate stuff
	$(CONTROLLER_GEN) object:headerFile="boilerplate.txt"  paths="./..."

.PHONY: manifests
manifests: generate controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) webhook paths="./..." \
	  rbac:roleName=manager-role output:rbac:artifacts:config=config/rbac \
	  output:webhook:artifacts:config=config/webhook \
	  output:crd:artifacts:config=config/crd/bases \
	  output:stdout

.PHONY: run-helmify
run-helmify: manifests helmify kustomize ## Generate the CRD with kustomize and helmify from the manifests
	@# could we do more here?
	$(KUSTOMIZE) build config/default | $(HELMIFY) tmp/k8s-agents-operator
	cp ./tmp/k8s-agents-operator/templates/instrumentation-crd.yaml ./charts/k8s-agents-operator/templates/instrumentation-crd.yaml
	printf "\nIMPORTANT: The generated chart needs to be transformed!\n- deployment.yaml is split into deployment.yaml and service-account.yaml\n- mutating-webhook-configuration.yaml and validating-webhook-configuration.yaml are merged into service-account.yaml\n- Documents generated are missing several config options (i.e. labels)\n"
