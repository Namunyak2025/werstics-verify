#!/bin/sh
set -eu

echo "Running Werstics Verify database migrations..."
bash /app/scripts/migrate.sh

echo "Starting Werstics Verify..."
exec /app/werstics-verify
