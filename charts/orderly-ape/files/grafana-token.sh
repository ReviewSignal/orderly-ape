#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal

set -euo pipefail
grafana() {
  local args=("$@")
  # endpoint is the last argument
  local endpoint="${args[-1]}"
  # all preceding arguments are curl options
  local curl_opts=("${@:1:$#-1}")
  curl -s -u "${GRAFANA_ADMIN_USER}:${GRAFANA_ADMIN_PASSWORD}" -H "Content-Type: application/json" "${curl_opts[@]}" "${GRAFANA_URL}/${endpoint}"
}

if [ -n "${GRAFANA_TOKEN:-}" ]; then
  echo "Grafana token already exists, skipping creation."
  exit 0
fi

# Wait for Grafana to be ready
echo "Waiting for Grafana to be ready..."
until grafana -fS -w '\n' api/health ; do
  echo "Grafana not ready yet, waiting..."
  sleep 5
done

echo "Grafana is ready, creating service account token..."

# Ensure Grafana service account 'orderly-ape' exists
http_code="$(grafana -o /tmp/sa_create.json -w "%{http_code}" -X POST -d '{"name": "orderly-ape", "role": "Admin"}' api/serviceaccounts)"
if [ "${http_code}" == "200" ] || [ "${http_code}" == "201" ]; then
  SA_ID=$(jq -r '.id // empty' /tmp/sa_create.json)
elif [ "${http_code}" == "400" ] &&  [ "$(jq -r '.messageId // empty' /tmp/sa_create.json)" == "serviceaccounts.ErrAlreadyExists" ]; then
  # Service account may already exist, try to get its ID
  echo "Service account may already exist, retrieving ID..."
  http_code="$(grafana -o /tmp/sa_search.json -w "%{http_code}" "api/serviceaccounts/search?perpage=1000&page=1&query=orderly-ape")"
  if [ "${http_code}" != "200" ]; then
    echo "Failed to retrieve service accounts, HTTP status: ${http_code}"
    cat /tmp/sa_search.json
    exit 1
  fi
  SA_ID=$(jq -r '.serviceAccounts[] | select(.name=="orderly-ape") | .id' /tmp/sa_search.json | head -n1)
else
  echo "Failed to create service account, HTTP status: ${http_code}"
  cat /tmp/sa_create.json
  exit 1
fi
if [ -z "${SA_ID}" ]; then
  echo "Invalid service account ID"
  echo "Response code: ${http_code}"
  echo "Response body:"
  cat /tmp/curl_body.txt
  exit 1
fi
echo "Service account created with ID: ${SA_ID}"

# Create service account token
TOKEN_NAME="orderly-ape-token-$(date +%s)"
http_code="$(grafana -o /tmp/sa_token.json -w "%{http_code}" -X POST -d '{"name": "'"${TOKEN_NAME}"'"}' "api/serviceaccounts/${SA_ID}/tokens")"
if [ "${http_code}" != "200" ] && [ "${http_code}" != "201" ]; then
  echo "Failed to create service account token, HTTP status: ${http_code}"
  cat /tmp/sa_token.json
  exit 1
fi
SA_TOKEN=$(jq -r '.key // empty' /tmp/sa_token.json)
if [ -z "$SA_TOKEN" ]; then
  echo "Failed to create service account token"
  echo "Response code: ${http_code}"
  echo "Response body:"
  cat /tmp/sa_token.json
  exit 1
fi
echo "Service account token created successfully"

# Now create Kubernetes secret to store the token
TOKEN=$(cat "/var/run/secrets/kubernetes.io/serviceaccount/token")
CA_FILE="/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"

SA_TOKEN_B64=$(echo -n "${SA_TOKEN}" | base64)

# Inject token into Secret JSON
echo '{"data": {}}' | jq --arg token "${SA_TOKEN_B64}" '.data["admin-token"]=$token' > /tmp/secret_patch.json

echo "Patching secret '${SECRET_NAME}' in namespace '${SECRET_NAMESPACE}'..."
curl -sS --fail-with-body \
  -X PATCH "${KUBERNETES_API}/api/v1/namespaces/${SECRET_NAMESPACE}/secrets/${SECRET_NAME}" \
  --cacert "${CA_FILE}" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/merge-patch+json" \
  --data-binary @/tmp/secret_patch.json

echo "Secret created successfully."
