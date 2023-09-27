// Copyright (c) 2023 Cisco and/or its affiliates.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at:
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package spireconfig contains configurations for spire
package spireconfig

const (
	// SpireAgentConfContents - defines spire-agent configuration
	SpireAgentConfContents = `agent {
    data_dir = "%[1]s/data"
    log_level = "WARN"
    server_address = "127.0.0.1"
    server_port = "8081"
    insecure_bootstrap = true
    trust_domain = "%[2]s"
}

plugins {
    NodeAttestor "join_token" {
        plugin_data {
        }
    }
    KeyManager "disk" {
        plugin_data {
            directory = "%[1]s/data"
        }
    }
    WorkloadAttestor "unix" {
        plugin_data {
            discover_workload_path = true
        }
    }
}
`
	// SpireServerConfContents - defines spire-server configuration
	SpireServerConfContents = `server {
    bind_address = "127.0.0.1"
    bind_port = "8081"
    trust_domain = "%[2]s"
    data_dir = "%[1]s/data"
    log_level = "WARN"
    ca_key_type = "rsa-2048"
    default_x509_svid_ttl = "1h"
    default_jwt_svid_ttl = "1h"
    ca_subject = {
        country = ["US"],
        organization = ["SPIFFE"],
        common_name = "",
    }
    federation {
        bundle_endpoint {
            address = "0.0.0.0"
            port = 8443
        }
        federates_with "%[3]s" {
            bundle_endpoint_url = "https://spire-server.spire.%[4]s:8443"
            bundle_endpoint_profile "https_spiffe" {
                endpoint_spiffe_id = "spiffe://%[3]s/spire/server"
            }
        }
    }
}

plugins {
    DataStore "sql" {
        plugin_data {
            database_type = "sqlite3"
            connection_string = "%[1]s/data/datastore.sqlite3"
        }
    }

    NodeAttestor "join_token" {
        plugin_data {
        }
    }

    KeyManager "memory" {
        plugin_data = {}
    }
}
`
)
