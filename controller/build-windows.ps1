$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = Split-Path -Parent $scriptDir
$output = Join-Path $repoRoot "服务端控制器.exe"

Push-Location $scriptDir
$resource = $null
try {
    $arch = (go env GOARCH).Trim()
    if ($LASTEXITCODE -ne 0) { throw "Cannot determine Go architecture" }
    $resourcePath = Join-Path $scriptDir "controller_icon_windows_$arch.syso"
    if (Test-Path $resourcePath) { throw "Resource already exists: $resourcePath" }
    $resource = $resourcePath
    go run github.com/akavel/rsrc@v0.10.2 -arch $arch -ico assets/controller.ico -o $resource
    if ($LASTEXITCODE -ne 0) { throw "Icon resource generation failed" }
    go build -trimpath -ldflags "-s -w -H=windowsgui -X main.builtRepoRoot=$repoRoot" -o $output .
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    Write-Output $output
} finally {
    if ($resource -and (Test-Path $resource)) { Remove-Item $resource }
    Pop-Location
}
