#!/bin/bash

# ref: https://github.com/kubedb/mysql/blob/master/hack

set -xeou pipefail

GOPATH=$(go env GOPATH)
REPO_ROOT=$GOPATH/src/github.com/ucloud/redis-cluster-operator

source "$REPO_ROOT/hack/lib/lib.sh"
source "$REPO_ROOT/hack/lib/image.sh"

DOCKER_REGISTRY=${DOCKER_REGISTRY:-operator}

IMG=redis-tools

DB_VERSION=5.0.4
TAG="$DB_VERSION"

OSM_VER=${OSM_VER:-0.9.1}

DIST=$REPO_ROOT/dist
mkdir -p $DIST

build() {
  pushd "$REPO_ROOT/hack/docker/redis-tools"

  # Download osm
  wget https://cdn.appscode.com/binaries/osm/${OSM_VER}/osm-alpine-amd64
  chmod +x osm-alpine-amd64
  mv osm-alpine-amd64 osm

  local cmd="docker build --pull -t $DOCKER_REGISTRY/$IMG:$TAG ."
  echo $cmd; $cmd

  rm osm
  popd
}

# shellcheck disable=SC2068
binary_repo $@
