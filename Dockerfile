FROM golang:1.16-buster as go
ENV GO111MODULE=on
ENV CGO_ENABLED=0
ENV GOBIN=/bin
RUN go get github.com/go-delve/delve/cmd/dlv@v1.5.0
RUN go get github.com/grpc-ecosystem/grpc-health-probe@v0.4.1
RUN go get github.com/edwarnicke/dl
RUN ["/bin/bash", "-c", "set -o pipefail && dl https://github.com/spiffe/spire/releases/download/v0.11.1/spire-0.11.1-linux-x86_64-glibc.tar.gz | tar -xzvf - -C /bin --strip=3 ./spire-0.11.1/bin/spire-server ./spire-0.11.1/bin/spire-agent"]

FROM go as build
WORKDIR /build
COPY go.mod go.sum ./
COPY ./local ./local
COPY ./internal/imports ./internal/imports
RUN go build ./internal/imports
COPY . .
RUN go build -o /bin/nsmgr .

FROM build as test
# CMD go test -test.v ./...
CMD ["go", "test", "-test.v ./..."]

FROM test as debug
# CMD dlv -l :40000 --headless=true --api-version=2 test -test.v ./...
CMD ["dlv", "-l :40000 --headless=true --api-version=2 test -test.v ./..."]

FROM alpine:3.15 as runtime
COPY --from=build /bin/nsmgr /bin/nsmgr
COPY --from=build /bin/dlv /bin/dlv
COPY --from=build /bin/grpc-health-probe /bin/grpc-health-probe
ENTRYPOINT ["/bin/nsmgr"]
