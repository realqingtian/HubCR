#!/bin/sh

set -eu

repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repository_root"

test_project=hubcr-m4-security-e2e
postgres_port=${HUBCR_M4_SECURITY_POSTGRES_PORT:-55436}
gateway_port=${HUBCR_M4_SECURITY_GATEWAY_PORT:-55004}
test_password=m4-security-password
owner_username=m2-e2e-owner
vulnerable_base=alpine:3.12
vulnerable_image="localhost:$gateway_port/m2-e2e-team/private-image:vulnerable"
clean_image="localhost:$gateway_port/m2-e2e-team/private-image:clean"
host_database_url="postgres://hubcr:hubcr-test-only@127.0.0.1:$postgres_port/hubcr?sslmode=disable"
gateway_origin="http://localhost:$gateway_port"
temporary_directory=$(mktemp -d)
registry_auth_directory=$temporary_directory/registry-auth
docker_configuration=$temporary_directory/docker
cosign_key_directory=$temporary_directory/cosign-keys
owner_cookie=$temporary_directory/owner.cookies
event_token=$(openssl rand -hex 32)
cosign_password=m4-cosign-test-password
cosign_image=ghcr.io/sigstore/cosign/cosign@sha256:de9c65609e6bde17e6b48de485ee788407c9502fa08b8f4459f595b21f56cd00

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
export HUBCR_API_IMAGE=hubcr-api:m4-security-e2e
export HUBCR_WEB_IMAGE=hubcr-web:m4-security-e2e
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

cosign_cli() {
    docker run --rm --user 0 \
        --network "${test_project}_default" \
        --env "COSIGN_PASSWORD=$cosign_password" \
        --env DOCKER_CONFIG=/docker \
        --volume "$docker_configuration:/docker:ro" \
        --volume "$cosign_key_directory:/keys" \
        "$cosign_image" "$@"
}

cosign_generate() {
    docker run --rm --user 0 \
        --env "COSIGN_PASSWORD=$cosign_password" \
        --volume "$cosign_key_directory:/keys" \
        "$cosign_image" "$@"
}

set_docker_credentials() {
    encoded=$(printf '%s:%s' "$owner_username" "$test_password" | base64 | tr -d '\n')
    printf '{"auths":{"localhost:%s":{"auth":"%s"},"host.docker.internal:%s":{"auth":"%s"},"registry:5000":{"auth":"%s"}}}\n' \
        "$gateway_port" "$encoded" "$gateway_port" "$encoded" "$encoded" \
        >"$docker_configuration/config.json"
}

cleanup() {
	if [ "${HUBCR_M4_SECURITY_KEEP_ON_FAILURE:-false}" = "true" ] && [ "${e2e_succeeded:-false}" != "true" ]; then
		echo "M4 security E2E environment retained: project=$test_project temporary_directory=$temporary_directory" >&2
		return
	fi
    compose down --volumes --remove-orphans >/dev/null 2>&1 || true
    docker_cli image rm "$vulnerable_image" "$clean_image" >/dev/null 2>&1 || true
    docker image rm hubcr-m4-clean:e2e "$HUBCR_API_IMAGE" "$HUBCR_WEB_IMAGE" >/dev/null 2>&1 || true
    rm -rf -- "$temporary_directory"
}

fail() {
    echo "$1" >&2
    compose logs api worker registry >&2 || true
    exit 1
}

web_login() {
    printf '{"username":"%s","password":"%s"}\n' "$owner_username" "$test_password" |
        curl --fail --silent --show-error --output /dev/null \
            --cookie-jar "$owner_cookie" --header 'Content-Type: application/json' \
            --data-binary @- "$gateway_origin/api/v1/auth/login"
}

artifact_digest() {
    tag=$1
    curl --fail --silent --show-error --cookie "$owner_cookie" \
        "$gateway_origin/api/v1/namespaces/m2-e2e-team/repositories/private-image/tags/$tag" 2>/dev/null |
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

security_detail() {
    digest=$1
    curl --fail --silent --show-error --cookie "$owner_cookie" \
        "$gateway_origin/api/v1/namespaces/m2-e2e-team/repositories/private-image/artifacts/$digest/security"
}

wait_for_security() {
    digest=$1
    attempts=0
    while [ "$attempts" -lt 240 ]; do
        detail=$(security_detail "$digest" 2>/dev/null || true)
        scan_state=$(printf '%s' "$detail" | sed -n 's/.*"scan":{"state":"\([A-Z]*\)".*/\1/p')
        sbom_state=$(printf '%s' "$detail" | sed -n 's/.*"sbom":{"state":"\([A-Z]*\)".*/\1/p')
        if [ "$scan_state" = "COMPLETED" ] && [ "$sbom_state" = "COMPLETED" ]; then
            printf '%s\n' "$detail"
            return 0
        fi
        if [ "$scan_state" = "FAILED" ] || [ "$sbom_state" = "FAILED" ]; then
            printf '%s\n' "$detail" >&2
            return 1
        fi
        attempts=$((attempts + 1))
        sleep 2
    done
    return 1
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

wait_for_retry() {
    attempts=0
    while [ "$attempts" -lt 60 ]; do
        retry_count=$(compose exec -T postgres psql --username hubcr --dbname hubcr --tuples-only --no-align \
            --command "SELECT count(*) FROM jobs WHERE kind IN ('TRIVY_SCAN','TRIVY_SBOM') AND state = 'QUEUED' AND attempt_count >= 1 AND last_error_code IS NOT NULL" 2>/dev/null || true)
        if [ "${retry_count:-0}" -ge 1 ]; then
            printf '%s\n' "$retry_count"
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

json_value() {
    expression=$1
    python3 -c "import json,sys; value=json.load(sys.stdin); print($expression)"
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

go -C backend run ./cmd/registry-keygen --output-dir "$registry_auth_directory"
cosign_generate generate-key-pair --output-key-prefix /keys/old >/dev/null
cosign_generate generate-key-pair --output-key-prefix /keys/current >/dev/null
old_fingerprint=$(key_fingerprint "$cosign_key_directory/old.pub")
current_fingerprint=$(key_fingerprint "$cosign_key_directory/current.pub")
printf '{"builder":"hubcr-m4-e2e","buildType":"https://hubcr.io/test-build/v1"}\n' \
    >"$cosign_key_directory/predicate.json"
compose config --quiet
compose build api web
compose up --detach --wait postgres redis minio minio-init
compose --profile operations run --rm migrate
HUBCR_DATABASE_URL="$host_database_url" HUBCR_E2E_PASSWORD="$test_password" \
    go -C backend run ./internal/testsupport/m2seed
compose up --detach --wait api worker web registry gateway
compose stop worker >/dev/null

set_docker_credentials
docker_cli pull "$vulnerable_base" >/dev/null
docker_cli tag "$vulnerable_base" "$vulnerable_image"
docker_cli build \
    --file backend/internal/testsupport/fixtures/m4-clean.Dockerfile \
    --tag hubcr-m4-clean:e2e . >/dev/null
docker_cli tag hubcr-m4-clean:e2e "$clean_image"
docker_cli push "$vulnerable_image" >/dev/null
docker_cli push "$clean_image" >/dev/null

web_login
vulnerable_digest=$(wait_for_digest vulnerable) || fail "vulnerable Artifact was not reconciled"
clean_digest=$(wait_for_digest clean) || fail "clean Artifact was not reconciled"
queued_before_restart=$(compose exec -T postgres psql --username hubcr --dbname hubcr --tuples-only --no-align \
    --command "SELECT count(*) FROM jobs WHERE kind IN ('TRIVY_SCAN','TRIVY_SBOM') AND state = 'QUEUED' AND payload->>'digest' IN ('$vulnerable_digest','$clean_digest')")
if [ "$queued_before_restart" -ne 4 ]; then
    fail "worker-stop durability mismatch: queued_jobs=$queued_before_restart"
fi
compose stop registry >/dev/null
docker start "${test_project}-worker-1" >/dev/null
retry_count=$(wait_for_retry) || fail "Registry outage did not produce a durable retriable security job"
compose up --detach --wait registry
vulnerable_detail=$(wait_for_security "$vulnerable_digest") || fail "vulnerable scan/SBOM did not complete"
clean_detail=$(wait_for_security "$clean_digest") || fail "clean scan/SBOM did not complete"

cosign_cli sign --yes --allow-http-registry --new-bundle-format=false --use-signing-config=false --tlog-upload=false \
    --key /keys/old.key --annotations "hubcr.io/key-fingerprint=$old_fingerprint" \
    "registry:5000/m2-e2e-team/private-image@$vulnerable_digest" >/dev/null
cosign_cli sign --yes --allow-http-registry --new-bundle-format=false --use-signing-config=false --tlog-upload=false \
    --key /keys/current.key --annotations "hubcr.io/key-fingerprint=$current_fingerprint" \
    "registry:5000/m2-e2e-team/private-image@$vulnerable_digest" >/dev/null
cosign_cli attest --yes --allow-http-registry --new-bundle-format=false --use-signing-config=false --tlog-upload=false \
    --key /keys/current.key --type custom --predicate /keys/predicate.json \
    "registry:5000/m2-e2e-team/private-image@$vulnerable_digest" >/dev/null
cosign_cli generate --allow-http-registry --annotations "hubcr.io/key-fingerprint=$current_fingerprint" \
    "registry:5000/m2-e2e-team/private-image@$vulnerable_digest" >"$cosign_key_directory/invalid.payload"
cosign_generate sign-blob --yes --new-bundle-format=false --use-signing-config=false \
    --tlog-upload=false --key /keys/current.key /keys/invalid.payload \
    >"$cosign_key_directory/invalid.sig"
python3 -c 'import base64,pathlib; p=pathlib.Path("'$cosign_key_directory'/invalid.sig"); raw=bytearray(base64.b64decode(p.read_text().strip())); raw[-1] ^= 1; p.write_text(base64.b64encode(raw).decode()+"\n")'
cosign_cli attach signature --allow-http-registry --payload /keys/invalid.payload \
    --signature /keys/invalid.sig "registry:5000/m2-e2e-team/private-image@$vulnerable_digest" >/dev/null
HUBCR_DATABASE_URL="$host_database_url" \
    HUBCR_E2E_COSIGN_PUBLIC_KEY_FILE="$cosign_key_directory/old.pub" \
    HUBCR_E2E_COSIGN_KEY_NAME=old-release \
    go -C backend run ./internal/testsupport/m4trustseed >/dev/null
wait_for_verification "$vulnerable_digest" 1 || fail "signed Artifact policy-v1 verification did not complete"
wait_for_verification "$clean_digest" 1 || fail "unsigned Artifact policy-v1 verification did not complete"
HUBCR_DATABASE_URL="$host_database_url" \
    HUBCR_E2E_COSIGN_PUBLIC_KEY_FILE="$cosign_key_directory/current.pub" \
    HUBCR_E2E_COSIGN_KEY_NAME=current-release \
    go -C backend run ./internal/testsupport/m4trustseed >/dev/null
wait_for_verification "$vulnerable_digest" 2 || fail "signed Artifact policy-v2 verification did not complete"
wait_for_verification "$clean_digest" 2 || fail "unsigned Artifact policy-v2 verification did not complete"

vulnerable_count=$(printf '%s' "$vulnerable_detail" | json_value "value['scan']['finding_count']")
clean_count=$(printf '%s' "$clean_detail" | json_value "value['scan']['finding_count']")
scanner_version=$(printf '%s' "$vulnerable_detail" | json_value "value['scan']['tool']['scanner_version']")
database_schema=$(printf '%s' "$vulnerable_detail" | json_value "value['scan']['tool']['database_schema_version']")
database_updated_at=$(printf '%s' "$vulnerable_detail" | json_value "value['scan']['tool']['database_updated_at']")
if [ "$vulnerable_count" -lt 1 ]; then
    fail "known-vulnerable image produced no vulnerability findings"
fi
if [ "$clean_count" -ne 0 ]; then
    fail "scratch clean fixture produced $clean_count vulnerability findings"
fi
if [ "$scanner_version" != "0.72.0" ] || [ "$database_schema" -lt 1 ] || [ -z "$database_updated_at" ]; then
    fail "scan response omitted pinned scanner or vulnerability database evidence"
fi

workflow_count=$(compose exec -T postgres psql --username hubcr --dbname hubcr --tuples-only --no-align \
    --command "SELECT count(*) FROM security_workflows WHERE digest IN ('$vulnerable_digest','$clean_digest')")
job_count=$(compose exec -T postgres psql --username hubcr --dbname hubcr --tuples-only --no-align \
    --command "SELECT count(*) FROM jobs WHERE kind IN ('TRIVY_SCAN','TRIVY_SBOM') AND payload->>'digest' IN ('$vulnerable_digest','$clean_digest')")
sbom_count=$(compose exec -T postgres psql --username hubcr --dbname hubcr --tuples-only --no-align \
    --command "SELECT count(*) FROM security_sboms AS sbom JOIN security_workflows AS workflow ON workflow.id = sbom.workflow_id WHERE workflow.digest IN ('$vulnerable_digest','$clean_digest') AND sbom.document->>'bomFormat' = 'CycloneDX'")
if [ "$workflow_count" -ne 2 ] || [ "$job_count" -ne 4 ] || [ "$sbom_count" -ne 2 ]; then
    fail "durable workflow evidence mismatch: workflows=$workflow_count jobs=$job_count sboms=$sbom_count"
fi

verification_count=$(compose exec -T postgres psql --username hubcr --dbname hubcr --tuples-only --no-align \
    --command "SELECT count(*) FROM signature_verifications AS verification JOIN signature_workflows AS workflow ON workflow.id = verification.workflow_id WHERE workflow.digest IN ('$vulnerable_digest','$clean_digest') AND workflow.policy_version = 2 AND verification.cosign_version = 'v3.0.6'")
trusted_count=$(compose exec -T postgres psql --username hubcr --dbname hubcr --tuples-only --no-align \
    --command "SELECT count(*) FROM signature_evidence AS evidence JOIN signature_workflows AS workflow ON workflow.id = evidence.workflow_id WHERE workflow.digest = '$vulnerable_digest' AND workflow.policy_version = 2 AND evidence.cryptographic_state = 'VALID' AND evidence.trust_state = 'TRUSTED'")
untrusted_count=$(compose exec -T postgres psql --username hubcr --dbname hubcr --tuples-only --no-align \
    --command "SELECT count(*) FROM signature_evidence AS evidence JOIN signature_workflows AS workflow ON workflow.id = evidence.workflow_id WHERE workflow.digest = '$vulnerable_digest' AND workflow.policy_version = 2 AND evidence.cryptographic_state = 'VALID' AND evidence.trust_state = 'UNTRUSTED'")
attestation_count=$(compose exec -T postgres psql --username hubcr --dbname hubcr --tuples-only --no-align \
    --command "SELECT count(*) FROM signature_evidence AS evidence JOIN signature_workflows AS workflow ON workflow.id = evidence.workflow_id WHERE workflow.digest = '$vulnerable_digest' AND workflow.policy_version = 2 AND evidence.kind = 'ATTESTATION' AND evidence.cryptographic_state = 'VALID' AND evidence.trust_state = 'TRUSTED'")
invalid_count=$(compose exec -T postgres psql --username hubcr --dbname hubcr --tuples-only --no-align \
    --command "SELECT count(*) FROM signature_evidence AS evidence JOIN signature_workflows AS workflow ON workflow.id = evidence.workflow_id WHERE workflow.digest = '$vulnerable_digest' AND workflow.policy_version = 2 AND evidence.cryptographic_state = 'INVALID' AND evidence.trust_state = 'NOT_EVALUATED'")
unsigned_count=$(compose exec -T postgres psql --username hubcr --dbname hubcr --tuples-only --no-align \
    --command "SELECT count(*) FROM signature_evidence AS evidence JOIN signature_workflows AS workflow ON workflow.id = evidence.workflow_id WHERE workflow.digest = '$clean_digest' AND workflow.policy_version = 2")
historical_policy_count=$(compose exec -T postgres psql --username hubcr --dbname hubcr --tuples-only --no-align \
    --command "SELECT count(DISTINCT workflow.policy_version) FROM signature_verifications AS verification JOIN signature_workflows AS workflow ON workflow.id = verification.workflow_id WHERE workflow.digest IN ('$vulnerable_digest','$clean_digest')")
evidence_summary=$(compose exec -T postgres psql --username hubcr --dbname hubcr --tuples-only --no-align \
    --command "SELECT concat(workflow.policy_version, ':', evidence.kind, ':', evidence.signer_type, ':', evidence.cryptographic_state, ':', evidence.trust_state, ':', count(*)) FROM signature_evidence AS evidence JOIN signature_workflows AS workflow ON workflow.id = evidence.workflow_id WHERE workflow.digest = '$vulnerable_digest' GROUP BY workflow.policy_version, evidence.kind, evidence.signer_type, evidence.cryptographic_state, evidence.trust_state ORDER BY workflow.policy_version, evidence.kind, evidence.signer_type")
if [ "$verification_count" -ne 2 ] || [ "$trusted_count" -lt 1 ] || \
    [ "$untrusted_count" -lt 1 ] || [ "$attestation_count" -lt 1 ] || [ "$invalid_count" -lt 1 ] || \
    [ "$unsigned_count" -ne 0 ] || [ "$historical_policy_count" -ne 2 ]; then
    fail "signature trust evidence mismatch: verifications=$verification_count trusted=$trusted_count untrusted=$untrusted_count invalid=$invalid_count attestations=$attestation_count unsigned=$unsigned_count policy_versions=$historical_policy_count evidence=$evidence_summary"
fi

verified_detail=$(security_detail "$vulnerable_digest") || fail "signed Artifact security API was unavailable"
unsigned_detail=$(security_detail "$clean_digest") || fail "unsigned Artifact security API was unavailable"
api_signature_state=$(printf '%s' "$verified_detail" | json_value "value['signature']['state']")
api_policy_version=$(printf '%s' "$verified_detail" | json_value "value['signature']['policy_version']")
api_trusted_count=$(printf '%s' "$verified_detail" | json_value "sum(item['trust_state'] == 'TRUSTED' for item in value['signature']['evidence'])")
api_untrusted_count=$(printf '%s' "$verified_detail" | json_value "sum(item['trust_state'] == 'UNTRUSTED' for item in value['signature']['evidence'])")
api_attestation_count=$(printf '%s' "$verified_detail" | json_value "sum(item['kind'] == 'ATTESTATION' for item in value['signature']['evidence'])")
api_invalid_count=$(printf '%s' "$verified_detail" | json_value "sum(item['cryptographic_state'] == 'INVALID' for item in value['signature']['evidence'])")
api_unsigned_count=$(printf '%s' "$unsigned_detail" | json_value "len(value['signature']['evidence'])")
if [ "$api_signature_state" != "COMPLETED" ] || [ "$api_policy_version" -ne 2 ] || \
    [ "$api_trusted_count" -lt 1 ] || [ "$api_untrusted_count" -lt 1 ] || \
    [ "$api_invalid_count" -lt 1 ] || [ "$api_attestation_count" -lt 1 ] || [ "$api_unsigned_count" -ne 0 ]; then
    fail "authorized signature API mismatch: state=$api_signature_state policy=$api_policy_version trusted=$api_trusted_count untrusted=$api_untrusted_count invalid=$api_invalid_count attestations=$api_attestation_count unsigned=$api_unsigned_count"
fi

e2e_succeeded=true
echo "M4 pinned Trivy, SBOM, worker restart/retry=$retry_count, Cosign trusted/untrusted/invalid/unsigned, policy re-evaluation, exact intent, and version evidence passed"
