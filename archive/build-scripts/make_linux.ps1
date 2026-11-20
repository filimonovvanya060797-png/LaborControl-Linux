$ErrorActionPreference = 'Stop'
$root = Resolve-Path (Join-Path $PSScriptRoot '..')
$src  = Join-Path $root 'SOURCE'
$out  = Join-Path $root 'LINUX'

if (-not (Get-Command go -ErrorAction SilentlyContinue)) { throw 'Go not found. Установи Go с go.dev/dl' }

New-Item -ItemType Directory -Force -Path $out | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $out 'shim') | Out-Null

$app  = Join-Path $src 'App'
$shim = Join-Path $src 'BrowserShim'
$core = $null

foreach ($f in (Get-ChildItem -Path $src -Recurse -Filter 'main.go')) {
    $t = [IO.File]::ReadAllText($f.FullName)
    if ($t -match 'go:embed index.html') { $core = $f.DirectoryName }
}

if (-not $core) { throw 'Core folder not found' }

$env:GOOS='linux'
$env:GOARCH='amd64'
$env:CGO_ENABLED='0'

Push-Location $app
go build -trimpath -ldflags '-s -w' -o (Join-Path $out 'LaborControl') .
Pop-Location

Push-Location $core
go build -trimpath -ldflags '-s -w' -o (Join-Path $out 'LaborControl_Core_R11') .
Pop-Location

Push-Location $shim
go build -trimpath -ldflags '-s -w' -o (Join-Path $out 'shim\xdg-open') .
Pop-Location

$netstat = @'
#!/bin/sh
ss -tlnp | awk 'NR>1 { pid=""; if (match($0,/pid=[0-9]+/)) pid=substr($0,RSTART+4,RLENGTH-4); if (pid!="") printf "  TCP    %s    0.0.0.0:0    LISTENING    %s\n", $4, pid }'
'@

$start = @'
#!/bin/sh
cd "$(dirname "$0")"
DIR=$(pwd)
export LOCALAPPDATA="$HOME/.local/share"
export PATH="$DIR/shim:$PATH"
./LaborControl &
sleep 3
xdg-open 'http://127.0.0.1:8779/?shell=timesheet-r11' >/dev/null
wait
'@

$utf8 = New-Object Text.UTF8Encoding($false)
[IO.File]::WriteAllText((Join-Path $out 'shim\netstat'), ($netstat -replace "`r`n", "`n"), $utf8)
[IO.File]::WriteAllText((Join-Path $out 'start.sh'), ($start -replace "`r`n", "`n"), $utf8)

Write-Host 'GOTOVO! Skopiruj папку LINUX в Linux.'