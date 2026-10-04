#Requires -Version 5.1
$ErrorActionPreference = "Stop"
Set-Location (Split-Path $PSScriptRoot -Parent)

function Test-Docker {
    try {
        docker info 2>$null | Out-Null
        return $LASTEXITCODE -eq 0
    } catch {
        return $false
    }
}

if (-not (Test-Docker)) {
    Write-Host "Docker не запущен. Установи/открой Docker Desktop и повтори." -ForegroundColor Red
    exit 1
}

if (-not (Test-Path ".env")) {
    Copy-Item ".env.example" ".env"
    Write-Host "Создан .env из .env.example (ключи можно ввести в UI)." -ForegroundColor Yellow
}

$envText = Get-Content ".env" -Raw

Write-Host "Собираю и поднимаю llm-proxy (web UI + proxy + postgres + pgAdmin)..." -ForegroundColor Cyan
docker compose up -d --build
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$proxyPort = "8080"
$uiPort = "3000"
$pgPort = "5050"
if ($envText -match "(?m)^\s*PROXY_PORT\s*=\s*(\d+)") { $proxyPort = $Matches[1] }
if ($envText -match "(?m)^\s*UI_PORT\s*=\s*(\d+)") { $uiPort = $Matches[1] }
if ($envText -match "(?m)^\s*PGADMIN_PORT\s*=\s*(\d+)") { $pgPort = $Matches[1] }

Write-Host ""
Write-Host "Готово." -ForegroundColor Green
Write-Host "  UI:      http://localhost:$uiPort"
Write-Host "  Proxy:   http://localhost:$proxyPort"
Write-Host "  Docs:    http://localhost:$uiPort/docs"
Write-Host "  pgAdmin: http://localhost:$pgPort  (admin@example.com / admin)"
Write-Host ""
Write-Host "Ключи провайдеров: открой UI → Setup/Ключи → Применить (без docker compose up)."
Write-Host "Логи: docker compose logs -f proxy web"
Write-Host "Стоп:  .\scripts\stop.ps1"
