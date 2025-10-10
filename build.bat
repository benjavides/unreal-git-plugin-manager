@echo off
echo Building UE Git Plugin Manager...

REM Check if Go is installed
go version >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo.
    echo Go is not installed or not in PATH!
    echo.
    echo Please install Go 1.21 or later from: https://golang.org/dl/
    echo After installation, restart Command Prompt and try again.
    echo.
    pause
    exit /b 1
)

REM Check Go version
for /f "tokens=3" %%i in ('go version') do set GO_VERSION=%%i
echo Go version: %GO_VERSION%

REM Create logs directory if it doesn't exist
if not exist "logs" mkdir logs

REM Initialize Go module if needed
if not exist "go.mod" (
    echo Initializing Go module...
    go mod init ue-git-plugin-manager
)

REM Download dependencies
echo Downloading dependencies...
go mod tidy

REM Create dist directory if it doesn't exist
if not exist "dist" mkdir dist

REM Check if UE-Git-Plugin-Manager.exe is running and stop it
tasklist /FI "IMAGENAME eq UE-Git-Plugin-Manager.exe" 2>NUL | find /I /N "UE-Git-Plugin-Manager.exe">NUL
if "%ERRORLEVEL%"=="0" (
    echo ⚠️  UE-Git-Plugin-Manager.exe is currently running. Stopping it...
    taskkill /F /IM UE-Git-Plugin-Manager.exe >NUL 2>&1
    timeout /t 2 >NUL
)

REM Remove any existing executable and temporary files
if exist "dist\UE-Git-Plugin-Manager.exe" del /F /Q "dist\UE-Git-Plugin-Manager.exe" >NUL 2>&1
if exist "dist\UE-Git-Plugin-Manager.exe~" del /F /Q "dist\UE-Git-Plugin-Manager.exe~" >NUL 2>&1

REM Build the executable
echo Building executable...
go build -o dist/UE-Git-Plugin-Manager.exe .

if %ERRORLEVEL% EQU 0 (
    echo.
    echo Build successful!
    echo UE-Git-Plugin-Manager.exe created in dist/ folder.
    echo.
    echo You can now run the application by double-clicking dist/UE-Git-Plugin-Manager.exe
    
    REM Clean up any temporary files that might have been created
    if exist "dist\UE-Git-Plugin-Manager.exe~" (
        echo Cleaning up temporary files...
        del /F /Q "dist\UE-Git-Plugin-Manager.exe~" >NUL 2>&1
    )
) else (
    echo.
    echo Build failed!
    echo Please check the error messages above.
    echo.
    echo Common issues:
    echo - Go not installed or not in PATH
    echo - Git not installed [required for dependencies]
    echo - Network connectivity issues
    echo - UE-Git-Manager.exe is locked by another process
    pause
)