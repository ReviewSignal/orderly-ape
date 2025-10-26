#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal

set -euo pipefail

# Wait for Dex to be ready
wait_for_dex() {
  echo "Waiting for Dex gRPC service to be ready..."
  local max_attempts=30
  local attempt=0
  
  until grpcurl -plaintext "${DEX_SERVICE}:${DEX_GRPC_PORT}" list > /dev/null 2>&1; do
    attempt=$((attempt + 1))
    if [ $attempt -ge $max_attempts ]; then
      echo "ERROR: Dex gRPC service not ready after ${max_attempts} attempts"
      exit 1
    fi
    echo "Dex not ready yet, waiting... (attempt ${attempt}/${max_attempts})"
    sleep 2
  done
  
  echo "Dex gRPC service is ready"
}

# Create password in Dex using gRPC API
create_password() {
  local email="$1"
  local password="$2"
  local username="$3"
  
  echo "Creating admin user in Dex with email: ${email}"
  
  # Generate bcrypt hash for the password
  local password_hash
  password_hash=$(htpasswd -nbBC 10 "" "${password}" | sed 's/^://')
  
  # Base64 encode the hash for gRPC (Dex expects hash as base64-encoded bytes)
  local password_hash_b64
  password_hash_b64=$(echo -n "${password_hash}" | base64 | tr -d '\n')
  
  # Create JSON payload using jq to properly escape values
  local json_payload
  json_payload=$(jq -n \
    --arg email "${email}" \
    --arg username "${username}" \
    --arg hash "${password_hash_b64}" \
    '{password: {email: $email, username: $username, hash: $hash, userId: "admin"}}')
  
  # Create password using grpcurl, ignore errors if user already exists
  if echo "${json_payload}" | grpcurl -plaintext -d @ \
    "${DEX_SERVICE}:${DEX_GRPC_PORT}" \
    api.Dex/CreatePassword 2>&1 | tee /tmp/grpc_output.txt; then
    echo "Admin user created successfully"
  else
    # Check if the error is "already exists"
    if grep -q "AlreadyExists\|already exists" /tmp/grpc_output.txt; then
      echo "Admin user already exists, skipping creation"
    else
      echo "ERROR: Failed to create admin user"
      cat /tmp/grpc_output.txt
      exit 1
    fi
  fi
}

main() {
  # Validate required environment variables
  : "${ADMIN_EMAIL:?ADMIN_EMAIL environment variable is required}"
  : "${ADMIN_PASSWORD:?ADMIN_PASSWORD environment variable is required}"
  : "${DEX_SERVICE:?DEX_SERVICE environment variable is required}"
  : "${DEX_GRPC_PORT:-5557}"
  
  # Extract username from email (part before @)
  ADMIN_USERNAME="${ADMIN_EMAIL%%@*}"
  
  # Wait for Dex to be ready
  wait_for_dex
  
  # Create admin user (will ignore if already exists)
  create_password "${ADMIN_EMAIL}" "${ADMIN_PASSWORD}" "${ADMIN_USERNAME}"
  
  echo "Admin user setup complete"
}

main "$@"
