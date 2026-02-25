@echo off
setlocal

set SERVER=www.driverdevelop.com
set USER=root
if "%DEPLOY_PASS%"=="" set /p DEPLOY_PASS=Enter password:

set PASS=%DEPLOY_PASS%
set REMOTE_DIR=/data/soft/askflow
set BINARY_NAME=askflow
set SSHPASS=C:\Users\ma139\sshpass\sshpass

echo ============================================
echo  Askflow One-Click Deploy
echo  Target: %USER%@%SERVER%:%REMOTE_DIR%
echo ============================================
echo.

REM --- Step 1: Package ---
echo [1/4] Packaging project files...
if exist deploy.tar.gz del /f deploy.tar.gz
tar -czf deploy.tar.gz --exclude=deploy.tar.gz --exclude=build.cmd --exclude=start.sh --exclude=.git --exclude=.kiro --exclude=.vscode --exclude=*.exe *.go go.mod go.sum internal frontend sqlite-vec
if %errorlevel% neq 0 (
    echo [ERROR] Packaging failed!
    exit /b 1
)
echo        Package OK
echo.

REM --- Step 2: Cache host key ---
echo [PREP] Caching server host key...
echo y | %SSHPASS% -p %PASS% ssh -o StrictHostKeyChecking=accept-new %USER%@%SERVER% "echo connected" >nul 2>&1
echo        Done
echo.

REM --- Step 3: Upload package and start script ---
echo [2/4] Uploading to server...
%SSHPASS% -p %PASS% ssh -o StrictHostKeyChecking=accept-new %USER%@%SERVER% "mkdir -p %REMOTE_DIR%"
%SSHPASS% -p %PASS% scp -o StrictHostKeyChecking=accept-new -q deploy.tar.gz %USER%@%SERVER%:%REMOTE_DIR%/deploy.tar.gz
if %errorlevel% neq 0 (
    echo [ERROR] Upload failed!
    exit /b 1
)
%SSHPASS% -p %PASS% scp -o StrictHostKeyChecking=accept-new -q start.sh %USER%@%SERVER%:%REMOTE_DIR%/start.sh
%SSHPASS% -p %PASS% ssh -o StrictHostKeyChecking=accept-new %USER%@%SERVER% "sed -i 's/\r$//' %REMOTE_DIR%/start.sh && sed -i 's|__REMOTE_DIR__|%REMOTE_DIR%|g' %REMOTE_DIR%/start.sh"
echo        Upload OK
echo.

REM --- Step 4: Remote extract, build, and restart ---
echo [3/4] Building on remote server...
%SSHPASS% -p %PASS% ssh -o StrictHostKeyChecking=accept-new %USER%@%SERVER% "cd %REMOTE_DIR% && rm -rf *.go internal sqlite-vec && tar -xzf deploy.tar.gz && rm -f deploy.tar.gz && export GOPROXY=https://goproxy.cn,direct && go get -u github.com/VantageDataChat/GoPPT && go mod tidy && go clean -cache && go build -o %BINARY_NAME% . 2>&1"
if %errorlevel% neq 0 (
    echo [ERROR] Remote build failed!
    exit /b 1
)
echo        Build OK
echo.

echo [4/7] Configuring ffmpeg in config.json...
%SSHPASS% -p %PASS% ssh -o StrictHostKeyChecking=accept-new %USER%@%SERVER% "sed -i 's/\"ffmpeg_path\": \"[^\"]*\"/\"ffmpeg_path\": \"\/usr\/bin\/ffmpeg\"/' %REMOTE_DIR%/data/config.json 2>/dev/null && echo '  Config updated'"
echo.

echo [5/7] Restarting service...
%SSHPASS% -p %PASS% ssh -o StrictHostKeyChecking=accept-new %USER%@%SERVER% "chmod +x %REMOTE_DIR%/start.sh && bash %REMOTE_DIR%/start.sh"
echo.

echo [6/7] Packaging built artifacts on server...
set RELEASE_NAME=askflow_release_linux.tar.gz
%SSHPASS% -p %PASS% ssh -o StrictHostKeyChecking=accept-new %USER%@%SERVER% "cd %REMOTE_DIR% && tar -czf %RELEASE_NAME% %BINARY_NAME% frontend"
echo        Package OK
echo.

echo [7/7] Downloading release package to local...
if exist %RELEASE_NAME% del /f %RELEASE_NAME%
%SSHPASS% -p %PASS% scp -o StrictHostKeyChecking=accept-new -q %USER%@%SERVER%:%REMOTE_DIR%/%RELEASE_NAME% .
if %errorlevel% neq 0 (
    echo [ERROR] Download failed!
) else (
    echo        Download OK: %RELEASE_NAME%
)
echo.

REM --- Cleanup ---
del /f deploy.tar.gz 2>nul

echo ============================================
echo  Deploy complete!
echo  URL: http://%SERVER%
echo ============================================

endlocal
if %errorlevel% neq 0 pause
