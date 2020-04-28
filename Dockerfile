# Set --build-args BUILD=false to copy in binaries built on host
# To Build on host start:
# CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o . ./...
# Build al test commands as well
FROM golang:1.14.2-alpine as build
ARG SPIRE_VERSION=0.9.3
ARG DLV_VERSION=1.4.0
WORKDIR /build
RUN apk add file binutils grep && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go get github.com/go-delve/delve/cmd/dlv@v${DLV_VERSION} && \
    wget -O - https://github.com/spiffe/spire/releases/download/v${SPIRE_VERSION}/spire-${SPIRE_VERSION}-linux-x86_64-glibc.tar.gz | tar -C /opt -xz && \
    mv /opt/spire-${SPIRE_VERSION}/bin/spire-server /bin && \
    mv /opt/spire-${SPIRE_VERSION}/bin/spire-agent /bin
COPY go.mod .
COPY go.sum .
ARG BUILD=true

RUN if [ $BUILD == true ]; then \
    go mod download; fi

# Copy all stuff from host system
COPY . .

# Build NSMGR
RUN if [ ${BUILD} == true ]; then \
      CGO_ENABLED=0 go build -o /go/bin/nsm ./src/nsm || (echo "Failed to compile" && exit 1); \
      nsm build ; \
    fi
# Copy nsm
RUN if [ ${BUILD} != true ]; then \
      cp /build/dist/nsm /go/bin/nsm; \
    fi
# Check nsmgr is a valid file format.
RUN readelf -h ./dist/nsmgr | grep ELF64 || (echo Compile with: CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./dist ./src/nsmgr && (echo "Failed to compile" && exit 1) )

# Copy the results of the build into the runtime container
FROM alpine as runtime
COPY --from=build /build/dist/nsmgr /bin
CMD ["/bin/nsmgr", "run"]

# Testing container
# use
# docker run $(docker build -q . --target test)
FROM ubuntu as test
COPY --from=build /go/bin/dlv /bin/
COPY --from=build /go/bin/nsm /bin/
COPY --from=build /bin/spire-* /bin/
COPY --from=build /build/dist/* /bin/
CMD ["sh", "-c", "/bin/nsm test"]
