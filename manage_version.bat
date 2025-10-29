@echo off
echo UE Git Plugin Manager - Version Management
echo.

REM Check if VERSION file exists
if not exist "VERSION" (
    echo VERSION file not found! Creating with default version...
    echo 1.0.1 > VERSION
    echo Created VERSION file with version 1.0.1
    echo.
)

REM Read current version
set /p CURRENT_VERSION=<VERSION
echo Current version: %CURRENT_VERSION%

echo.
echo Options:
echo 1. Show current version
echo 2. Set patch version (e.g., 1.0.2)
echo 3. Set minor version (e.g., 1.1.0)
echo 4. Set major version (e.g., 2.0.0)
echo 5. Set custom version
echo 6. Exit
echo.

set /p choice="Enter your choice (1-6): "

if "%choice%"=="1" goto show_version
if "%choice%"=="2" goto set_patch
if "%choice%"=="3" goto set_minor
if "%choice%"=="4" goto set_major
if "%choice%"=="5" goto set_custom
if "%choice%"=="6" goto exit
goto invalid_choice

:show_version
echo Current version: %CURRENT_VERSION%
goto end

:set_patch
for /f "tokens=1,2,3 delims=." %%a in ("%CURRENT_VERSION%") do (
    set /a NEW_PATCH=%%c+1
    set NEW_VERSION=%%a.%%b.!NEW_PATCH!
)
echo %NEW_VERSION% > VERSION
echo Version set to: !NEW_VERSION!
goto end

:set_minor
for /f "tokens=1,2,3 delims=." %%a in ("%CURRENT_VERSION%") do (
    set /a NEW_MINOR=%%b+1
    set NEW_VERSION=%%a.!NEW_MINOR!.0
)
echo %NEW_VERSION% > VERSION
echo Version set to: !NEW_VERSION!
goto end

:set_major
for /f "tokens=1,2,3 delims=." %%a in ("%CURRENT_VERSION%") do (
    set /a NEW_MAJOR=%%a+1
    set NEW_VERSION=!NEW_MAJOR!.0.0
)
echo %NEW_VERSION% > VERSION
echo Version set to: !NEW_VERSION!
goto end

:set_custom
set /p NEW_VERSION="Enter new version (e.g., 1.2.3): "
echo %NEW_VERSION% > VERSION
echo Version set to: %NEW_VERSION%
goto end

:invalid_choice
echo Invalid choice. Please enter 1-6.
goto end

:end
echo.
echo Next steps:
echo 1. Commit this change: git add VERSION
echo 2. Commit: git commit -m "Prepare release v%NEW_VERSION%"
echo 3. Push: git push origin main
echo 4. GitHub Action will automatically create the release
echo.
pause
goto exit

:exit
