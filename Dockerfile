# Build the manager binary
FROM registry.access.redhat.com/ubi9/go-toolset:1.25@sha256:90a36bc2013b3fcb28e2a4b082c9b895d7c2c679e58b95aed9721970f3339d0e AS builder

ARG TARGETOS
ARG TARGETARCH

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

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest@sha256:1bc3c5c15720506a0cf48adfdf8b623dfe704377e007d7bbae8d14876392ca6a
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
