SHELL=/bin/bash -o pipefail

PROJECT_NAME=redis-cluster-operator
REPO=ucloud/$(PROJECT_NAME)

# replace with your public registry
ALTREPO=$(DOCKER_REGISTRY)/$(PROJECT_NAME)
E2EALTREPO=$(DOCKER_REGISTRY)/$(PROJECT_NAME)-e2e

VERSION=$(shell git describe --always --tags --dirty | sed "s/\(.*\)-g`git rev-parse --short HEAD`/\1/")
GIT_SHA=$(shell git rev-parse --short HEAD)
BIN_DIR=build/bin
.PHONY: all build check clean test login build-e2e push push-e2e build-tools

all: check build

build: test build-go build-image

build-go:
	GO111MODULE=on CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
	-ldflags "-X github.com/$(REPO)/version.Version=$(VERSION) -X github.com/$(REPO)/version.GitSHA=$(GIT_SHA)" \
	-o $(BIN_DIR)/$(PROJECT_NAME)-linux-amd64 cmd/manager/main.go
	GO111MODULE=on CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build \
	-ldflags "-X github.com/$(REPO)/version.Version=$(VERSION) -X github.com/$(REPO)/version.GitSHA=$(GIT_SHA)" \
	-o $(BIN_DIR)/$(PROJECT_NAME)-darwin-amd64 cmd/manager/main.go

build-image:
	docker build --build-arg VERSION=$(VERSION) --build-arg GIT_SHA=$(GIT_SHA) -t $(ALTREPO):$(VERSION) .
	docker tag $(ALTREPO):$(VERSION) $(ALTREPO):latest

build-e2e:
	docker build -t $(E2EALTREPO):$(VERSION)  -f test/e2e/Dockerfile .

build-tools:
	bash hack/docker/redis-tools/make.sh build

test:
	GO111MODULE=on go test $$(go list ./... | grep -v /vendor/) -race -coverprofile=coverage.txt -covermode=atomic

login:
	@docker login -u "$(DOCKER_USER)" -p "$(DOCKER_PASS)"

push: build-image
	docker push $(ALTREPO):$(VERSION)
	docker push $(ALTREPO):latest

push-e2e: build-e2e
	docker push $(E2EALTREPO):$(VERSION)

clean:
	rm -f $(BIN_DIR)/$(PROJECT_NAME)*

check: check-format

check-format:
	@test -z "$$(gofmt -s -l . 2>&1 | grep -v -e vendor/ | tee /dev/stderr)"
