# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.26.1-bookworm@sha256:8e8aa801e8417ef0b5c42b504dd34db3db911bb73dba933bd8bde75ed815fdbb AS builder

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
