#Requires -Version 5.1
$ErrorActionPreference = "Stop"
Set-Location (Split-Path $PSScriptRoot -Parent)
docker compose down
Write-Host "Остановлено. Данные Postgres сохранены в volume." -ForegroundColor Green
Write-Host "Полный сброс БД: docker compose down -v"
