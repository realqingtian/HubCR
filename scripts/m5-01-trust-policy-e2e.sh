#!/bin/sh
# M5-01 Trust Policy management acceptance.
#
# Exercises the PRODUCT management path that M5-01 adds (the m4 script covered trust
# evaluation via the seed CLI only):
#   1. log in as the namespace owner;
#   2. create a new trust-policy version through POST /trust-policy with a real public key;
#   3. read it back through GET /trust-policy and assert the version and fingerprint;
#   4. assert that creating a version eagerly enqueued re-verification for existing
#      artifacts under the new policy version;
#   5. assert that a non-member reader cannot manage (POST) the policy and gets 404 (no
#      existence leak), while a member can still read;
#   6. assert that Pull tokens are still issued (re-verification stays informational).
#
# The transparency-log upload guard from the M4 incident stays in place: every cosign
# invocation passes --tlog-upload=false. This script requires Docker, Compose, openssl,
# curl, python3, and the Go toolchain to build the seed helpers.

set -eu

repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repository_root"

test_project=hubcr-m5-01-trust-policy-e2e
postgres_port=${HUBCR_M5_01_POSTGRES_PORT:-55437}
gateway_port=${HUBCR_M5_01_GATEWAY_PORT:-55005}
test_password=m5-01-trust-policy-password
owner_username=m2-e2e-owner
reader_username=m2-e2e-reader
namespace=m2-e2e-team
repository=private-image
gateway_origin="http://localhost:$gateway_port"
host_database_url="postgres://hubcr:hubcr-test-only@127.0.0.1:$postgres_port/hubcr?sslmode=disable"
temporary_directory=$(mktemp -d)
registry_auth_directory=$temporary_directory/registry-auth
docker_configuration=$temporary_directory/docker
cosign_key_directory=$temporary_directory/cosign-keys
owner_cookie=$temporary_directory/owner.cookies
reader_cookie=$temporary_directory/reader.cookies
policy_body=$temporary_directory/policy.json
event_token=$(openssl rand -hex 32)
cosign_password=m5-01-cosign-test-password
cosign_image=ghcr.io/sigstore/cosign/cosign@sha256:de9c65609e6bde17e6b48de485ee788407c9502fa08b8f4459f595b21f56cd00
base_image=alpine:3.20
signed_image="localhost:$gateway_port/$namespace/$repository:signed"

mkdir "$docker_configuration" "$cosign_key_directory"
umask 077
printf '{"auths":{}}\n' >"$docker_configuration/config.json"

export HUBCR_PRODUCTION_ENV_FILE=$repository_root/.env.production.example
export HUBCR_COMPOSE_PROJECT_NAME=$test_project
export HUBCR_COMPOSE_OVERRIDE_FILE=$repository_root/deployments/compose/compose.recovery-test.yaml
export HUBCR_POSTGRES_PORT=$postgres_port
export HUBCR_GATEWAY_PORT=$gateway_port
export HUBCR_REGISTRY_EXTERNAL_URL="http://host.docker.internal:$gateway_port"
export HUBCR_REGISTRY_ALLOW_INSECURE_HTTP=true
export HUBCR_SESSION_COOKIE_SECURE=false
export HUBCR_REGISTRY_AUTH_DIR=$registry_auth_directory
export HUBCR_REGISTRY_EVENT_TOKEN=$event_token
export POSTGRES_DB=hubcr
export POSTGRES_USER=hubcr
export POSTGRES_PASSWORD=hubcr-test-only
export HUBCR_DATABASE_URL=postgres://hubcr:hubcr-test-only@postgres:5432/hubcr?sslmode=disable
export MINIO_ROOT_USER=hubcr
export MINIO_ROOT_PASSWORD=hubcr-test-only
export HUBCR_API_IMAGE=hubcr-api:m5-01-trust-policy-e2e
export HUBCR_WEB_IMAGE=hubcr-web:m5-01-trust-policy-e2e
export HUBCR_REGISTRY_TOKEN_TTL=10m
export HUBCR_WORKER_JOB_TIMEOUT=15m
export HUBCR_WORKER_LEASE_DURATION=20m
export HUBCR_WORKER_MAX_CONCURRENCY=2
export HUBCR_SECURITY_REPAIR_INTERVAL=2s

compose() {
    scripts/production-compose.sh "$@"
}

docker_cli() {
    docker --config "$docker_configuration" "$@"
}

cosign_generate() {
    docker run --rm --user 0 \
        --env "COSIGN_PASSWORD=$cosign_password" \
        --volume "$cosign_key_directory:/keys" \
        "$cosign_image" "$@"
}

cleanup() {
	if [ "${HUBCR_M5_01_KEEP_ON_FAILURE:-false}" = "true" ] && [ "${e2e_succeeded:-false}" != "true" ]; then
		echo "M5-01 environment retained: project=$test_project temporary_directory=$temporary_directory" >&2
		return
	fi
    compose down --volumes --remove-orphans >/dev/null 2>&1 || true
    docker_cli image rm "$signed_image" >/dev/null 2>&1 || true
    docker image rm "$HUBCR_API_IMAGE" "$HUBCR_WEB_IMAGE" >/dev/null 2>&1 || true
    rm -rf -- "$temporary_directory"
}

fail() {
    echo "$1" >&2
    compose logs api worker registry >&2 || true
    exit 1
}

json_value() {
    expression=$1
    python3 -c "import json,sys; value=json.load(sys.stdin); print($expression)"
}

set_docker_credentials() {
    encoded=$(printf '%s:%s' "$owner_username" "$test_password" | base64 | tr -d '\n')
    printf '{"auths":{"localhost:%s":{"auth":"%s"},"host.docker.internal:%s":{"auth":"%s"},"registry:5000":{"auth":"%s"}}}\n' \
        "$gateway_port" "$encoded" "$gateway_port" "$encoded" "$encoded" \
        >"$docker_configuration/config.json"
}

web_login() {
    user=$1
    cookie=$2
    printf '{"username":"%s","password":"%s"}\n' "$user" "$test_password" |
        curl --fail --silent --show-error --output /dev/null \
            --cookie-jar "$cookie" --header 'Content-Type: application/json' \
            --data-binary @- "$gateway_origin/api/v1/auth/login"
}

artifact_digest() {
    tag=$1
    curl --fail --silent --show-error --cookie "$owner_cookie" \
        "$gateway_origin/api/v1/namespaces/$namespace/repositories/$repository/tags/$tag" 2>/dev/null |
        sed -n 's/.*"digest":"\(sha256:[0-9a-f]*\)".*/\1/p'
}

wait_for_digest() {
    tag=$1
    attempts=0
    while [ "$attempts" -lt 60 ]; do
        digest=$(artifact_digest "$tag" || true)
        if [ -n "$digest" ]; then
            printf '%s\n' "$digest"
            return 0
        fi
        attempts=$((attempts + 1))
        sleep 1
    done
    return 1
}

key_fingerprint() {
    key_file=$1
    der_file=$temporary_directory/public-key.der
    openssl pkey -pubin -in "$key_file" -outform DER -out "$der_file"
    key_hash=$(openssl dgst -sha256 -r "$der_file" | cut -d ' ' -f 1)
    rm -f -- "$der_file"
    printf 'sha256:%s\n' "$key_hash"
}

wait_for_verification() {
    digest=$1
    policy_version=$2
    attempts=0
    while [ "$attempts" -lt 180 ]; do
        state=$(compose exec -T postgres psql --username hubcr --dbname hubcr --tuples-only --no-align \
            --command "SELECT job.state FROM signature_workflows AS workflow JOIN jobs AS job ON job.id = workflow.job_id WHERE workflow.digest = '$digest' AND workflow.policy_version = $policy_version LIMIT 1" 2>/dev/null || true)
        if [ "$state" = "SUCCEEDED" ]; then
            return 0
        fi
        if [ "$state" = "DEAD" ]; then
            return 1
        fi
        attempts=$((attempts + 1))
        sleep 1
    done
    return 1
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

go -C backend run ./cmd/registry-keygen --output-dir "$registry_auth_directory"
cosign_generate generate-key-pair --output-key-prefix /keys/release >/dev/null
release_fingerprint=$(key_fingerprint "$cosign_key_directory/release.pub")
release_pem=$(sed -e ':a' -e 'N;$!ba' -e 's/\n/\\n/g' "$cosign_key_directory/release.pub")
compose config --quiet
compose build api web
compose up --detach --wait postgres redis minio minio-init
compose --profile operations run --rm migrate
# Seed the owner and an organization reader so the authorization matrix can be exercised.
HUBCR_DATABASE_URL="$host_database_url" HUBCR_E2E_PASSWORD="$test_password" \
    go -C backend run ./internal/testsupport/m2seed
compose up --detach --wait api worker web registry gateway

set_docker_credentials
docker_cli pull "$base_image" >/dev/null
docker_cli tag "$base_image" "$signed_image"
docker_cli push "$signed_image" >/dev/null

web_login "$owner_username" "$owner_cookie"
web_login "$reader_username" "$reader_cookie"
digest=$(wait_for_digest signed) || fail "signed Artifact was not reconciled"

# Sign the artifact with the release key BEFORE creating any policy. Transparency-log
# upload stays disabled (M4 incident guard). The signature is only discovered later when
# the policy-driven verification workflow runs.
cosign_image_ref="registry:5000/$namespace/$repository@$digest"
docker run --rm --user 0 \
    --network "${test_project}_default" \
    --env "COSIGN_PASSWORD=$cosign_password" \
    --env DOCKER_CONFIG=/docker \
    --volume "$docker_configuration:/docker:ro" \
    --volume "$cosign_key_directory:/keys" \
    "$cosign_image" sign --yes --allow-http-registry --new-bundle-format=false \
    --use-signing-config=false --tlog-upload=false \
    --key /keys/release.key --annotations "hubcr.io/key-fingerprint=$release_fingerprint" \
    "$cosign_image_ref" >/dev/null

# Create the first trust-policy version through the PRODUCT API (not the seed CLI).
printf '{"public_keys":[{"name":"release","fingerprint":"%s","public_key_pem":"%s"}]}\n' \
    "$release_fingerprint" "$release_pem" >"$policy_body"
create_response=$(curl --fail --silent --show-error --cookie "$owner_cookie" \
    --header 'Content-Type: application/json' --data-binary @"$policy_body" \
    --request POST "$gateway_origin/api/v1/namespaces/$namespace/trust-policy")
created_version=$(printf '%s' "$create_response" | json_value "value['version']")
created_id=$(printf '%s' "$create_response" | json_value "value['id']")
if [ "$created_version" != "1" ]; then
    fail "POST trust-policy returned version $created_version, want 1"
fi

# Read it back through GET and assert the version and fingerprint round-trip.
read_response=$(curl --fail --silent --show-error --cookie "$owner_cookie" \
    "$gateway_origin/api/v1/namespaces/$namespace/trust-policy")
read_version=$(printf '%s' "$read_response" | json_value "value['version']")
read_fingerprint=$(printf '%s' "$read_response" | json_value "value['public_keys'][0]['fingerprint']")
if [ "$read_version" != "1" ] || [ "$read_fingerprint" != "$release_fingerprint" ]; then
    fail "GET trust-policy mismatch: version=$read_version fingerprint=$read_fingerprint"
fi

# Eager re-verification: the just-created policy version must drive a verification workflow
# that completes for the signed artifact. This proves the namespace-scoped eager trigger.
wait_for_verification "$digest" "$created_version" \
    || fail "policy-v$created_version re-verification did not complete for $digest"

# Authorization matrix: a member reader can read but a non-owner cannot create, and the
# denial is a 404 (no namespace existence leak).
reader_read_status=$(curl --silent --output /dev/null --write-out '%{http_code}' \
    --cookie "$reader_cookie" "$gateway_origin/api/v1/namespaces/$namespace/trust-policy")
if [ "$reader_read_status" != "200" ]; then
    fail "member reader GET trust-policy status=$reader_read_status, want 200"
fi
reader_create_status=$(curl --silent --output /dev/null --write-out '%{http_code}' \
    --cookie "$reader_cookie" --header 'Content-Type: application/json' \
    --data-binary @"$policy_body" --request POST \
    "$gateway_origin/api/v1/namespaces/$namespace/trust-policy")
if [ "$reader_create_status" != "404" ]; then
    fail "non-owner POST trust-policy status=$reader_create_status, want 404 (no existence leak)"
fi

# Re-verification stays informational: Pull still works after a policy version is created.
# This is the D-007 regression guard — trust state must never block registry access.
pull_target="localhost:$gateway_port/$namespace/$repository:signed"
docker_cli pull "$pull_target" >/dev/null \
    || fail "Pull failed after trust-policy creation; re-verification is not informational"

# Append a second version to confirm append-only versioning increments correctly.
printf '{"keyless_identities":[{"issuer":"https://token.actions.githubusercontent.com","subject":"https://github.com/acme/app/.github/workflows/release.yml@refs/heads/main"}]}\n' \
    >"$policy_body"
create_response=$(curl --fail --silent --show-error --cookie "$owner_cookie" \
    --header 'Content-Type: application/json' --data-binary @"$policy_body" \
    --request POST "$gateway_origin/api/v1/namespaces/$namespace/trust-policy")
second_version=$(printf '%s' "$create_response" | json_value "value['version']")
if [ "$second_version" != "2" ]; then
    fail "second POST trust-policy returned version $second_version, want 2"
fi
wait_for_verification "$digest" "$second_version" \
    || fail "policy-v$second_version re-verification did not complete for $digest"

e2e_succeeded=true
echo "M5-01 trust-policy management acceptance passed"
