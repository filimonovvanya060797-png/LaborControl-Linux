@echo off
setlocal
cd /d "%~dp0..\SOURCE\App"
where go >nul 2>nul || (echo Go not found & exit /b 1)
go test ./...