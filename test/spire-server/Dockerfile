# Test container with spire-server and all configurations setup
# Will be used to start spire and use in tests.
# CONTAINER-NAME: networkservicemesh/test-spire-server

FROM ubuntu:18.04 as test
RUN apt-get update; apt-get install -y dumb-init ca-certificates curl

# Install Spire
WORKDIR /test
RUN curl -L https://github.com/spiffe/spire/releases/download/v0.9.3/spire-0.9.3-linux-x86_64-glibc.tar.gz | tar -C /opt -xz
ENV PATH ${PATH}:/opt/spire-0.9.3/bin:/test/bin/
COPY test/spire-server .
COPY ./dist/spire-proxy .
EXPOSE 9099/tcp

CMD ./bin/spire.sh && ./spire-proxy
