#!/bin/bash
set -e

# Create additional databases
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    CREATE DATABASE project_db;
    CREATE USER project_user WITH PASSWORD 'project_pass';
    GRANT ALL PRIVILEGES ON DATABASE project_db TO project_user;
EOSQL
