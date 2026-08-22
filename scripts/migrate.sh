#!/usr/bin/env bash
set -euo pipefail

: "${WERSTICS_VERIFY_DATABASE_URL:?WERSTICS_VERIFY_DATABASE_URL is required}"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MIGRATIONS_DIR="${ROOT_DIR}/db/migrations"

PSQL=(
  psql
  "${WERSTICS_VERIFY_DATABASE_URL}"
  -v ON_ERROR_STOP=1
)

echo "Waiting for PostgreSQL..."
"${PSQL[@]}" -c "SELECT 1" >/dev/null

echo "Acquiring migration lock..."
"${PSQL[@]}" -c "SELECT pg_advisory_lock(784552019)" >/dev/null

cleanup() {
  "${PSQL[@]}" -c "SELECT pg_advisory_unlock(784552019)" >/dev/null 2>&1 || true
}

trap cleanup EXIT INT TERM

schema_exists="$(
  "${PSQL[@]}" -Atc \
    "SELECT to_regclass('public.schema_migrations') IS NOT NULL"
)"

for migration in "${MIGRATIONS_DIR}"/*_*.up.sql; do
  filename="$(basename "$migration")"
  version="${filename%%_*}"

  if ! [[ "$version" =~ ^[0-9]+$ ]]; then
    echo "Skipping migration with invalid version: $filename"
    continue
  fi

  version_num=$((10#$version))

  # Bootstrap migration creates schema_migrations itself.
  if [[ "$schema_exists" != "t" ]]; then
    if [[ "$version_num" -ne 1 ]]; then
      echo "schema_migrations does not exist before $filename" >&2
      echo "Refusing to apply migrations out of bootstrap order." >&2
      exit 1
    fi

    echo "Applying bootstrap migration: $filename"
    "${PSQL[@]}" -f "$migration"

    schema_exists="t"
    continue
  fi

  applied="$(
    "${PSQL[@]}" -Atc \
      "SELECT EXISTS (
         SELECT 1
         FROM schema_migrations
         WHERE version = ${version_num}
       )"
  )"

  if [[ "$applied" == "t" ]]; then
    echo "Already applied: $filename"
    continue
  fi

  # Migration 002 predates the migration ledger entry.
  # Its schema state is detectable, so reconcile the historical omission
  # without replaying ALTER TABLE statements on an already-upgraded DB.
  if [[ "$version_num" -eq 2 ]]; then
    payment_id_exists="$(
      "${PSQL[@]}" -Atc \
        "SELECT EXISTS (
           SELECT 1
           FROM information_schema.columns
           WHERE table_schema = 'public'
             AND table_name = 'payments'
             AND column_name = 'payment_id'
         )"
    )"

    if [[ "$payment_id_exists" == "t" ]]; then
      echo "Reconciling historical migration 002 ledger omission."
      "${PSQL[@]}" -c \
        "INSERT INTO schema_migrations (version)
         VALUES (2)
         ON CONFLICT (version) DO NOTHING"
      continue
    fi
  fi

  echo "Applying: $filename"
  "${PSQL[@]}" -f "$migration"

  # Migration 002 predates the migration-ledger convention.
  # Record it after the schema change succeeds so a clean installation
  # reaches a complete, contiguous migration state in one run.
  if [[ "$version_num" -eq 2 ]]; then
    "${PSQL[@]}" -c "
      INSERT INTO schema_migrations (version)
      VALUES (2)
      ON CONFLICT (version) DO NOTHING;
    "
  fi
done

latest_file="$(
  find "${MIGRATIONS_DIR}" -maxdepth 1 -type f -name '*_*.up.sql' \
    -printf '%f\n' |
    sed -n 's/^\([0-9][0-9]*\)_.*/\1/p' |
    sort -n |
    tail -1
)"

latest_db="$(
  "${PSQL[@]}" -Atc \
    "SELECT COALESCE(MAX(version), 0) FROM schema_migrations"
)"

latest_file_num=$((10#$latest_file))
latest_db_num=$((10#$latest_db))

if [[ "$latest_db_num" -ne "$latest_file_num" ]]; then
  echo "Migration state mismatch: database=${latest_db_num}, files=${latest_file_num}" >&2
  exit 1
fi

missing_version="$(
  "${PSQL[@]}" -Atc "
    SELECT COALESCE(
      MIN(v),
      0
    )
    FROM generate_series(1, ${latest_file_num}) AS v
    WHERE NOT EXISTS (
      SELECT 1
      FROM schema_migrations
      WHERE version = v
    );
  "
)"

missing_version_num=$((10#$missing_version))

if [[ "$missing_version_num" -ne 0 ]]; then
  echo "Migration ledger gap detected at version ${missing_version_num}" >&2
  exit 1
fi

echo "Migration complete. Schema version: ${latest_db_num}"
