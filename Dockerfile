FROM golang:1.23.1 as go
ENV GO111MODULE=on
ENV CGO_ENABLED=0
ENV GOBIN=/bin
ARG BUILDARCH=amd64
RUN go install github.com/go-delve/delve/cmd/dlv@v1.8.2
RUN go install github.com/grpc-ecosystem/grpc-health-probe@v0.4.25
ADD https://github.com/spiffe/spire/releases/download/v1.8.0/spire-1.8.0-linux-${BUILDARCH}-musl.tar.gz .
RUN tar xzvf spire-1.8.0-linux-${BUILDARCH}-musl.tar.gz -C /bin --strip=2 spire-1.8.0/bin/spire-server spire-1.8.0/bin/spire-agent


FROM go as build
WORKDIR /build
COPY go.mod go.sum ./
COPY ./local ./local
COPY ./internal/imports ./internal/imports
RUN go build ./internal/imports
COPY . .
RUN go build -o /bin/nsmgr .

FROM build as test
CMD go test -test.v ./...

FROM test as debug
CMD dlv -l :40000 --headless=true --api-version=2 test -test.v ./...

FROM alpine as runtime
ARG user=nsm-user
ARG group=nsm-user
ARG uid=10001
ARG gid=10001
RUN apk add libcap shadow
RUN groupadd -g ${gid} ${user} && useradd -g ${gid} -l -M -u ${uid} ${user}
COPY --from=build /bin/nsmgr /bin/nsmgr
RUN /usr/sbin/setcap cap_dac_override=eip /bin/nsmgr
COPY --from=build /bin/dlv /bin/dlv
COPY --from=build /bin/grpc-health-probe /bin/grpc-health-probe
ENTRYPOINT ["/bin/nsmgr"]
