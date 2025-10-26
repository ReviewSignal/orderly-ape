# Build the manager binary
FROM golang:1.24 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN --mount=type=cache,target=/go/pkg/mod/ \
    go mod download -x

# Copy the go source
COPY cmd/main.go cmd/main.go
COPY api/ api/
COPY internal/ internal/
COPY web/ web/

# Build
ENV GOCACHE=/root/.cache/go-build
ARG BUILD_DATE
ARG GIT_TREE_STATE
ARG GIT_COMMIT
ARG VERSION
ARG BRANCH_NAME

ENV GO_LDFLAGS="-X github.com/ReviewSignal/orderly-ape/internal/info.buildDate=${BUILD_DATE} -X github.com/ReviewSignal/orderly-ape/internal/info.gitVersion=${VERSION} -X github.com/ReviewSignal/orderly-ape/internal/info.gitCommit=${GIT_COMMIT} -X github.com/ReviewSignal/orderly-ape/internal/info.gitTreeState=${GIT_TREE_STATE}"

RUN --mount=type=cache,target=/go/pkg/mod/ \
    --mount=type=cache,target="/root/.cache/go-build" \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -ldflags="${GO_LDFLAGS}" -trimpath -a -o manager cmd/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
ARG BUILD_DATE
ARG GIT_TREE_STATE
ARG GIT_COMMIT
ARG VERSION
ARG BRANCH_NAME

LABEL \
    org.opencontainers.image.created=${BUILD_DATE} \
    org.opencontainers.image.revision=${GIT_COMMIT} \
    org.opencontainers.image.version=${VERSION} \
    org.opencontainers.image.source="https://github.com/ReviewSignal/orderly-ape"

WORKDIR /
COPY --from=builder /workspace/manager .
COPY public/ public/
USER 65532:65532


ENTRYPOINT ["/manager"]
