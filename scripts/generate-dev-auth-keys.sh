#!/usr/bin/env bash

set -euo pipefail

output_dir="${1:-.local/auth}"
private_key="${output_dir}/private.pem"
public_key="${output_dir}/public.pem"

command -v openssl >/dev/null 2>&1 || {
	echo "openssl is required to generate development authentication keys" >&2
	exit 1
}

if [[ -e "$private_key" || -e "$public_key" ]]; then
	echo "refusing to overwrite existing authentication key files in ${output_dir}" >&2
	exit 1
fi

umask 077
mkdir -p "$output_dir"
openssl genpkey -algorithm Ed25519 -out "$private_key" >/dev/null 2>&1
openssl pkey -in "$private_key" -pubout -out "$public_key" >/dev/null 2>&1
chmod 600 "$private_key" "$public_key"
echo "Generated development-only Ed25519 keys in ${output_dir}."
echo "The directory is ignored by Git and must never be committed or used in production."
