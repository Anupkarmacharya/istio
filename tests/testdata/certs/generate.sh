#!/bin/bash

# Copyright 2018 Istio Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http:#www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Generates certificates used for testing
# We generate a cert for a workload (ns=default, sa=default) and control plane
WD=$(dirname "$0")
WD=$(cd "$WD"; pwd)

set -ex

cat > "${WD}/client.conf" <<EOF
[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name
[req_distinguished_name]
[ v3_req ]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
extendedKeyUsage = clientAuth, serverAuth
subjectAltName = @alt_names
[alt_names]
URI = spiffe://cluster.local/ns/default/sa/default
EOF

cat > "${WD}/server.conf" <<EOF
[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name
[req_distinguished_name]
[ v3_req ]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
extendedKeyUsage = clientAuth, serverAuth
subjectAltName = @alt_names
[alt_names]
URI = spiffe://cluster.local/ns/istio-system/sa/istio-pilot-service-account
DNS.1 = istiod.istio-system
DNS.2 = istiod.istio-system.svc
DNS.3 = istio-pilot.istio-system
DNS.4 = istio-pilot.istio-system.svc
EOF


# Create a certificate authority
openssl genrsa -out "${WD}/pilot/ca-key.pem" 2048
openssl req -x509 -new -nodes -key "${WD}/pilot/ca-key.pem" -days 100000 -out "${WD}/pilot/root-cert.pem" -subj "/CN=cluster.local"
cp "${WD}/pilot/root-cert.pem" "${WD}/default/root-cert.pem"
cp "${WD}/pilot/root-cert.pem" "${WD}/pilot/ca-cert.pem"

# Create a server certificate
openssl genrsa -out "${WD}/pilot/key.pem" 2048
openssl req -new -key "${WD}/pilot/key.pem" -out "${WD}/server.csr" -subj "/CN=cluster.local" -config "${WD}/server.conf"
openssl x509 -req -in "${WD}/server.csr" -CA "${WD}/pilot/root-cert.pem" -CAkey "${WD}/pilot/ca-key.pem" -CAcreateserial -out "${WD}/pilot/cert-chain.pem"  -days 100000 -extensions v3_req -extfile "${WD}/server.conf"

# Create a client certificate
openssl genrsa -out "${WD}/default/key.pem" 2048
openssl req -new -key "${WD}/default/key.pem" -out "${WD}/client.csr" -subj "/CN=cluster.local" -config "${WD}/client.conf"
openssl x509 -req -in "${WD}/client.csr" -CA "${WD}/pilot/root-cert.pem" -CAkey "${WD}/pilot/ca-key.pem" -CAcreateserial -out "${WD}/default/cert-chain.pem" -days 100000 -extensions v3_req -extfile "${WD}/client.conf"

rm "${WD}/server.conf" "${WD}/client.conf"
rm "${WD}/server.csr" "${WD}/client.csr"
rm "${WD}/pilot/root-cert.srl"
