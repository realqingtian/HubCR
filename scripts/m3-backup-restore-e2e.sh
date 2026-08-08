#!/bin/sh

set -eu

repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repository_root"

test_project=hubcr-m3-recovery-e2e
postgres_port=${HUBCR_M3_RECOVERY_POSTGRES_PORT:-55435}
gateway_port=${HUBCR_M3_RECOVERY_GATEWAY_PORT:-55003}
test_password=m3-recovery-password
owner_username=m2-e2e-owner
base_image=alpine:3.22
public_image="localhost:$gateway_port/m2-e2e-team/public-image:smoke"
private_image="localhost:$gateway_port/m2-e2e-team/private-image:smoke"
host_database_url="postgres://hubcr:hubcr-test-only@127.0.0.1:$postgres_port/hubcr?sslmode=disable"
gateway_origin="http://localhost:$gateway_port"
temporary_directory=$(mktemp -d)
registry_auth_directory=$temporary_directory/registry-auth
backup_directory=$temporary_directory/backup
docker_configuration=$temporary_directory/docker
owner_cookie=$temporary_directory/owner.cookies
old_auth_directory=$temporary_directory/old-registry-auth
event_token=$(openssl rand -hex 32)

mkdir "$docker_configuration"
umask 077
printf '{"auths":{}}\n' >"$docker_configuration/config.json"

export HUBCR_PRODUCTION_ENV_FILE=$repository_root/.env.production.example
export HUBCR_COMPOSE_PROJECT_NAME=$test_project
export HUBCR_COMPOSE_OVERRIDE_FILE=$repository_root/deployments/compose/compose.recovery-test.yaml
export HUBCR_POSTGRES_PORT=$postgres_port
export HUBCR_GATEWAY_PORT=$gateway_port
export HUBCR_REGISTRY_EXTERNAL_URL=$gateway_origin
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
export HUBCR_API_IMAGE=hubcr-api:m3-recovery-e2e
export HUBCR_WEB_IMAGE=hubcr-web:m3-recovery-e2e
export HUBCR_SECURITY_SCANNER_ENABLED=false

compose() {
    scripts/production-compose.sh "$@"
}

docker_cli() {
    docker --config "$docker_configuration" "$@"
}

set_docker_credentials() {
    encoded=$(printf '%s:%s' "$owner_username" "$test_password" | base64 | tr -d '\n')
    printf '{"auths":{"localhost:%s":{"auth":"%s"}}}\n' \
        "$gateway_port" "$encoded" >"$docker_configuration/config.json"
}

cleanup() {
    compose down --volumes --remove-orphans >/dev/null 2>&1 || true
    docker_cli image rm "$public_image" "$private_image" >/dev/null 2>&1 || true
    docker image rm "$HUBCR_API_IMAGE" "$HUBCR_WEB_IMAGE" >/dev/null 2>&1 || true
    rm -rf -- "$temporary_directory"
}

fail() {
    echo "$1" >&2
    compose logs >&2 || true
    exit 1
}

web_login() {
    printf '{"username":"%s","password":"%s"}\n' "$owner_username" "$test_password" |
        curl --fail --silent --show-error --output /dev/null \
            --cookie-jar "$owner_cookie" --header 'Content-Type: application/json' \
            --data-binary @- "$gateway_origin/api/v1/auth/login"
}

artifact_digest() {
    curl --fail --silent --show-error --cookie "$owner_cookie" \
        "$gateway_origin/api/v1/namespaces/m2-e2e-team/repositories/private-image/tags/smoke" |
        sed -n 's/.*"digest":"\(sha256:[0-9a-f]*\)".*/\1/p'
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

go -C backend run ./cmd/registry-keygen --output-dir "$registry_auth_directory"
compose config --quiet
compose build api web
compose up --detach --wait postgres redis minio minio-init
compose --profile operations run --rm migrate

HUBCR_DATABASE_URL="$host_database_url" HUBCR_E2E_PASSWORD="$test_password" \
    go -C backend run ./internal/testsupport/m2seed

compose up --detach --wait api worker web registry gateway

set_docker_credentials
docker_cli pull "$base_image" >/dev/null
docker_cli tag "$base_image" "$public_image"
docker_cli tag "$base_image" "$private_image"
docker_cli push "$public_image" >/dev/null
docker_cli push "$private_image" >/dev/null
if ! HUBCR_DATABASE_URL="$host_database_url" go -C backend run ./internal/testsupport/m2assert; then
    fail "source deployment did not reconcile pushed Artifact metadata"
fi

web_login
source_digest=$(artifact_digest)
if [ -z "$source_digest" ]; then
    fail "source deployment did not expose the private Tag digest"
fi
if [ "$(curl --silent --output /dev/null --write-out '%{http_code}' "$gateway_origin/")" != "200" ]; then
    fail "production gateway did not serve the Web application"
fi

compose stop gateway registry web api worker
HUBCR_BACKUP_MAINTENANCE_CONFIRMED=true scripts/compose-backup.sh "$backup_directory"
if find "$backup_directory" -type f \( -name private.pem -o -name jwks.json -o -name event-token \) | grep -q .; then
    fail "backup unexpectedly included Registry keys or event secrets"
fi

compose down --volumes --remove-orphans
mv "$registry_auth_directory" "$old_auth_directory"
go -C backend run ./cmd/registry-keygen --output-dir "$registry_auth_directory"
if cmp -s "$old_auth_directory/private.pem" "$registry_auth_directory/private.pem"; then
    fail "recovery rehearsal did not rotate separately protected Registry signing material"
fi

compose up --detach --wait postgres redis minio minio-init
HUBCR_RESTORE_CONFIRM=restore scripts/compose-restore.sh "$backup_directory"
compose up --detach --wait api worker web registry gateway

if ! HUBCR_DATABASE_URL="$host_database_url" go -C backend run ./internal/testsupport/m2assert; then
    fail "restored deployment lost Artifact metadata"
fi
rm -f "$owner_cookie"
web_login
restored_digest=$(artifact_digest)
if [ "$restored_digest" != "$source_digest" ]; then
    fail "restored Tag digest $restored_digest does not match source $source_digest"
fi

docker_cli image rm "$private_image" >/dev/null 2>&1 || true
set_docker_credentials
docker_cli pull "$private_image" >/dev/null
if [ "$(curl --silent --output /dev/null --write-out '%{http_code}' --cookie "$owner_cookie" "$gateway_origin/api/v1/auth/me")" != "200" ]; then
    fail "restored user could not authenticate through the production gateway"
fi

echo "M3 single-host Compose migration, backup, restore, login, private pull, and Digest consistency rehearsal passed"
