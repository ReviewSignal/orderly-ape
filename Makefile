## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

GOOS = $(shell go env GOOS)
GOARCH = $(shell go env GOARCH)

GIT_SEMVER = $(LOCALBIN)/git-semver
GIT_SEMVER_VERSION ?= 6.9.0
.PHONY: git-semver
git-semver: $(GIT_SEMVER) ## Download git-semver locally if necessary.
$(GIT_SEMVER): $(LOCALBIN)
	@echo "Downloading git-semver"
	@curl -sL https://github.com/mdomke/git-semver/releases/download/v$(GIT_SEMVER_VERSION)/git-semver_$(GIT_SEMVER_VERSION)_$(GOOS)_$(GOARCH).tar.gz | tar xz -C $(LOCALBIN) git-semver
	@touch $(GIT_SEMVER)

$(LOCALBIN)/.git-semver.mk: $(GIT_SEMVER)
	@echo 'include version.mk' > $@
	@touch "$@"

include $(LOCALBIN)/.git-semver.mk

# ====================================================================================
# Docker image defaults
#
IMAGE_TAG := $(subst +,-,$(VERSION:v%=%))
IMAGE_RELEASE_TRACK_TAG := $(subst +,-,$(RELEASE_TRACK:v%=%))

REPOSITORY ?= github.com/ReviewSignal/orderly-ape
OCI_REGISTRY ?= ghcr.io
OCI_REPOSITORY ?= $(OCI_REGISTRY)/reviewsignal

# Image URL to use all building/pushing image targets
IMG ?= $(OCI_REPOSITORY)/orderly-ape:$(IMAGE_TAG)

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: kubebuilder controller-gen yq ## Generate WebhookConfiguration, ClusterRole, CustomResourceDefinition
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./api/..." paths="./internal/..." output:crd:artifacts:config=config/crd/bases
	@#$(KUBEBUILDER) edit --plugins=helm/v1-alpha

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	go generate ./api/...
	$(CONTROLLER_GEN) object paths="./api/..."
	go tool templ generate
	npx tailwindcss -i ./src/style.css -o ./public/style.css --cwd $(CURDIR)

.PHONY: chart
chart: yq ## Update Helm chart version and appVersion based on git-semver
	@for role in config/rbac/role.yaml config/rbac/*_role.yaml; do echo "---" ; cat $$role; done > charts/orderly-ape/templates/clusterroles.yaml.tmp
	@yq eval '.metadata.name = "orderly-ape-" + .metadata.name' -i charts/orderly-ape/templates/clusterroles.yaml.tmp
	@yq eval '.metadata.labels."helm.sh/chart" = "{{ include \"orderly-ape.chart\" . }}"' -i charts/orderly-ape/templates/clusterroles.yaml.tmp
	@yq eval '.metadata.labels."app.kubernetes.io/version" = "{{ .Chart.AppVersion  }}"' -i charts/orderly-ape/templates/clusterroles.yaml.tmp
	@yq eval '.metadata.labels."app.kubernetes.io/managed-by" = "{{ .Release.Service }}"' -i charts/orderly-ape/templates/clusterroles.yaml.tmp
	@echo "{{- if .Values.rbac.create }}" > charts/orderly-ape/templates/clusterroles.yaml
	@cat charts/orderly-ape/templates/clusterroles.yaml.tmp >> charts/orderly-ape/templates/clusterroles.yaml
	@echo "{{- end }}" >> charts/orderly-ape/templates/clusterroles.yaml
	@rm charts/orderly-ape/templates/clusterroles.yaml.tmp


.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet setup-envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

# TODO(user): To use a different vendor for e2e tests, modify the setup under 'tests/e2e'.
# The default setup assumes Kind is pre-installed and builds/loads the Manager Docker image locally.
# CertManager is installed by default; skip with:
# - CERT_MANAGER_INSTALL_SKIP=true
KIND_CLUSTER ?= project-v4-test-e2e

.PHONY: setup-test-e2e
setup-test-e2e: ## Set up a Kind cluster for e2e tests if it does not exist
	@command -v $(KIND) >/dev/null 2>&1 || { \
		echo "Kind is not installed. Please install Kind manually."; \
		exit 1; \
	}
	@case "$$($(KIND) get clusters)" in \
		*"$(KIND_CLUSTER)"*) \
			echo "Kind cluster '$(KIND_CLUSTER)' already exists. Skipping creation." ;; \
		*) \
			echo "Creating Kind cluster '$(KIND_CLUSTER)'..."; \
			$(KIND) create cluster --name $(KIND_CLUSTER) ;; \
	esac

.PHONY: test-e2e
test-e2e: setup-test-e2e manifests generate fmt vet ## Run the e2e tests. Expected an isolated environment using Kind.
	KIND_CLUSTER=$(KIND_CLUSTER) go test ./test/e2e/ -v -ginkgo.v
	$(MAKE) cleanup-test-e2e

.PHONY: cleanup-test-e2e
cleanup-test-e2e: ## Tear down the Kind cluster used for e2e tests
	@$(KIND) delete cluster --name $(KIND_CLUSTER)

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

.PHONY: lint-config
lint-config: golangci-lint ## Verify golangci-lint linter configuration
	$(GOLANGCI_LINT) config verify

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/main.go

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	$(CONTAINER_TOOL) build -t ${IMG} \
		--build-arg TAG=$(IMAGE_TAG) \
		--build-arg REPOSITORY=$(REPOSITORY) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg GIT_TREE_STATE=$(GIT_TREE_STATE) \
		--build-arg GIT_COMMIT=$(COMMIT_HASH) \
		--build-arg VERSION=$(VERSION) \
		--build-arg BRANCH_NAME=$(BRANCH_NAME) \
		.

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	$(CONTAINER_TOOL) buildx create --name project-v4-builder
	$(CONTAINER_TOOL) buildx use project-v4-builder
	$(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross \
		--build-arg TAG=$(IMAGE_TAG) \
		--build-arg REPOSITORY=$(REPOSITORY) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg GIT_TREE_STATE=$(GIT_TREE_STATE) \
		--build-arg GIT_COMMIT=$(COMMIT_HASH) \
		--build-arg VERSION=$(VERSION) \
		--build-arg BRANCH_NAME=$(BRANCH_NAME) \
		.
	$(CONTAINER_TOOL) buildx rm project-v4-builder
	rm Dockerfile.cross

.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	cd config/manager && $(KUSTOMIZE) edit set image controller=ghcr.io/reviewsignal/orderly-ape/orderly-ape:$(IMAGE_RELEASE_TRACK_TAG)
	$(KUSTOMIZE) build config/default > dist/install.yaml

.PHONY: github-env
github-env: ## Generate GitHub environment variables for the deployer chart.
		@echo "VERSION=$(VERSION)"
		@echo "RELEASE_TRACK=$(RELEASE_TRACK)"
		@echo "IMAGE_TAG=$(IMAGE_TAG)"
		@echo "IMAGE_RELEASE_TRACK_TAG=$(IMAGE_RELEASE_TRACK_TAG)"
		@echo "REPOSITORY=$(REPOSITORY)"
		@echo "BUILD_DATE=$(BUILD_DATE)"
		@echo "GIT_TREE_STATE=$(GIT_TREE_STATE)"
		@echo "GIT_COMMIT=$(COMMIT_HASH)"
		@echo "BRANCH_NAME=$(BRANCH_NAME)"

.PHONY: build-all
build-all:
	$(MAKE) docker-build

.PHONY: push-all
push-all:
	$(MAKE) docker-push

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

##@ Dependencies

## Tool Binaries
KUBEBUILDER ?= $(LOCALBIN)/kubebuilder
KUBECTL ?= kubectl
KIND ?= kind
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint
YQ = $(LOCALBIN)/yq

## Tool Versions
KUBEBUILDER_VERSION ?= v4.5.2
KUSTOMIZE_VERSION ?= v5.6.0
# This version allows MinLength and MaxLength for string aliased types in CRDs
CONTROLLER_TOOLS_VERSION ?= v0.19.1-0.20250829124250-5d6015328208
#ENVTEST_VERSION is the version of controller-runtime release branch to fetch the envtest setup script (i.e. release-0.20)
ENVTEST_VERSION ?= $(shell go list -m -f "{{ .Version }}" sigs.k8s.io/controller-runtime | awk -F'[v.]' '{printf "release-%d.%d", $$2, $$3}')
#ENVTEST_K8S_VERSION is the version of Kubernetes to use for setting up ENVTEST binaries (i.e. 1.31)
ENVTEST_K8S_VERSION ?= $(shell go list -m -f "{{ .Version }}" k8s.io/api | awk -F'[v.]' '{printf "1.%d", $$3}')
GOLANGCI_LINT_VERSION ?= v2.1.6
YQ_VERSION ?= v4.45.4

.PHONY: kubebuilder
kubebuilder: $(KUBEBUILDER) ## Download kubebuilder locally if necessary.
$(KUBEBUILDER): $(LOCALBIN)
	@echo "Downloading kubebuilder"
	@curl -sLo $(KUBEBUILDER) https://github.com/kubernetes-sigs/kubebuilder/releases/download/$(KUBEBUILDER_VERSION)/kubebuilder_$(GOOS)_$(GOARCH)
	@chmod +x $(LOCALBIN)/kubebuilder
	@touch $(KUBEBUILDER)

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: setup-envtest
setup-envtest: envtest ## Download the binaries required for ENVTEST in the local bin directory.
	@echo "Setting up envtest binaries for Kubernetes version $(ENVTEST_K8S_VERSION)..."
	@$(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path || { \
		echo "Error: Failed to set up envtest binaries for version $(ENVTEST_K8S_VERSION)."; \
		exit 1; \
	}

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

.PHONY: yq
yq: $(YQ) ## Download golangci-lint locally if necessary.
$(YQ): $(LOCALBIN)
	$(call go-install-tool,$(YQ),github.com/mikefarah/yq/v4,$(YQ_VERSION))

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) || true ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef

.PHONY: templ templ-web templ-internal server tailwind dev
# Run templ generation in watch mode
templ:
	go tool templ generate -watch --proxy="http://localhost:8000" --open-browser=false

templ-proxy:
	TMP=$$(mktemp -d) && ( \
		touch "$${TMP}/dummy.templ" ;\
		go tool templ generate --watch --path "$${TMP}" --proxy="http://localhost:8000" --open-browser=false -v ;\
		rm -rf "$${TMP}" ;\
	)

# Run air for Go hot reload
server:
	GOGC=2000 go tool wgo -file=.go -file=.yaml time go build -race -o ./bin/orderly-ape ./cmd/main.go :: ./bin/orderly-ape -dev=true -zap-log-level=debug -metrics-bind-address 0 -health-probe-bind-address 0

# Watch Tailwind CSS changes
tailwind:
	npx tailwindcss -i ./src/style.css -o ./public/style.css --watch --cwd $(CURDIR)

# Start development server with all watchers
dev:
	make -j3 templ server tailwind
