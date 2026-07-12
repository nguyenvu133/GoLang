param(
    [switch]$SkipDocker
)

$ErrorActionPreference = "Stop"

$serverDir = Split-Path -Parent $MyInvocation.MyCommand.Path
Push-Location $serverDir
try {
    if (-not $SkipDocker) {
        $docker = Get-Command docker -ErrorAction SilentlyContinue
        if (-not $docker) {
            throw "docker not found in PATH. Install Docker Desktop or Docker Engine first."
        }

        Write-Host "[1/2] Starting local PostgreSQL container..."
        docker compose up -d postgres
        if ($LASTEXITCODE -ne 0) {
            throw "docker compose up failed."
        }

        Write-Host "[2/2] Waiting for PostgreSQL to accept connections..."
        $ready = $false
        while (-not $ready) {
            docker compose exec -T postgres pg_isready -U postgres -d gamegodot 2>$null | Out-Null
            if ($LASTEXITCODE -eq 0) {
                $ready = $true
            }
            else {
                Start-Sleep -Seconds 1
            }
        }
    }

    Write-Host "Ensuring local DB .env is selected..."
    & .\switch-db.ps1 -Target local

    Write-Host "Starting Go server..."
    go run .
}
finally {
    Pop-Location
}
