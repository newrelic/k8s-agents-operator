# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.23.2-bookworm@sha256:2341ddffd3eddb72e0aebab476222fbc24d4a507c4d490a51892ec861bdb71fc AS builder

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

RUN make build

# Use minimal base image to package the operator
# Source: https://github.com/GoogleContainerTools/distroless
FROM gcr.io/distroless/static:nonroot
WORKDIR /bin
COPY --from=builder /app/bin/operator .
USER 65532:65532

ENTRYPOINT ["/bin/operator"]
