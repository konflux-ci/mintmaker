# Build the manager binary
FROM registry.access.redhat.com/ubi9/go-toolset:1.22.9@sha256:6ec9c3ce36c929ff98c1e82a8b7fb6c79df766d1ad8009844b59d97da9afed43 as builder

ARG TARGETOS
ARG TARGETARCH

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/manager/main.go cmd/main.go
COPY cmd/osv-generator/main.go cmd/osv-generator/main.go
COPY api/ api/
COPY tools/ tools/
COPY internal/ internal/
COPY licenses/ licenses/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager cmd/main.go
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o osv-generator cmd/osv-generator/main.go

# Use ubi-micro as minimal base image to package the manager binary
# See https://catalog.redhat.com/software/containers/ubi9/ubi-micro/615bdf943f6014fa45ae1b58
FROM registry.access.redhat.com/ubi9/ubi-minimal:9.5@sha256:14f14e03d68f7fd5f2b18a13478b6b127c341b346c86b6e0b886ed2b7573b8e0
WORKDIR /
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
