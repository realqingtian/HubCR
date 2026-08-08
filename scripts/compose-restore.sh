#!/bin/sh

set -eu

if [ "$#" -ne 1 ]; then
    echo "usage: scripts/compose-restore.sh BACKUP_DIRECTORY" >&2
    exit 2
fi
if [ "${HUBCR_RESTORE_CONFIRM:-}" != "restore" ]; then
    echo "restore refused: set HUBCR_RESTORE_CONFIRM=restore to replace PostgreSQL and Registry object data" >&2
    exit 2
fi

script_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repository_root=$(CDPATH= cd -- "$script_directory/.." && pwd)
backup_directory=$1
case "$backup_directory" in
    /*) ;;
    *) backup_directory=$repository_root/$backup_directory ;;
esac

if [ ! -d "$backup_directory" ] || [ -L "$backup_directory" ]; then
    echo "restore refused: backup directory is missing or is a symbolic link: $backup_directory" >&2
    exit 2
fi
for required in manifest.txt SHA256SUMS postgres.dump registry/hubcr-registry; do
    if [ ! -e "$backup_directory/$required" ]; then
        echo "restore refused: backup is missing $required" >&2
        exit 2
    fi
done
if ! grep -Fxq 'format=hubcr-compose-backup-v1' "$backup_directory/manifest.txt"; then
    echo "restore refused: unsupported backup format" >&2
    exit 2
fi
(
    cd "$backup_directory"
    shasum -a 256 --check SHA256SUMS
)

compose() {
    "$script_directory/production-compose.sh" "$@"
}

for service in api worker web gateway registry; do
    if [ -n "$(compose ps --status running --quiet "$service")" ]; then
        echo "restore refused: $service is still running" >&2
        exit 2
    fi
done

compose exec -T postgres sh -c '
    export PGPASSWORD="$POSTGRES_PASSWORD"
    exec pg_restore --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
        --clean --if-exists --no-owner --no-privileges --exit-on-error --single-transaction
' <"$backup_directory/postgres.dump"

compose run --rm --no-deps \
    --user "$(id -u):$(id -g)" \
    --volume "$backup_directory/registry:/backup:ro" \
    --entrypoint /bin/sh minio-init -c '
        export MC_CONFIG_DIR=/tmp/mc
        mc alias set target http://minio:9000 "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD" >/dev/null
        mc mb --ignore-existing target/hubcr-registry >/dev/null
        mc mirror --overwrite --remove /backup/hubcr-registry target/hubcr-registry
    '

compose --profile operations run --rm migrate

echo "HubCR data restored and current database migrations applied."
echo "Start the application only after supplying the separately protected Registry keys and secrets."
