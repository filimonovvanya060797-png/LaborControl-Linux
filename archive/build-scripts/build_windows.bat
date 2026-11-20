@echo off
setlocal
set ROOT=%~dp0..
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0
cd /d "%ROOT%SOURCE\App"
where go >nul 2>nul || (echo Go not found & exit /b 1)
go test ./... || exit /b 1
go vet ./... || exit /b 1
go build -trimpath -ldflags="-s -w -H=windowsgui" -o "%ROOT%LaborControl.exe" . || exit /b 1
if not exist "%ROOT%shim" mkdir "%ROOT%shim"
cd /d "%ROOT%SOURCE\BrowserShim"
go build -trimpath -ldflags="-s -w -H=windowsgui" -o "%ROOT%shim\rundll32.exe" . || exit /b 1
echo Built: %ROOT%LaborControl.exe