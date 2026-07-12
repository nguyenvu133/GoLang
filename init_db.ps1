param(
    [string]$Host = "localhost",
    [string]$Port = "5432",
    [string]$AdminUser = "postgres",
    [string]$AdminPassword = "postgres",
    [string]$DbName = "gamegodot"
)

$ErrorActionPreference = "Stop"
$env:PGPASSWORD = $AdminPassword

$psqlCommand = Get-Command psql -ErrorAction SilentlyContinue
if (-not $psqlCommand) {
    throw "psql not found in PATH. Add PostgreSQL bin folder to PATH first."
}

$roleSql = @"
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'postgres') THEN
        CREATE ROLE postgres LOGIN SUPERUSER PASSWORD 'postgres';
    ELSE
        ALTER ROLE postgres WITH LOGIN SUPERUSER PASSWORD 'postgres';
    END IF;
END $$;
"@

& psql -h $Host -p $Port -U $AdminUser -d postgres -v ON_ERROR_STOP=1 -c $roleSql

$createDbSql = @"
SELECT 'CREATE DATABASE gamegodot OWNER postgres'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'gamegodot')\gexec
"@

& psql -h $Host -p $Port -U $AdminUser -d postgres -v ON_ERROR_STOP=1 -c $createDbSql

& psql -h $Host -p $Port -U $AdminUser -d $DbName -v ON_ERROR_STOP=1 -f .\schema.sql

Write-Host "Database '$DbName' is ready."
Write-Host "Connection string: postgres://postgres:postgres@$Host:$Port/$DbName?sslmode=disable"
