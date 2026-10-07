#!/bin/sh
set -eu
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" --set=app_password="$TF_DB_APP_PASSWORD" <<'SQL'
CREATE ROLE tenantforge_app LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS PASSWORD :'app_password';
CREATE DATABASE tenantforge_test OWNER tenantforge_owner;
SQL
