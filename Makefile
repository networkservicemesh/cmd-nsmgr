.PHONY: build docker test docker-test install-deps

install-deps:
	go get gotest.tools/gotestsum

dest_dir: dist
	mkdir -p dist

build: dest_dir
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./dist ./src/nsmgr

test:
	gotestsum --format short-verbose ./...

docker: build
	docker build --build-arg BUILD=false .

docker-build: dest_dir
	docker build --build-arg BUILD=true . -t networkservicemesh/cmd-nsmgr

spire-proxy: dest_dir
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./dist ./test/spire-proxy

spire-server: dest_dir spire-proxy
	docker build -f test/spire-server/Dockerfile . -t networkservicemesh/test-spire-server

