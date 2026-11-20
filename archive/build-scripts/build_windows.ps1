$ErrorActionPreference = 'Stop'
$root = Join-Path $PSScriptRoot '..'
Set-Location (Join-Path $root 'SOURCE\App')
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
$env:CGO_ENABLED = '0'
go test ./...
go vet ./...
go build -trimpath -ldflags '-s -w -H=windowsgui' -o (Join-Path $root 'LaborControl.exe') .
Set-Location (Join-Path $root 'SOURCE\BrowserShim')
New-Item -ItemType Directory -Force -Path (Join-Path $root 'shim') | Out-Null
go build -trimpath -ldflags '-s -w -H=windowsgui' -o (Join-Path $root 'shim\rundll32.exe') .
Write-Host 'LaborControl.exe and browser shim built.'