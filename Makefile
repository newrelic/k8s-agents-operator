# Directories
GO_DIR = ./src
BIN_DIR = ./bin
TMP_DIR = $(shell pwd)/tmp

# Go packages to test
TEST_PACKAGES = ./src/internal/config \
				./src/api/v1alpha1 \
				./src/autodetect \
				./src/instrumentation/ \
				./src/instrumentation/upgrade \
                ./src/internal/version

# Kubebuilder variables
SETUP_ENVTEST = sigs.k8s.io/controller-runtime/tools/setup-envtest
ENVTEST_VERSION = release-0.18
ENVTEST_BIN = $(TMP_DIR)/setup-envtest
ENVTEST_K8S_VERSION = 1.29.0

# Temp location to install dependencies
$(TMP_DIR):
	mkdir $(TMP_DIR)

# Install setup-envtest
$(ENVTEST_BIN): $(TMP_DIR)
	GOBIN="$(realpath $(TMP_DIR))" go install $(SETUP_ENVTEST)@$(ENVTEST_VERSION)

.PHONY: all
all: clean format modules test build

.PHONY: clean
clean:
	rm -rf $(BIN_DIR) $(TMP_DIR)

.PHONY: format
format:
	go fmt ./...
	go vet ./...

.PHONY: modules
modules:
	@# Add any missing modules and remove unused modules in go.mod and go.sum
	go mod tidy
	@# Verify dependencies have not been modified since being downloaded
	go mod verify

.PHONY: test
test: $(ENVTEST_BIN)
	@chmod -R 755 $(TMP_DIR)/k8s
	KUBEBUILDER_ASSETS="$(shell $(TMP_DIR)/setup-envtest use $(ENVTEST_K8S_VERSION) --bin-dir $(TMP_DIR) -p path)" \
		go test -cover -covermode=count -coverprofile=$(TMP_DIR)/cover.out $(TEST_PACKAGES)

.PHONY: build
build:
	CGO_ENABLED=0 go build -o $(BIN_DIR)/operator $(GO_DIR)

.PHONY: coverprofile
coverprofile: 
	go tool cover -html=$(TMP_DIR)/cover.out
	go tool cover -func=$(TMP_DIR)/cover.out

## Tool Versions
KUSTOMIZE_VERSION ?= v5.0.0
CONTROLLER_TOOLS_VERSION ?= v0.14.0
HELMIFY ?= $(LOCALBIN)/helmify
HELMIFY_VERSION ?= v0.3.34

KUSTOMIZE ?= kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
CRD_OPTIONS ?= "crd:generateEmbeddedObjectMeta=true"

LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

.PHONY: helmify
helmify: $(HELMIFY) ## Download helmify locally if necessary.
$(HELMIFY): $(LOCALBIN)
	test -s $(LOCALBIN)/helmify || GOBIN=$(LOCALBIN) go install github.com/arttor/helmify/cmd/helmify@$(HELMIFY_VERSION)


.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	test -s $(LOCALBIN)/controller-gen || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

# Generate manifests e.g. CRD, RBAC etc.
.PHONY: manifests
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=tests/kustomize/crd/bases

generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="boilerplate.txt"  paths="./..."

helm: manifests helmify
	$(KUSTOMIZE) build tests/kustomize/default | $(HELMIFY)
	cp ./chart/templates/instrumentation-crd.yaml ./charts/k8s-agents-operator/templates/instrumentation-crd.yaml
