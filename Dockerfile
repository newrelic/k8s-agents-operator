# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.22.4@sha256:a66eda637829ce891e9cf61ff1ee0edf544e1f6c5b0e666c7310dce231a66f28 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY ./src/ ./src/
COPY Makefile .

ARG TARGETOS TARGETARCH
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
