FROM golang:1.13.3 as go-builder

RUN apt-get update && apt-get -y upgrade && \
    apt-get install -y ca-certificates git mercurial

ARG PROJECT_NAME=redis-cluster-operator
ARG REPO_PATH=github.com/ucloud/$PROJECT_NAME
ARG BUILD_PATH=${REPO_PATH}/cmd/manager

# Build version and commit should be passed in when performing docker build
ARG VERSION=0.1.1
ARG GIT_SHA=0000000

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY pkg ./ cmd ./ version ./

#RUN GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o ${GOBIN}/${PROJECT_NAME} \
#    -ldflags "-X ${REPO_PATH}/version.Version=${VERSION} -X ${REPO_PATH}/version.GitSHA=${GIT_SHA}" \
#    $BUILD_PATH

COPY . ./
RUN apt-get install -y bash git make cmake gcc openssh-client openssh-server
RUN apt-get install -y libc-dev
RUN make build