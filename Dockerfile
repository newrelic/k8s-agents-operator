# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.23.3-bookworm@sha256:3f3b9daa3de608f3e869cd2ff8baf21555cf0fca9fd34251b8f340f9b7c30ec5 AS builder

WORKDIR /app

COPY go.mod go.sum ./
COPY ./interop/ ./interop/
RUN go mod download

COPY ./src/ ./src/
COPY Makefile .

ARG TARGETOS
ARG TARGETARCH
ARG GOOS=$TARGETOS
ARG GOARCH=$TARGETARCH
ARG K8S_AGENTS_OPERATOR_VERSION="development"

RUN make build K8S_AGENTS_OPERATOR_VERSION="${K8S_AGENTS_OPERATOR_VERSION}"

# Use minimal base image to package the operator
# Source: https://github.com/GoogleContainerTools/distroless
FROM gcr.io/distroless/static:nonroot
WORKDIR /bin
COPY --from=builder /app/bin/operator .
USER 65532:65532

ENTRYPOINT ["/bin/operator"]
