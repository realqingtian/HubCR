#!/bin/sh

set -eu

test_project=hubcr-m2-registry-e2e
postgres_port=${HUBCR_M2_E2E_POSTGRES_PORT:-55434}
redis_port=${HUBCR_M2_E2E_REDIS_PORT:-56380}
minio_port=${HUBCR_M2_E2E_MINIO_PORT:-59000}
minio_console_port=${HUBCR_M2_E2E_MINIO_CONSOLE_PORT:-59001}
registry_port=${HUBCR_M2_E2E_REGISTRY_PORT:-55001}
registry_debug_port=${HUBCR_M2_E2E_REGISTRY_DEBUG_PORT:-55002}
api_port=${HUBCR_M2_E2E_API_PORT:-18081}
web_port=${HUBCR_M3_E2E_WEB_PORT:-3101}
run_artifact_web_e2e=${HUBCR_M3_ARTIFACT_WEB_E2E:-false}
database_url="postgres://hubcr:hubcr-dev-only@127.0.0.1:$postgres_port/hubcr?sslmode=disable"
registry_origin="http://localhost:$registry_port"
api_origin="http://127.0.0.1:$api_port"
web_origin="http://127.0.0.1:$web_port"
registry_auth_dir=$(pwd)/.data/registry-auth
test_password=m2-e2e-password
event_token=$(openssl rand -hex 32)
owner_username=m2-e2e-owner
reader_username=m2-e2e-reader
outsider_username=m2-e2e-outsider
base_image=alpine:3.22
public_image="localhost:$registry_port/m2-e2e-team/public-image:smoke"
private_image="localhost:$registry_port/m2-e2e-team/private-image:smoke"
reader_image="localhost:$registry_port/m2-e2e-team/private-image:reader-denied"
log_directory=$(mktemp -d)
docker_config=$(mktemp -d)
api_pid=
web_pid=
owner_cookie="$log_directory/owner.cookies"
outsider_cookie="$log_directory/outsider.cookies"

# An existing empty config disables Docker CLI's macOS default credential-helper
# discovery, so this test never reads from or writes to the user's Keychain.
umask 077
printf '{"auths":{}}\n' >"$docker_config/config.json"

docker_cli() {
    docker --config "$docker_config" "$@"
}

set_docker_credentials() {
    username=$1
    password=$2
    encoded=$(printf '%s:%s' "$username" "$password" | base64 | tr -d '\n')
    printf '{"auths":{"localhost:%s":{"auth":"%s"}}}\n' \
        "$registry_port" "$encoded" >"$docker_config/config.json"
}

clear_docker_credentials() {
    printf '{"auths":{}}\n' >"$docker_config/config.json"
}

compose() {
    HUBCR_POSTGRES_PORT="$postgres_port" \
    HUBCR_REDIS_PORT="$redis_port" \
    HUBCR_MINIO_PORT="$minio_port" \
    HUBCR_MINIO_CONSOLE_PORT="$minio_console_port" \
    HUBCR_REGISTRY_PORT="$registry_port" \
    HUBCR_REGISTRY_DEBUG_PORT="$registry_debug_port" \
    HUBCR_API_PORT="$api_port" \
    HUBCR_REGISTRY_AUTH_DIR="$registry_auth_dir" \
    HUBCR_REGISTRY_EVENT_TOKEN="$event_token" \
    docker compose --project-name "$test_project" --env-file .env.example \
        --file deployments/compose/compose.yaml "$@"
}

cleanup() {
    if [ -n "$web_pid" ]; then
        kill "$web_pid" 2>/dev/null || true
        wait "$web_pid" 2>/dev/null || true
    fi
    if [ -n "$api_pid" ]; then
        kill "$api_pid" 2>/dev/null || true
        wait "$api_pid" 2>/dev/null || true
    fi
    compose down --volumes --remove-orphans
    docker_cli image rm "$public_image" "$private_image" "$reader_image" >/dev/null 2>&1 || true
    rm -rf "$log_directory" "$docker_config"
}

wait_for_ready() {
    url=$1
    expected=$2
    attempts=0
    while [ "$attempts" -lt 80 ]; do
        status=$(curl --silent --output /dev/null --write-out '%{http_code}' "$url" || true)
        if [ "$status" = "$expected" ]; then
            return 0
        fi
        attempts=$((attempts + 1))
        sleep 0.25
    done
    return 1
}

fail() {
    message=$1
    echo "$message" >&2
    if [ -f "$log_directory/api.log" ]; then
        cat "$log_directory/api.log" >&2
    fi
    exit 1
}

web_login() {
    username=$1
    cookie_file=$2
    printf '{"username":"%s","password":"%s"}\n' "$username" "$test_password" |
        curl --fail --silent --show-error --output /dev/null \
            --cookie-jar "$cookie_file" --header 'Content-Type: application/json' \
            --data-binary @- "$api_origin/api/v1/auth/login"
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

go -C backend run ./cmd/registry-keygen --output-dir "$registry_auth_dir"
compose up --detach --wait

HUBCR_DATABASE_URL="$database_url" HUBCR_E2E_PASSWORD="$test_password" \
    go -C backend run ./internal/testsupport/m2seed

HUBCR_DATABASE_URL="$database_url" \
HUBCR_API_ADDRESS="127.0.0.1:$api_port" \
HUBCR_REGISTRY_AUTH_ENABLED=true \
HUBCR_REGISTRY_EXTERNAL_URL="$registry_origin" \
HUBCR_REGISTRY_ALLOW_INSECURE_HTTP=true \
HUBCR_REGISTRY_SERVICE=hubcr-registry \
HUBCR_REGISTRY_ISSUER=hubcr-token-service \
HUBCR_REGISTRY_TOKEN_TTL=5m \
HUBCR_REGISTRY_TOKEN_PRIVATE_KEY_FILE="$registry_auth_dir/private.pem" \
HUBCR_REGISTRY_TOKEN_JWKS_FILE="$registry_auth_dir/jwks.json" \
HUBCR_REGISTRY_EVENT_TOKEN="$event_token" \
    go -C backend run ./cmd/api >"$log_directory/api.log" 2>&1 &
api_pid=$!

if ! wait_for_ready "$api_origin/api/v1/health/ready" 200; then
    fail "control plane did not become ready"
fi
if ! wait_for_ready "$registry_origin/v2/" 401; then
    fail "Registry did not return a Bearer challenge"
fi
if ! wait_for_ready "http://127.0.0.1:$registry_debug_port/metrics" 200; then
    fail "Distribution metrics did not become ready"
fi
curl --silent --dump-header "$log_directory/challenge.headers" --output /dev/null "$registry_origin/v2/"
if ! grep -qi "www-authenticate: Bearer realm=\"$registry_origin/token\",service=\"hubcr-registry\"" "$log_directory/challenge.headers"; then
    fail "Registry challenge did not contain the configured realm and service"
fi

# Docker Desktop auto-selects macOS Keychain for `docker login`, even with an
# isolated --config directory. Populate only this temporary config explicitly;
# subsequent real Docker operations still execute the complete Basic -> token ->
# Bearer challenge flow without reading or modifying the user's credentials.
owner_login_status=$(
    printf 'user = "%s:%s"\n' "$owner_username" "$test_password" |
        curl --silent --output /dev/null --write-out '%{http_code}' --config - --get \
            --data-urlencode service=hubcr-registry \
            --data-urlencode offline_token=true \
            "$registry_origin/token"
)
if [ "$owner_login_status" != "200" ]; then
    fail "owner Registry credential exchange returned HTTP $owner_login_status"
fi
set_docker_credentials "$owner_username" "$test_password"
docker_cli pull "$base_image" >/dev/null
docker_cli tag "$base_image" "$public_image"
docker_cli tag "$base_image" "$private_image"
docker_cli push "$public_image" >/dev/null
docker_cli push "$private_image" >/dev/null
if ! HUBCR_DATABASE_URL="$database_url" go -C backend run ./internal/testsupport/m2assert; then
    compose logs registry >&2
    fail "Distribution notifications did not reconcile Artifact metadata"
fi
clear_docker_credentials
docker_cli image rm "$public_image" "$private_image" >/dev/null

docker_cli pull "$public_image" >/dev/null
if docker_cli pull "$private_image" >"$log_directory/anonymous-private.log" 2>&1; then
    fail "anonymous private pull unexpectedly succeeded"
fi

set_docker_credentials "$reader_username" "$test_password"
docker_cli pull "$private_image" >/dev/null
docker_cli tag "$base_image" "$reader_image"
if docker_cli push "$reader_image" >"$log_directory/reader-push.log" 2>&1; then
    fail "READER push unexpectedly succeeded"
fi
clear_docker_credentials
if ! HUBCR_DATABASE_URL="$database_url" go -C backend run ./internal/testsupport/m2assert; then
    fail "denied push changed Artifact metadata"
fi

set_docker_credentials "$outsider_username" "$test_password"
if docker_cli pull "$private_image" >"$log_directory/outsider-pull.log" 2>&1; then
    fail "wrong-organization private pull unexpectedly succeeded"
fi
clear_docker_credentials

invalid_login_status=$(
    printf 'user = "%s:%s"\n' "$owner_username" wrong-password |
        curl --silent --output "$log_directory/invalid-login.log" \
            --write-out '%{http_code}' --config - --get \
            --data-urlencode service=hubcr-registry \
            --data-urlencode offline_token=true \
            "$registry_origin/token"
)
if [ "$invalid_login_status" != "401" ]; then
    fail "invalid Registry credentials returned HTTP $invalid_login_status instead of 401"
fi

token_response=$(
    printf 'user = "%s:%s"\n' "$owner_username" "$test_password" |
        curl --fail --silent --config - --get \
            --data-urlencode service=hubcr-registry \
            --data-urlencode scope=repository:m2-e2e-team/private-image:pull \
            "$registry_origin/token"
)
token=$(printf '%s' "$token_response" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
if [ -z "$token" ]; then
    fail "token response did not contain a token"
fi
cross_status=$(
    printf 'header = "Authorization: Bearer %s"\n' "$token" |
        curl --silent --output /dev/null --write-out '%{http_code}' --config - \
            "$registry_origin/v2/m2-e2e-team/public-image/tags/list"
)
if [ "$cross_status" != "401" ]; then
    fail "cross-repository token reuse returned HTTP $cross_status instead of 401"
fi
pull_only_push_status=$(
    printf 'header = "Authorization: Bearer %s"\nrequest = "POST"\n' "$token" |
        curl --silent --output "$log_directory/pull-only-push.log" \
            --write-out '%{http_code}' --config - \
            "$registry_origin/v2/m2-e2e-team/private-image/blobs/uploads/"
)
if [ "$pull_only_push_status" != "401" ]; then
    fail "pull-only token used for push returned HTTP $pull_only_push_status instead of 401"
fi

expired_token=$(go -C backend run ./internal/testsupport/m2token "$registry_auth_dir/private.pem")
expired_status=$(
    printf 'header = "Authorization: Bearer %s"\n' "$expired_token" |
        curl --silent --output "$log_directory/expired-token.log" \
            --write-out '%{http_code}' --config - \
            "$registry_origin/v2/m2-e2e-team/private-image/tags/list"
)
if [ "$expired_status" != "401" ]; then
    fail "expired token returned HTTP $expired_status instead of 401"
fi

token_prefix=${token%.*}
token_signature=${token##*.}
case "$token_signature" in
    A*) tampered_signature="B${token_signature#?}" ;;
    *) tampered_signature="A${token_signature#?}" ;;
esac
tampered_token="$token_prefix.$tampered_signature"
tampered_status=$(
    printf 'header = "Authorization: Bearer %s"\n' "$tampered_token" |
        curl --silent --output "$log_directory/tampered-token.log" \
            --write-out '%{http_code}' --config - \
            "$registry_origin/v2/m2-e2e-team/private-image/tags/list"
)
if [ "$tampered_status" != "401" ]; then
    fail "invalid-signature token returned HTTP $tampered_status instead of 401"
fi

web_login "$owner_username" "$owner_cookie"
owner_tag_detail=$(
    curl --fail --silent --show-error --cookie "$owner_cookie" \
        "$api_origin/api/v1/namespaces/m2-e2e-team/repositories/private-image/tags/smoke"
)
case "$owner_tag_detail" in
    *'"name":"smoke"'*'"digest":"sha256:'*'"media_type":'*'"size_bytes":'*) ;;
    *) fail "owner Artifact/Tag detail omitted reconciled metadata" ;;
esac
web_login "$outsider_username" "$outsider_cookie"
outsider_private_status=$(
    curl --silent --output "$log_directory/outsider-artifacts.log" --write-out '%{http_code}' \
        --cookie "$outsider_cookie" \
        "$api_origin/api/v1/namespaces/m2-e2e-team/repositories/private-image/artifacts"
)
if [ "$outsider_private_status" != "404" ]; then
    fail "private Artifact API returned HTTP $outsider_private_status to an outsider instead of 404"
fi
outsider_public_status=$(
    curl --silent --output "$log_directory/public-artifacts.json" --write-out '%{http_code}' \
        --cookie "$outsider_cookie" \
        "$api_origin/api/v1/namespaces/m2-e2e-team/repositories/public-image/artifacts"
)
if [ "$outsider_public_status" != "200" ] ||
    ! grep -Fq '"digest":"sha256:' "$log_directory/public-artifacts.json"; then
    fail "public Artifact API did not expose reconciled metadata to an authenticated outsider"
fi
curl --fail --silent --show-error "$api_origin/internal/metrics" >"$log_directory/control-plane.metrics"
for metric in \
    'hubcr_registry_token_requests_total{outcome="issued"}' \
    'hubcr_registry_token_actions_total{action="push",decision="denied"}' \
    'hubcr_registry_notification_requests_total{outcome="accepted"}' \
    'hubcr_registry_notification_events_total{outcome="processed"}'
do
    if ! grep -Eq "^$metric [1-9][0-9]*$" "$log_directory/control-plane.metrics"; then
        fail "control-plane Registry metric did not record runtime activity: $metric"
    fi
done
curl --fail --silent --show-error "http://127.0.0.1:$registry_debug_port/metrics" \
    >"$log_directory/distribution.metrics"
if ! grep -Fq '# HELP' "$log_directory/distribution.metrics"; then
    fail "Distribution Prometheus endpoint did not expose metrics"
fi
curl --fail --silent --show-error "http://127.0.0.1:$registry_debug_port/debug/vars" \
    >"$log_directory/distribution-vars.json"
if ! grep -Fq 'notifications' "$log_directory/distribution-vars.json"; then
    fail "Distribution debug variables did not expose notification queue state"
fi
compose logs gateway >"$log_directory/gateway.log"
if ! grep -Eq '"service":"hubcr-gateway".*"request_id":"[^"]+".*"route":"registry".*"status":401.*"registry_challenge":true' \
    "$log_directory/gateway.log"; then
    fail "gateway logs did not record a structured Registry challenge"
fi
if ! grep -Fq '"outcome":"issued"' "$log_directory/api.log" ||
    ! grep -Fq '"outcome":"accepted"' "$log_directory/api.log" ||
    ! grep -Fq '"request_id":' "$log_directory/api.log"; then
    fail "control-plane logs omitted correlated Registry decisions"
fi
if grep -Fq "$test_password" "$log_directory/api.log" ||
    grep -Fq "$token" "$log_directory/api.log" ||
    grep -Fq "$expired_token" "$log_directory/api.log" ||
    grep -Fq "$tampered_token" "$log_directory/api.log" ||
    grep -Fq "$test_password" "$log_directory/gateway.log" ||
    grep -Fq "$token" "$log_directory/gateway.log"; then
    fail "Registry operational logs leaked credentials or tokens"
fi

if [ "$run_artifact_web_e2e" = "true" ]; then
    HUBCR_CONTROL_PLANE_URL="$api_origin" bun run --cwd frontend build
    HUBCR_CONTROL_PLANE_URL="$api_origin" \
        bun run --cwd frontend start --hostname 127.0.0.1 --port "$web_port" \
        >"$log_directory/web.log" 2>&1 &
    web_pid=$!
    if ! wait_for_ready "$web_origin" 200; then
        cat "$log_directory/web.log" >&2
        fail "M3 Artifact web application did not become ready"
    fi
    HUBCR_E2E_WEB_ORIGIN="$web_origin" \
    HUBCR_M3_E2E_USERNAME="$owner_username" \
    HUBCR_M3_E2E_PASSWORD="$test_password" \
        frontend/node_modules/.bin/playwright test \
            --config frontend/playwright.fullstack.config.ts \
            m3-artifact-journey.spec.ts
fi

echo "M2 Registry security matrix, reconciliation, Artifact API, and operational telemetry checks passed"
if [ "$run_artifact_web_e2e" = "true" ]; then
    echo "M3 real Push-to-Web Artifact discovery journey passed"
fi
