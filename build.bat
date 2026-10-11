@echo off
setlocal EnableExtensions EnableDelayedExpansion
cd /d "%~dp0"

if "%GO_EXE%"=="" set "GO_EXE=go"
if "%WAILS_EXE%"=="" set "WAILS_EXE=wails"
if exist "%LOCALAPPDATA%\OpenAI\Codex\bin\node.exe" set "PATH=%LOCALAPPDATA%\OpenAI\Codex\bin;%PATH%"

"%GO_EXE%" version >nul 2>nul
if errorlevel 1 (
  echo [ERROR] Go not found, please install Go or set GO_EXE.
  exit /b 1
)

"%WAILS_EXE%" version >nul 2>nul
if errorlevel 1 (
  echo [ERROR] Wails not found, please install Wails or set WAILS_EXE.
  exit /b 1
)

powershell -NoProfile -ExecutionPolicy Bypass -File "scripts\ensure-latest-core.ps1"
if errorlevel 1 (
  echo [ERROR] Failed to ensure latest sing-box core!
  exit /b 1
)

if not exist "data" mkdir "data"
set "VERSION_FILE=data\build-version.json"
for /f "usebackq delims=" %%v in (`powershell -NoProfile -Command "$p='%VERSION_FILE%'; if (Test-Path $p) { $j=Get-Content $p -Raw | ConvertFrom-Json; $a=([string]$j.version).Split('.'); '{0}.{1}.{2}' -f $a[0],$a[1],([int]$a[2]+1) } else { '0.0.1' }"`) do set "VERSION=%%v"
powershell -NoProfile -Command "$p='%VERSION_FILE%'; @{version='%VERSION%'} | ConvertTo-Json | Set-Content -Encoding UTF8 $p"

"%WAILS_EXE%" build -clean -ldflags "-s -w -X main.AppVersion=%VERSION%"
if errorlevel 1 exit /b 1

if not exist "build\bin\data" mkdir "build\bin\data"
if not exist "build\bin\logs" mkdir "build\bin\logs"
if exist "bin" xcopy /e /i /y "bin" "build\bin\bin\" >nul
del /q "build\bin\bin\*.bak" >nul 2>nul
del /q "build\bin\bin\v2rayn_xray.exe" >nul 2>nul
if not exist "build\bin\bin\sing-box.exe" (
  echo [ERROR] build\bin\bin\sing-box.exe not found!
  exit /b 1
)
if exist "data\gui\rules" xcopy /e /i /y "data\gui\rules" "build\bin\data\gui\rules\" >nul
del /q "build\bin\data\gui\rules\loyalsoldier-reject.json" >nul 2>nul
del /q "build\bin\data\state.dat" >nul 2>nul
del /q "build\bin\data\gui\state.dat" >nul 2>nul
del /q "build\bin\data\sing-box.json" >nul 2>nul
del /q "build\bin\data\gui\sing-box.json" >nul 2>nul
del /q "build\bin\data\gui\cache.db" >nul 2>nul
del /q "build\bin\logs\*.log" >nul 2>nul
del /q "build\bin\logs\app.log" >nul 2>nul
del /q "build\bin\logs\gui\*.log" >nul 2>nul
del /q "build\bin\logs\web\*.log" >nul 2>nul
del /q "build\bin\start.bat" >nul 2>nul

set "OUT=dist\SanTian-Singbox-v%VERSION%"
if exist "%OUT%" rmdir /s /q "%OUT%"
mkdir "%OUT%"
xcopy /e /i /y "build\bin\*" "%OUT%\" >nul
python -c "import os, zipfile, sys; src, out = sys.argv[1], sys.argv[2]; (os.remove(out) if os.path.exists(out) else None); z = zipfile.ZipFile(out, 'w', zipfile.ZIP_DEFLATED, 6); [(z.write(os.path.join(r, f), os.path.relpath(os.path.join(r, f), src).replace('\\\\', '/'))) for r, _, fs in os.walk(src) for f in fs]; z.close()" "%OUT%" "dist\SanTian-Singbox-v%VERSION%-windows-amd64.zip"
echo Build completed: %OUT%
echo Archive created: dist\SanTian-Singbox-v%VERSION%-windows-amd64.zip
