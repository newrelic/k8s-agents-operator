# Directories
GO_DIR = ./src
BIN_DIR = ./bin
TMP_DIR = ./tmp

.PHONY: all
all: clean format modules build

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

.PHONY: build
build:
	CGO_ENABLED=0 go build -o $(BIN_DIR)/operator $(GO_DIR)
