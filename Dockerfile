# Build the manager binary
FROM registry.access.redhat.com/ubi9/go-toolset:latest@sha256:3bbd87d77ea93742bd71a5275a31ec4a7693454ab80492c6a7d28ce6eef35378 AS builder

ARG TARGETOS
ARG TARGETARCH
ENV GOTOOLCHAIN=auto

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/manager/main.go cmd/manager/main.go
COPY cmd/osv-generator/main.go cmd/osv-generator/main.go
COPY api/ api/
COPY tools/ tools/
COPY internal/ internal/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager cmd/manager/main.go
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o osv-generator cmd/osv-generator/main.go

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest@sha256:383329bf9c4f968e87e85d30ba3a5cb988a3bbde28b8e4932dcd3a025fd9c98c
WORKDIR /
# OpenShift preflight check requires licensing files under /licenses
COPY licenses/ licenses

# Copy the binary files from builder
COPY --from=builder /opt/app-root/src/manager .
COPY --from=builder /opt/app-root/src/osv-generator .

# It is mandatory to set these labels
LABEL name="Konflux Mintmaker"
LABEL description="Konflux Mintmaker"
LABEL io.k8s.description="Konflux Mintmaker"
LABEL io.k8s.display-name="mintmaker"
LABEL summary="Konflux Mintmaker"
LABEL com.redhat.component="mintmaker"

USER 65532:65532

ENTRYPOINT ["/manager"]
