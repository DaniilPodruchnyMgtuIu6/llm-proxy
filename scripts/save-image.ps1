#Requires -Version 5.1
# Сохранить образ в tar — удобно передать другу без сборки из исходников.
$ErrorActionPreference = "Stop"
Set-Location (Split-Path $PSScriptRoot -Parent)

if (-not (Test-Path ".env")) {
    Write-Host "Сначала нужен .env (хотя бы для сборки proxy не обязателен, но compose build проще с ним)." -ForegroundColor Yellow
}

Write-Host "Собираю llm-proxy:local ..." -ForegroundColor Cyan
docker compose build proxy
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$out = Join-Path (Get-Location) "llm-proxy-image.tar"
docker save llm-proxy:local -o $out
Write-Host "Образ сохранён: $out" -ForegroundColor Green
Write-Host "Другу: docker load -i llm-proxy-image.tar"
Write-Host "Потом всё равно нужны docker-compose.yml + .env и: docker compose up -d"
