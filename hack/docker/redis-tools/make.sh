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

RCLONE_VER=${RCLONE_VER:-v1.50.2}

DIST=$REPO_ROOT/dist
mkdir -p $DIST

build() {
  pushd "$REPO_ROOT/hack/docker/redis-tools"

  if [ ! -f "rclone" ]; then
    # Download rclone
    wget https://downloads.rclone.org/"${RCLONE_VER}"/rclone-"${RCLONE_VER}"-linux-amd64.zip
    unzip rclone-"${RCLONE_VER}"-linux-amd64.zip
    chmod +x rclone-"${RCLONE_VER}"-linux-amd64/rclone
    mv rclone-"${RCLONE_VER}"-linux-amd64/rclone rclone
  fi

  local cmd="docker build --pull -t $DOCKER_REGISTRY/$IMG:$TAG ."
  echo $cmd; $cmd

  rm -rf rclone-"${RCLONE_VER}"-linux-amd64*
  rm rclone
  popd
}

# shellcheck disable=SC2068
binary_repo $@
