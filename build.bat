@echo off
setlocal enabledelayedexpansion
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

REM Read version from VERSION file
set VERSION=1.0.0
if exist "VERSION" (
    for /f "usebackq tokens=*" %%v in ("VERSION") do set VERSION=%%v
    set VERSION=!VERSION: =!
)
echo Building version: !VERSION!

REM Build executable name with version
set EXE_NAME=UE-Git-Plugin-Manager-v!VERSION!.exe
set EXE_PATH=dist\!EXE_NAME!

REM Check if the versioned executable is running and stop it
tasklist /FI "IMAGENAME eq !EXE_NAME!" 2>NUL | find /I /N "!EXE_NAME!">NUL
if "%ERRORLEVEL%"=="0" (
    echo !EXE_NAME! is currently running. Stopping it...
    taskkill /F /IM !EXE_NAME! >NUL 2>&1
    timeout /t 2 >NUL
)

REM Remove any existing executable and temporary files
if exist "!EXE_PATH!" del /F /Q "!EXE_PATH!" >NUL 2>&1
if exist "!EXE_PATH!~" del /F /Q "!EXE_PATH!~" >NUL 2>&1

REM Build the executable
echo Building executable...
go build -o "!EXE_PATH!" .

if %ERRORLEVEL% EQU 0 (
    echo.
    echo Build successful!
    echo !EXE_NAME! created in dist/ folder.
    echo.
    echo You can now run the application by double-clicking dist\!EXE_NAME!
    
    REM Clean up any temporary files that might have been created
    if exist "!EXE_PATH!~" (
        echo Cleaning up temporary files...
        del /F /Q "!EXE_PATH!~" >NUL 2>&1
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