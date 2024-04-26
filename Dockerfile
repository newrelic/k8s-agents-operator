# Build the manager binary
FROM golang:1.20 as builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/main.go cmd/main.go
COPY api/ api/
COPY internal/ internal/
COPY pkg/ pkg/
COPY versions.txt versions.txt

ARG VERSION_PKG
ARG VERSION
ARG VERSION_DATE
ARG NEWRELIC_INSTRUMENTATION_JAVA_VERSION
ARG NEWRELIC_INSTRUMENTATION_NODEJS_VERSION
ARG NEWRELIC_INSTRUMENTATION_PYTHON_VERSION
ARG NEWRELIC_INSTRUMENTATION_DOTNET_VERSION
ARG NEWRELIC_INSTRUMENTATION_PHP_VERSION
ARG AUTO_INSTRUMENTATION_GO_VERSION
# Build
# the GOARCH has not a default value to allow the binary be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -ldflags="-X ${VERSION_PKG}.version=${VERSION} -X ${VERSION_PKG}.buildDate=${VERSION_DATE} -X ${VERSION_PKG}.autoInstrumentationJava=${NEWRELIC_INSTRUMENTATION_JAVA_VERSION} -X ${VERSION_PKG}.autoInstrumentationNodeJS=${NEWRELIC_INSTRUMENTATION_NODEJS_VERSION} -X ${VERSION_PKG}.autoInstrumentationPython=${NEWRELIC_INSTRUMENTATION_PYTHON_VERSION} -X ${VERSION_PKG}.autoInstrumentationDotNet=${NEWRELIC_INSTRUMENTATION_DOTNET_VERSION} -X ${VERSION_PKG}.autoInstrumentationPhp=${NEWRELIC_INSTRUMENTATION_PHP_VERSION}" -a -o manager cmd/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
