.PHONY: build docker test docker-test install-deps dest_dir

GOPATH=$(shell go env GOPATH)
GOTESTSUM=${GOPATH}/bin/gotestsum
GOBUILD=go build
ARCH=CGO_ENABLED=0 GOOS=linux GOARCH=amd64
DOCKER_BUILD=docker build

install-deps:
	go get gotest.tools/gotestsum

dest_dir:
	mkdir -p dist

build: dest_dir
	 ${ARCH} ${GOBUILD} -o ./dist ./src/nsmgr

test: spire-server
	${GOTESTSUM} --format short-verbose ./...

docker: build
	${DOCKER_BUILD} --build-arg BUILD=false .

docker-build: dest_dir
	${DOCKER_BUILD} --build-arg BUILD=true . -t networkservicemesh/cmd-nsmgr

spire-proxy: dest_dir
	${ARCH} ${GOBUILD} -o ./dist ./test/spire-proxy

spire-server: dest_dir spire-proxy
	${DOCKER_BUILD} -f test/spire-server/Dockerfile . -t networkservicemesh/test-spire-server

