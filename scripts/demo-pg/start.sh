#!/usr/bin/env bash
# Starts a PostgreSQL 17 container and loads the Rowan demo database.
#
#   scripts/demo-pg/start.sh          # start and seed
#   docker rm -f rowan-demo-pg        # remove it again
#
# The seed takes 2 to 5 minutes and uses about 6 GB of disk.
set -euo pipefail

name=${ROWAN_DEMO_PG_NAME:-rowan-demo-pg}
port=${ROWAN_DEMO_PG_PORT:-55433}
dir=$(cd "$(dirname "$0")" && pwd)

if docker ps -a --format '{{.Names}}' | grep -qx "$name"; then
  echo "The container $name exists. To start again, run: docker rm -f $name" >&2
  exit 1
fi

docker run -d --name "$name" \
  -e POSTGRES_PASSWORD=rowan -e POSTGRES_DB=acme \
  -p "127.0.0.1:$port:5432" --shm-size=1g \
  postgres:17-alpine \
  -c shared_buffers=1GB -c work_mem=64MB -c maintenance_work_mem=1GB \
  -c max_wal_size=8GB -c checkpoint_timeout=30min >/dev/null

until docker exec "$name" pg_isready -U postgres -d acme -q; do sleep 0.5; done
# pg_isready answers during the init restart. Wait for the final server.
sleep 2
until docker exec "$name" psql -U postgres -d acme -qtAc 'SELECT 1' >/dev/null 2>&1; do sleep 0.5; done

echo "Loading the demo data. This takes a few minutes."
docker exec -i "$name" psql -U postgres -d acme -q < "$dir/seed.sql"

cat <<INFO

The demo database is ready.
  Host:     127.0.0.1
  Port:     $port
  Database: acme
  User:     postgres
  Password: rowan
INFO
