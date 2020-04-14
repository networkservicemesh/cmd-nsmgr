#!/bin/bash

# Run spire-server
spire-server run &
until spire-server healthcheck; do echo "spire-server healthcheck failed, retrying in 100 ms";sleep 0.1;done

# Create identity for /bin/nsmgr
spire-server entry create -parentID spiffe://example.org/myagent -spiffeID spiffe://example.org/nsmgr -selector unix:path:/bin/nsmgr

# Create fallback identity for tests
spire-server entry create -parentID spiffe://example.org/myagent -spiffeID spiffe://example.org/tests -selector unix:uid:"$(id -u)"

# Run spire-agent
(spire-server token generate -spiffeID spiffe://example.org/myagent | sed 's;Token:;;' | xargs spire-agent run -config conf/agent/agent.conf -joinToken) &
until spire-agent healthcheck; do echo "spire-agent healthcheck failed, retrying in 100 ms";sleep 0.1;done
