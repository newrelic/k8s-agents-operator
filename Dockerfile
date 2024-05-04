# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.22.2@sha256:450e3822c7a135e1463cd83e51c8e2eb03b86a02113c89424e6f0f8344bb4168 AS builder

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
