#!/bin/sh

set -eu

if [ "$#" -ne 1 ]; then
    echo "usage: scripts/compose-backup.sh BACKUP_DIRECTORY" >&2
    exit 2
fi
if [ "${HUBCR_BACKUP_MAINTENANCE_CONFIRMED:-}" != "true" ]; then
    echo "backup refused: stop API, worker, Web, gateway, and Registry writes, then set HUBCR_BACKUP_MAINTENANCE_CONFIRMED=true" >&2
    exit 2
fi

script_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repository_root=$(CDPATH= cd -- "$script_directory/.." && pwd)
destination=$1
case "$destination" in
    /*) ;;
    *) destination=$repository_root/$destination ;;
esac

if [ -e "$destination" ] || [ -L "$destination" ]; then
    echo "backup refused: destination already exists: $destination" >&2
    exit 2
fi

parent_directory=$(dirname -- "$destination")
mkdir -p "$parent_directory"
temporary_directory=$(mktemp -d "$parent_directory/.hubcr-backup.XXXXXX")
cleanup() {
    rm -rf -- "$temporary_directory"
}
trap cleanup EXIT INT TERM
umask 077

compose() {
    "$script_directory/production-compose.sh" "$@"
}

for service in api worker web gateway registry; do
    if [ -n "$(compose ps --status running --quiet "$service")" ]; then
        echo "backup refused: $service is still running" >&2
        exit 2
    fi
done

compose exec -T postgres sh -c '
    export PGPASSWORD="$POSTGRES_PASSWORD"
    exec pg_dump --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
        --format=custom --no-owner --no-privileges
' >"$temporary_directory/postgres.dump"

mkdir "$temporary_directory/registry"
compose run --rm --no-deps \
    --user "$(id -u):$(id -g)" \
    --volume "$temporary_directory/registry:/backup" \
    --entrypoint /bin/sh minio-init -c '
        export MC_CONFIG_DIR=/tmp/mc
        mc alias set source http://minio:9000 "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD" >/dev/null
        mc mirror --overwrite --remove source/hubcr-registry /backup/hubcr-registry
    '

created_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
cat >"$temporary_directory/manifest.txt" <<EOF
format=hubcr-compose-backup-v1
created_at=$created_at
deployment=single-host-docker-compose
database_format=postgres-custom
registry_bucket=hubcr-registry
secrets_included=false
EOF

(
    cd "$temporary_directory"
    find . -type f ! -name SHA256SUMS -print | LC_ALL=C sort | while IFS= read -r path; do
        shasum -a 256 "$path"
    done >SHA256SUMS
)
chmod -R go-rwx "$temporary_directory"
mv "$temporary_directory" "$destination"
trap - EXIT INT TERM

echo "HubCR backup created at $destination"
echo "The backup contains credential hashes and Registry content; encrypt and protect it."
