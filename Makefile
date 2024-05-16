# Directories
GO_DIR = ./src
BIN_DIR = ./bin
TMP_DIR = ./tmp

# Go packages to test
TEST_PACKAGES = ./src/internal/config \
                ./src/internal/version

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
test:
	mkdir $(TMP_DIR) || true
	go test -cover -covermode=count -coverprofile=$(TMP_DIR)/cover.out $(TEST_PACKAGES)

.PHONY: build
build:
	CGO_ENABLED=0 go build -o $(BIN_DIR)/operator $(GO_DIR)

.PHONY: coverprofile
coverprofile: 
	go tool cover -html=$(TMP_DIR)/cover.out
	go tool cover -func=$(TMP_DIR)/cover.out
