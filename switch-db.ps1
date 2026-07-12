param(
    [ValidateSet("local", "aiven")]
    [string]$Target = "local",

    [string]$AivenUrl = ""
)

$ErrorActionPreference = "Stop"

$serverDir = Split-Path -Parent $MyInvocation.MyCommand.Path
Push-Location $serverDir
try {
    $envPath = Join-Path $serverDir ".env"
    $examplePath = Join-Path $serverDir ".env.example"

    if (-not (Test-Path $envPath)) {
        if (Test-Path $examplePath) {
            Copy-Item $examplePath $envPath -Force
        }
        else {
            throw "Missing .env and .env.example."
        }
    }

    $settings = @{}
    if (Test-Path $envPath) {
        Get-Content $envPath | ForEach-Object {
            if ([string]::IsNullOrWhiteSpace($_)) { return }
            if ($_ -match '^\s*#') { return }
            $parts = $_ -split '=', 2
            if ($parts.Count -eq 2) {
                $settings[$parts[0].Trim()] = $parts[1].Trim()
            }
        }
    }

    if ($Target -eq "local") {
        $settings["DATABASE_URL"] = "postgres://postgres:postgres@localhost:5432/gamegodot?sslmode=disable"
        $settings["PORT"] = "9000"
    }
    else {
        if ([string]::IsNullOrWhiteSpace($AivenUrl)) {
            $settings["DATABASE_URL"] = "postgres://avnadmin:YOUR_AIVEN_PASSWORD@YOUR_AIVEN_HOST:PORT/gamegodot?sslmode=require"
        }
        else {
            $settings["DATABASE_URL"] = $AivenUrl
        }
        $settings["PORT"] = "9000"
    }

    $lines = @()
    foreach ($key in $settings.Keys) {
        $lines += "$key=$($settings[$key])"
    }

    Set-Content -Path $envPath -Value ($lines -join [Environment]::NewLine)

    Write-Host "Switched server config to $Target mode."
    Write-Host "DATABASE_URL=$($settings["DATABASE_URL"])"
}
finally {
    Pop-Location
}
