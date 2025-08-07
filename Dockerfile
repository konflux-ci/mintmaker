# Build the manager binary
FROM registry.access.redhat.com/ubi9/go-toolset:latest@sha256:6ef4378dccbf2319fa67a856151e0f79540b05bd577ec4d394edc953aa246556 AS builder

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

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest@sha256:e6b39b0a2cd88c0d904552eee0dca461bc74fe86fda3648ca4f8150913c79d0f
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
