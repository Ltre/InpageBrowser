@echo off
setlocal EnableExtensions

rem InpageBrowser Windows development launcher.
rem Proxy, runtime and listener variables are process-local to this window.

set "EXIT_CODE=1"
set "PUSHD_OK=0"

rem Proxy defaults can be overridden before launching this script.
if not defined INPAGE_HTTP_PROXY set "INPAGE_HTTP_PROXY=http://127.0.0.1:58591"
if not defined INPAGE_HTTPS_PROXY set "INPAGE_HTTPS_PROXY=http://127.0.0.1:58591"
if not defined INPAGE_ALL_PROXY set "INPAGE_ALL_PROXY=socks5://127.0.0.1:51837"
if not defined INPAGE_NO_PROXY set "INPAGE_NO_PROXY=127.0.0.1,localhost,::1,host.docker.internal"
rem Chromium runs inside Docker, so it cannot use host 127.0.0.1 directly.
if not defined INPAGE_BROWSER_PROXY set "INPAGE_BROWSER_PROXY=socks5://host.docker.internal:51837"

set "HTTP_PROXY=%INPAGE_HTTP_PROXY%"
set "HTTPS_PROXY=%INPAGE_HTTPS_PROXY%"
set "ALL_PROXY=%INPAGE_ALL_PROXY%"
set "http_proxy=%HTTP_PROXY%"
set "https_proxy=%HTTPS_PROXY%"
set "all_proxy=%ALL_PROXY%"
set "NO_PROXY=%INPAGE_NO_PROXY%"
set "no_proxy=%NO_PROXY%"

set "SCRIPT_DIR=%~dp0"
for %%I in ("%SCRIPT_DIR%..") do set "REPO_ROOT=%%~fI"
if not exist "%REPO_ROOT%\go.mod" goto :repo_failed

pushd "%REPO_ROOT%" >nul
if errorlevel 1 goto :pushd_failed
set "PUSHD_OK=1"

if not defined INPAGE_ADDR set "INPAGE_ADDR=0.0.0.0:4002"
if not defined INPAGE_DATA_DIR set "INPAGE_DATA_DIR=%REPO_ROOT%\data"
if not defined INPAGE_BROWSER_IMAGE set "INPAGE_BROWSER_IMAGE=kasmweb/chromium:1.18.0"
if not defined INPAGE_MAX_ACTIVE set "INPAGE_MAX_ACTIVE=1"
if not defined INPAGE_IDLE_MINUTES set "INPAGE_IDLE_MINUTES=10"

if not exist "%INPAGE_DATA_DIR%" mkdir "%INPAGE_DATA_DIR%" >nul 2>&1
if not exist "%INPAGE_DATA_DIR%\profiles" mkdir "%INPAGE_DATA_DIR%\profiles" >nul 2>&1
if not exist "%REPO_ROOT%\bin" mkdir "%REPO_ROOT%\bin" >nul 2>&1

where go >nul 2>&1
if errorlevel 1 goto :go_missing

where docker >nul 2>&1
if errorlevel 1 goto :docker_missing

docker info >nul 2>&1
if errorlevel 1 (
    call :ensure_docker_daemon
    if errorlevel 1 goto :docker_unavailable
)

echo [InpageBrowser] Repository: %REPO_ROOT%
echo [InpageBrowser] HTTP proxy: %HTTP_PROXY%
echo [InpageBrowser] HTTPS proxy: %HTTPS_PROXY%
echo [InpageBrowser] SOCKS proxy: %ALL_PROXY%
echo [InpageBrowser] Chromium proxy: %INPAGE_BROWSER_PROXY%
echo [InpageBrowser] Listener: %INPAGE_ADDR%
echo [InpageBrowser] Local URL: http://localhost:4002/
echo [InpageBrowser] Data directory: %INPAGE_DATA_DIR%
echo [InpageBrowser] Browser image: %INPAGE_BROWSER_IMAGE%
echo [InpageBrowser] Max active browser sessions: %INPAGE_MAX_ACTIVE%
echo [InpageBrowser] Idle recycle: %INPAGE_IDLE_MINUTES% minute(s)
echo.
echo [InpageBrowser] Note: Passkey on another device requires a secure HTTPS origin.
echo [InpageBrowser] LAN HTTP can reach the page, but WebAuthn may be blocked by the browser.
echo.

docker image inspect "%INPAGE_BROWSER_IMAGE%" >nul 2>&1
if errorlevel 1 (
    echo [InpageBrowser] Kasm Chromium image is missing; pulling it now...
    docker pull "%INPAGE_BROWSER_IMAGE%"
    if errorlevel 1 goto :image_pull_failed
) else (
    echo [InpageBrowser] Kasm Chromium image is ready.
)

echo [InpageBrowser] Downloading Go module dependencies...
go mod download
if errorlevel 1 goto :go_download_failed

echo [InpageBrowser] Verifying Go module dependencies...
go mod verify
if errorlevel 1 goto :go_verify_failed

set "DEV_EXE=%REPO_ROOT%\bin\inpagebrowser-dev.exe"
echo [InpageBrowser] Building %DEV_EXE% ...
go build -o "%DEV_EXE%" ./cmd/inpagebrowser
if errorlevel 1 goto :build_failed

echo.
echo [InpageBrowser] Starting server...
"%DEV_EXE%"
set "EXIT_CODE=%ERRORLEVEL%"
if "%EXIT_CODE%"=="0" goto :success

echo.
echo [InpageBrowser] Server exited with code %EXIT_CODE%.
goto :fail

:ensure_docker_daemon
echo [InpageBrowser] Docker CLI exists but the daemon is not ready.
set "DOCKER_DESKTOP_EXE=%ProgramFiles%\Docker\Docker\Docker Desktop.exe"
if not exist "%DOCKER_DESKTOP_EXE%" exit /b 1

echo [InpageBrowser] Starting Docker Desktop...
start "" "%DOCKER_DESKTOP_EXE%"
echo [InpageBrowser] Waiting for Docker Desktop engine...
for /L %%N in (1,1,45) do (
    docker info >nul 2>&1
    if not errorlevel 1 exit /b 0
    timeout /t 2 /nobreak >nul
)
exit /b 1

:repo_failed
echo [InpageBrowser] Cannot resolve repository root from: %SCRIPT_DIR%
echo [InpageBrowser] Missing expected file: %REPO_ROOT%\go.mod
goto :fail

:pushd_failed
echo [InpageBrowser] Cannot enter repository root: %REPO_ROOT%
goto :fail

:go_missing
echo [InpageBrowser] Go was not found in PATH.
echo [InpageBrowser] Install Go 1.23+ and open a new terminal.
goto :fail

:docker_missing
echo [InpageBrowser] Docker CLI was not found in PATH.
echo [InpageBrowser] Windows development requires Docker Desktop with the WSL2/Linux container engine.
goto :fail

:docker_unavailable
echo [InpageBrowser] Docker Desktop could not become ready automatically.
echo [InpageBrowser] Start Docker Desktop and make sure it is using Linux containers, then run this script again.
goto :fail

:image_pull_failed
echo.
echo [InpageBrowser] Failed to pull %INPAGE_BROWSER_IMAGE%.
echo [InpageBrowser] The script already exported HTTP_PROXY/HTTPS_PROXY/ALL_PROXY for this process.
echo [InpageBrowser] If Docker Desktop still cannot pull, check whether Docker Desktop is using the Windows/system proxy.
goto :fail

:go_download_failed
echo.
echo [InpageBrowser] go mod download failed.
echo [InpageBrowser] Check that the local proxy is running, or override INPAGE_HTTP_PROXY/INPAGE_HTTPS_PROXY/INPAGE_ALL_PROXY.
goto :fail

:go_verify_failed
echo.
echo [InpageBrowser] go mod verify failed.
goto :fail

:build_failed
echo.
echo [InpageBrowser] Go build failed.
goto :fail

:fail
if "%PUSHD_OK%"=="1" popd >nul
echo.
echo [InpageBrowser] Startup failed. Press any key to close this window.
pause >nul
endlocal & exit /b %EXIT_CODE%

:success
if "%PUSHD_OK%"=="1" popd >nul
endlocal & exit /b 0
