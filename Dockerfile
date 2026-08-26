# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.26.6-bookworm@sha256:116d58cbd88c1297624acc6e967a060012422bacf9930927e23fb719189c6f36 AS builder

WORKDIR /app
# Copy the Go Modules manifests
COPY go.mod go.sum ./
COPY ./interop/ ./interop/
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/main.go cmd/main.go
COPY api/ api/
COPY internal/ internal/
COPY Makefile .

ARG TARGETOS
ARG TARGETARCH
ARG GOOS=$TARGETOS
ARG GOARCH=$TARGETARCH
ARG K8S_AGENTS_OPERATOR_VERSION="development"

RUN make build K8S_AGENTS_OPERATOR_VERSION="${K8S_AGENTS_OPERATOR_VERSION}"

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /bin
COPY --from=builder /app/bin/operator .
USER 65532:65532

ENTRYPOINT ["/bin/operator"]
