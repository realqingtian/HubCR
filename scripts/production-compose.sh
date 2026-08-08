#!/bin/sh

set -eu

script_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repository_root=$(CDPATH= cd -- "$script_directory/.." && pwd)
environment_file=${HUBCR_PRODUCTION_ENV_FILE:-$repository_root/.env.production}
project_name=${HUBCR_COMPOSE_PROJECT_NAME:-hubcr}

if [ -n "${HUBCR_COMPOSE_OVERRIDE_FILE:-}" ]; then
    exec docker compose \
        --project-name "$project_name" \
        --env-file "$environment_file" \
        --file "$repository_root/deployments/compose/compose.yaml" \
        --file "$repository_root/deployments/compose/compose.production.yaml" \
        --file "$HUBCR_COMPOSE_OVERRIDE_FILE" \
        "$@"
fi

exec docker compose \
    --project-name "$project_name" \
    --env-file "$environment_file" \
    --file "$repository_root/deployments/compose/compose.yaml" \
    --file "$repository_root/deployments/compose/compose.production.yaml" \
    "$@"
