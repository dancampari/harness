param(
  [switch]$Force
)

$ErrorActionPreference = "Stop"

$repo = Split-Path -Parent $PSScriptRoot
$npmPrefix = npm prefix -g
$shimDir = $npmPrefix.Trim()
$oldPackage = "@dancampari/agent-harness-kit"

Write-Host "Harness local dev link"
Write-Host "  Repo: $repo"

Push-Location $repo
try {
  $go = Get-Command go -ErrorAction SilentlyContinue
  if (-not $go) {
    $goPath = "C:\Program Files\Go\bin\go.exe"
    if (Test-Path $goPath) {
      $env:Path = "C:\Program Files\Go\bin;$env:Path"
    }
  }

  $version = node -p "require('./package.json').version"
  New-Item -ItemType Directory -Force -Path "dist" | Out-Null
  go build -ldflags "-X main.version=$version" -o ".\dist\harness.exe" .

  npm uninstall -g $oldPackage | Out-Host

  foreach ($name in @("harness", "harness.cmd", "harness.ps1")) {
    $path = Join-Path $shimDir $name
    if (-not (Test-Path $path)) {
      continue
    }
    $content = Get-Content -Raw -LiteralPath $path -ErrorAction SilentlyContinue
    if ($Force -or $content -like "*$oldPackage*") {
      Remove-Item -LiteralPath $path -Force
    }
  }

  npm link | Out-Host
  harness --version
  Write-Host "OK local harness command is linked to this checkout."
} finally {
  Pop-Location
}
