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
    Write-Host ""
    Write-Host "Создан файл .env из .env.example" -ForegroundColor Yellow
    Write-Host "Открой .env и вставь хотя бы один ключ:" -ForegroundColor Yellow
    Write-Host "  GEMINI_API_KEY / GROQ_API_KEY / OPENROUTER_API_KEY"
    Write-Host "Потом снова запусти: .\scripts\start.ps1"
    Write-Host ""
    exit 1
}

$envText = Get-Content ".env" -Raw
$hasKey = $false
foreach ($name in @("GEMINI_API_KEY", "GROQ_API_KEY", "OPENROUTER_API_KEY")) {
    if ($envText -match "(?m)^\s*$name\s*=\s*\S+") {
        $hasKey = $true
        break
    }
}
if (-not $hasKey) {
    Write-Host "В .env нет ключей провайдеров. Заполни хотя бы один и перезапусти." -ForegroundColor Red
    exit 1
}

Write-Host "Собираю и поднимаю llm-proxy (proxy + postgres + pgAdmin)..." -ForegroundColor Cyan
docker compose up -d --build
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$proxyPort = "8080"
$pgPort = "5050"
if ($envText -match "(?m)^\s*PROXY_PORT\s*=\s*(\d+)") { $proxyPort = $Matches[1] }
if ($envText -match "(?m)^\s*PGADMIN_PORT\s*=\s*(\d+)") { $pgPort = $Matches[1] }

Write-Host ""
Write-Host "Готово." -ForegroundColor Green
Write-Host "  Proxy:   http://localhost:$proxyPort"
Write-Host "  Docs:    http://localhost:$proxyPort/docs"
Write-Host "  pgAdmin: http://localhost:$pgPort  (admin@example.com / admin)"
Write-Host "  В pgAdmin Host БД = localhost (порт из DB_PORT), user/pass/db = llmproxy"
Write-Host ""
Write-Host "Логи: docker compose logs -f proxy"
Write-Host "Стоп:  .\scripts\stop.ps1"
