param(
    [string]$EnvFile = ".env"
)

$ErrorActionPreference = "Stop"

$serverDir = Split-Path -Parent $MyInvocation.MyCommand.Path
Push-Location $serverDir
try {
    $envPath = Join-Path $serverDir $EnvFile
    $examplePath = Join-Path $serverDir ".env.example"

    if (-not (Test-Path $envPath)) {
        if (Test-Path $examplePath) {
            Copy-Item $examplePath $envPath -Force
        }
        else {
            throw "Missing $EnvFile. Copy .env.example to .env and fill in the Aiven details first."
        }
    }

    $databaseUrl = $null
    Get-Content $EnvFile | ForEach-Object {
        if ([string]::IsNullOrWhiteSpace($_)) { return }
        if ($_ -match '^\s*#') { return }
        $parts = $_ -split '=', 2
        if ($parts.Count -eq 2 -and $parts[0].Trim() -eq "DATABASE_URL") {
            $databaseUrl = $parts[1].Trim()
        }
    }

    if ([string]::IsNullOrWhiteSpace($databaseUrl)) {
        throw "DATABASE_URL was not found in $EnvFile."
    }

    $probeCode = @'
package main

import (
    "database/sql"
    "fmt"
    "os"

    _ "github.com/lib/pq"
)

func main() {
    url := os.Getenv("DATABASE_URL")
    db, err := sql.Open("postgres", url)
    if err != nil {
        fmt.Println("OPEN_ERR:", err)
        os.Exit(1)
    }
    defer db.Close()

    if err := db.Ping(); err != nil {
        fmt.Println("PING_ERR:", err)
        os.Exit(1)
    }

    fmt.Println("PING_OK")
}
'@

    $probePath = Join-Path $serverDir "tmp_aiven_ping.go"
    Set-Content -Path $probePath -Value $probeCode

    $env:DATABASE_URL = $databaseUrl
    go run $probePath

    if ($LASTEXITCODE -ne 0) {
        throw "Aiven connection test failed."
    }

    Write-Host "Aiven connection probe succeeded."
}
finally {
    if (Test-Path (Join-Path $serverDir "tmp_aiven_ping.go")) {
        Remove-Item (Join-Path $serverDir "tmp_aiven_ping.go") -Force
    }
    Pop-Location
}
