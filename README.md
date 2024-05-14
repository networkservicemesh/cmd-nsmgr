# Intro

This repo contains 'nsmgr' that implements the Network Service Manager.

This README will provide directions for building, testing, and debugging that container.

# Build

## Build nsmgr binary locally

You can build the locally by executing

```bash
go build ./...
```

## Build Docker container

You can build the docker container by running:

```bash
docker build .
```

# Usage

## Environment config


* `NSM_NAME`                           - Name of Network service manager (default: "nmgr")
* `NSM_LISTEN_ON`                      - url to listen on. tcp:// one will be used a public to register NSM. (default: "unix:///var/lib/networkservicemesh/nsm.io.sock")
* `NSM_REGISTRY_URL`                   - A NSE registry url to use (default: "tcp://localhost:5001")
* `NSM_MAX_TOKEN_LIFETIME`             - maximum lifetime of tokens (default: "10m")
* `NSM_REGISTRY_SERVER_POLICIES`       - paths to files and directories that contain registry server policies (default: "etc/nsm/opa/common/.*.rego,etc/nsm/opa/registry/.*.rego,etc/nsm/opa/server/.*.rego")
* `NSM_REGISTRY_CLIENT_POLICIES`       - paths to files and directories that contain registry client policies (default: "etc/nsm/opa/common/.*.rego,etc/nsm/opa/registry/.*.rego,etc/nsm/opa/client/.*.rego")
* `NSM_LOG_LEVEL`                      - Log level (default: "INFO")
* `NSM_DIAL_TIMEOUT`                   - Timeout for the dial the next endpoint (default: "750ms")
* `NSM_FORWARDER_NETWORK_SERVICE_NAME` - the default service name for forwarder discovering (default: "forwarder")
* `NSM_OPEN_TELEMETRY_ENDPOINT`        - OpenTelemetry Collector Endpoint (default: "otel-collector.observability.svc.cluster.local:4317")
* `NSM_METRICS_EXPORT_INTERVAL`        - interval between mertics exports (default: "10s")

# Testing

## Testing Docker container

Testing is run via a Docker container.  To run testing run:

```bash
docker run --rm $(docker build -q --target test .)
```

# Debugging

## Debugging the tests
If you wish to debug the test code itself, that can be achieved by running:

```bash
docker run --rm -p 40000:40000 $(docker build -q --target debug .)
```

This will result in the tests running under dlv.  Connecting your debugger to localhost:40000 will allow you to debug.

```bash
-p 40000:40000
```
forwards port 40000 in the container to localhost:40000 where you can attach with your debugger.

```bash
--target debug
```

Runs the debug target, which is just like the test target, but starts tests with dlv listening on port 40000 inside the container.

## Debugging the nsmgr

When you run 'nsmgr' you will see an early line of output that tells you:

```Setting env variable DLV_LISTEN_FORWARDER to a valid dlv '--listen' value will cause the dlv debugger to execute this binary and listen as directed.```

If you follow those instructions when running the Docker container:
```bash
docker run -e DLV_LISTEN_NSMGR=:50000 -p 50000:50000 --rm $(docker build -q --target test .)
```

```-e DLV_LISTEN_NSMGR=:50000``` tells docker to set the environment variable DLV_LISTEN_NSMGR to :50000 telling
dlv to listen on port 50000.

```-p 50000:50000``` tells docker to forward port 50000 in the container to port 50000 in the host.  From there, you can
just connect dlv using your favorite IDE and debug nsmgr.

## Debugging the tests and the nsmgr

```bash
docker run --rm -p 40000:40000 $(docker build -q --target debug .)
```

Please note, the tests **start** the nsmgr, so until you connect to port 40000 with your debugger and walk the tests
through to the point of running nsmgr, you will not be able to attach a debugger on port 50000 to the nsmgr.

# Build Docker image compatible with integration testing suite: 

`docker build . -t networkservicemeshci/cmd-nsmgr:master && kind load docker-image networkservicemeshci/cmd-nsmgr:master` 
