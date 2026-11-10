# jedkx-cli installer for Windows — per-user, no admin.
#   irm https://raw.githubusercontent.com/jedkx/jedkx-cli/main/scripts/install.ps1 | iex
# Downloads the latest release, verifies its SHA-256, runs `jedkx-cli install`,
# and deletes everything it downloaded.

$ErrorActionPreference = 'Stop'
$repo = 'jedkx/jedkx-cli'

if (-not [Environment]::Is64BitOperatingSystem) { throw 'jedkx-cli needs 64-bit Windows.' }
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$asset = "jedkx-cli_windows_$arch.exe"

$headers = @{ 'User-Agent' = 'jedkx-cli-installer' }
$rel = Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest" -Headers $headers
$bin = $rel.assets | Where-Object name -eq $asset
$sum = $rel.assets | Where-Object name -eq 'checksums.txt'
if (-not $bin -or -not $sum) { throw "Release $($rel.tag_name) has no $asset." }

$tmp = Join-Path ([IO.Path]::GetTempPath()) ("jedkx-cli-" + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
    $exe = Join-Path $tmp 'jedkx-cli.exe'
    Write-Host "  jedkx-cli $($rel.tag_name) — downloading…"
    Invoke-WebRequest $bin.browser_download_url -OutFile $exe -Headers $headers -UseBasicParsing
    $want = ((Invoke-RestMethod $sum.browser_download_url -Headers $headers) -split "`n" |
        Where-Object { $_ -match "\s$([regex]::Escape($asset))$" }) -split '\s+' | Select-Object -First 1
    $got = (Get-FileHash $exe -Algorithm SHA256).Hash
    if (-not $want -or $got -ne $want.ToUpper()) { throw 'Checksum mismatch — nothing was installed.' }
    & $exe install
    if ($LASTEXITCODE -ne 0) { throw 'Install failed.' }
}
finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
