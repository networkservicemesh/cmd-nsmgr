FROM golang:1.13.8-alpine3.11 as build
# Set --build-args BUILD=false to copy in binaries built on host
# To Build on host run:
# CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o . ./...
# CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c ./src/nsmgr
WORKDIR /build
RUN apk add file
COPY go.mod .
COPY go.sum .
ARG BUILD=true
RUN  [ "${BUILD}" != "true" ] ||  go mod download
COPY . .
RUN  [ "${BUILD}" != "true" ] || CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o . ./...
RUN  [ "${BUILD}" != "false" ] || mv ./dist/nsmgr /build
RUN file nsmgr | grep "ELF 64-bit LSB executable, x86-64" || (echo "Compile with: CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o . ./..." && exit 1 )

# Copy the results of the build into the runtime container
FROM alpine as runtime
COPY --from=build /build/nsmgr /bin
