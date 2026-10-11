# Ensure latest sing-box core and companion libraries in bin/
$ErrorActionPreference = 'Stop'
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$rootDir = Split-Path -Parent $scriptDir
$binDir = Join-Path $rootDir "bin"
$coreExe = Join-Path $binDir "sing-box.exe"
$cronetDll = Join-Path $binDir "libcronet.dll"

if (-not (Test-Path $binDir)) {
    New-Item -ItemType Directory -Path $binDir -Force | Out-Null
}

$localVersion = ""
if (Test-Path $coreExe) {
    try {
        $verOut = & $coreExe version 2>$null
        $verStr = ($verOut -join "`n")
        if ($verStr -match 'sing-box version (\d+\.\d+\.\d+)') {
            $localVersion = $matches[1]
        }
    } catch {}
}

$localStatus = "not found"
if ($localVersion -ne "") {
    $localStatus = "v" + $localVersion
}
Write-Host ("[Core Check] Local sing-box version: " + $localStatus)

$needDownload = $false
$latestTag = ""
$assetUrl = ""

try {
    $req = [System.Net.HttpWebRequest]::Create("https://api.github.com/repos/SagerNet/sing-box/releases/latest")
    $req.UserAgent = "go-singbox-builder"
    $req.Timeout = 6000
    $resp = $req.GetResponse()
    $stream = $resp.GetResponseStream()
    $reader = New-Object System.IO.StreamReader($stream)
    $jsonText = $reader.ReadToEnd()
    $reader.Close()
    $resp.Close()

    $release = $jsonText | ConvertFrom-Json
    $latestTag = [string]$release.tag_name
    $cleanTag = $latestTag.TrimStart('v')

    foreach ($asset in $release.assets) {
        if ($asset.name -match 'windows-amd64\.zip$') {
            $assetUrl = [string]$asset.browser_download_url
            break
        }
    }

    if ($cleanTag -ne "" -and ($cleanTag -ne $localVersion -or -not (Test-Path $cronetDll))) {
        Write-Host ("[Core Check] Found latest release: " + $latestTag + ", downloading...")
        $needDownload = $true
    } else {
        Write-Host ("[Core Check] Core is already latest (" + $latestTag + ") with required companion DLLs.")
    }
} catch {
    Write-Host ("[Core Check] GitHub API unavailable or rate-limited: " + $_.Exception.Message)
    if (Test-Path $coreExe) {
        Write-Host ("[Core Check] Proceeding with local core (" + $localStatus + ").")
    } else {
        Write-Error "[Core Check] Local sing-box.exe not found and cannot download!"
        exit 1
    }
}

if ($needDownload -and $assetUrl -ne "") {
    $tempZip = Join-Path $env:TEMP ("sing-box-" + [System.Guid]::NewGuid().ToString() + ".zip")
    $tempExtract = Join-Path $env:TEMP ("sing-box-extract-" + [System.Guid]::NewGuid().ToString())
    try {
        Write-Host ("[Core Check] Downloading: " + $assetUrl)
        $wc = New-Object System.Net.WebClient
        $wc.Headers.Add("User-Agent", "go-singbox-builder")
        $wc.DownloadFile($assetUrl, $tempZip)
        Write-Host "[Core Check] Download complete. Extracting..."

        New-Item -ItemType Directory -Path $tempExtract -Force | Out-Null
        Expand-Archive -Path $tempZip -DestinationPath $tempExtract -Force

        $foundExe = Get-ChildItem -Path $tempExtract -Filter "sing-box.exe" -Recurse | Select-Object -First 1
        if ($foundExe) {
            Copy-Item -Path $foundExe.FullName -Destination $coreExe -Force
            Write-Host ("[Core Check] Updated sing-box.exe -> " + $coreExe)
        } else {
            throw "sing-box.exe not found in extracted archive"
        }

        $dlls = Get-ChildItem -Path $tempExtract -Filter "*.dll" -Recurse
        foreach ($dll in $dlls) {
            $destDll = Join-Path $binDir $dll.Name
            Copy-Item -Path $dll.FullName -Destination $destDll -Force
            Write-Host ("[Core Check] Updated " + $dll.Name + " -> " + $destDll)
        }

        Write-Host ("[Core Check] Successfully updated sing-box to " + $latestTag + "!")
    } catch {
        Write-Host ("[Core Check] Download failed: " + $_.Exception.Message)
        if (-not (Test-Path $coreExe)) {
            Write-Error "[Core Check] Missing required core binary, cannot build!"
            exit 1
        }
        Write-Host "[Core Check] Falling back to local core binary."
    } finally {
        if (Test-Path $tempZip) { Remove-Item -Force $tempZip -ErrorAction SilentlyContinue }
        if (Test-Path $tempExtract) { Remove-Item -Force -Recurse $tempExtract -ErrorAction SilentlyContinue }
    }
}
