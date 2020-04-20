FROM golang:1.13.8-alpine3.11 as build
# Set --build-args BUILD=false to copy in binaries built on host
# To Build on host run:
# CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o . ./...
# CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c ./src/nsmgr

ARG SPIRE_VERSION=0.9.3
ARG DLV_VERSION=1.4.0
WORKDIR /build
RUN apk add file binutils grep jq && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on \
        go get github.com/go-delve/delve/cmd/dlv@v${DLV_VERSION} && \
    wget -O - https://github.com/spiffe/spire/releases/download/v${SPIRE_VERSION}/spire-${SPIRE_VERSION}-linux-x86_64-glibc.tar.gz | tar -C /opt -xz
COPY go.mod .
COPY go.sum .
ARG BUILD=true
RUN if [ $BUILD == true ]; then go mod download; fi
COPY . .
# Build NSMGR
RUN if [ ${BUILD} == true ]; then \
        echo Build cmd-nsmgr; \
        CGO_ENABLED=0 GOOS=linux GOARCH=amd64 time go build -o . ./... || (echo "Failed to compile" && exit 1); \
    fi
ARG TIME=1
# Build NSMGR tests
RUN if [ ${BUILD} == true ]; then \
        echo Build nsmgr tests; \
        export test_packages=$(CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test ./... --list .* -json | jq 'select(.Action == "pass", .Action == "fail")' | jq .Package | xargs); \
        if [ ${#test_packages} == "0" ]; then echo "Find tests failed" && exit 1; fi; \
        for cmd in ${test_packages}; do \
            echo "Compile ${cmd}"; \
            CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c ${cmd} || (echo "Failed to compile" && exit 1); \
        done; \
        CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o . ./test/spire-proxy || (echo "Failed to compile" && exit 1); \
    fi
# Build NSMGR
RUN readelf -h nsmgr | grep ELF64 || (echo Compile with: CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./dist ./src/nsmgr && (echo "Failed to compile" && exit 1) )

# Copy the results of the build into the runtime container
FROM alpine as runtime
COPY --from=build /build/nsmgr /bin
CMD ["/bin/nsmgr", "run"]

# Testing container
# use
# docker run $(docker build -q . --target test)
FROM ubuntu as test
COPY --from=build /go/bin/dlv /bin/dlv
COPY --from=build /opt/spire-*/bin /bin
COPY --from=build /build/*.test /bin/
COPY --from=build /build/spire-proxy /bin/
COPY --from=build /build/test/spire-server /
VOLUME "/var/run/"
CMD ["sh", "-c", "/bin/spire.sh && find /bin/ -name '*?.test' | NSM_FROM_DOCKER=true xargs -n 1 -I{} sh -c '{} -test.v' "]
