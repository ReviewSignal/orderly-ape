# ====================================================================================
# git introspection

ifeq ($(COMMIT_HASH),)
override COMMIT_HASH := $(shell git rev-parse HEAD)
endif

TAGS := $(shell git tag -l --points-at HEAD)

GIT_HEAD := $(shell git rev-parse --abbrev-ref HEAD)

# Set default GIT_TREE_STATE
ifeq ($(shell git status -s | head -c1 | wc -c | tr -d '[[:space:]]'), 0)
GIT_TREE_STATE = clean
else
GIT_TREE_STATE = dirty
endif

# ====================================================================================
# Version and Tagging
#

BUILD_DATE ?= $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

# set a semantic version number from git if VERSION is undefined.
ifeq ($(origin VERSION), undefined)
ifeq ($(GIT_TREE_STATE),clean)
VERSION := $(shell $(GIT_SEMVER) -prefix v $(ROOT_DIR))
else
VERSION := $(shell $(GIT_SEMVER) -prefix v -set-meta $(shell echo "$(COMMIT_HASH)" | head -c8)-dirty $(ROOT_DIR))
endif
else
endif

VERSION_REGEX := ^v?([0-9]*)[.]([0-9]*)[.]([0-9]*)(-(alpha|beta|rc|dev)[.][0-9]+)?(\+[[:alnum:].+_-]+)?$$
VERSION_VALID := $(shell echo "$(VERSION)" | grep -E -q '$(VERSION_REGEX)' && echo 1 || echo 0)
VERSION_MAJOR := $(shell echo "$(VERSION)" | sed -E -e 's/$(VERSION_REGEX)/\1/')
VERSION_MINOR := $(shell echo "$(VERSION)" | sed -E -e 's/$(VERSION_REGEX)/\2/')
VERSION_PATCH := $(shell echo "$(VERSION)" | sed -E -e 's/$(VERSION_REGEX)/\3/')

ifneq ($(VERSION_VALID),1)
$(error invalid version $(VERSION). must be a semantic semver version (eg. with v[Major].[Minor].[Patch]))
endif

RELEASE_TRACK := $(VERSION_MAJOR).$(VERSION_MINOR)

ifeq ($(origin BRANCH_NAME), undefined)
ifeq ($(GIT_HEAD),HEAD)
# pretend we are on a release branch if we are just checking out a commit
BRANCH_NAME := release-$(VERSION_MAJOR).$(VERSION_MINOR)
else
BRANCH_NAME := $(GIT_HEAD)
endif
endif

export VERSION
export BRANCH_NAME
export COMMIT_HASH
export BUILD_DATE
export RELEASE_TRACK
