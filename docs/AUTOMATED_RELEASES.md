# Automated Releases

This project uses GitHub Actions to automatically build and release new versions when changes are pushed to the main branch.

## How it works

1. **Trigger**: Every push to the `main` branch triggers the build and release workflow
2. **Version Control**: The version is controlled by the `VERSION` file in the repository
3. **Building**: The application is built using the same process as `build.bat`
4. **Release**: A new GitHub release is created with the built executable using the version from the VERSION file

## Version Management

- The version is stored in the `VERSION` file (e.g., `1.0.2`)
- **Developer controls the version**: Update the VERSION file to the desired version before pushing
- Version format follows semantic versioning: `MAJOR.MINOR.PATCH`
- The workflow uses whatever version is specified in the VERSION file

### Version Update Process

**Before creating a release:**
1. Update the `VERSION` file to the desired version (e.g., `1.0.2`)
2. Commit and push your changes to the main branch
3. The GitHub Action will automatically create a release with that version

**Example workflow:**
```bash
# Update VERSION file to 1.0.2
echo "1.0.2" > VERSION

# Commit and push
git add VERSION
git commit -m "Prepare release v1.0.2"
git push origin main

# GitHub Action automatically creates release v1.0.2
```

## Manual Release

You can also trigger a release manually:
1. Go to the "Actions" tab in GitHub
2. Select "Build and Release" workflow
3. Click "Run workflow" button
4. Choose the branch (usually `main`) and click "Run workflow"

## Release Assets

Each release includes:
- `UE-Git-Plugin-Manager.exe` - The built executable ready to run

## Workflow Details

The GitHub Action workflow (`/.github/workflows/build-and-release.yml`) performs these steps:

1. **Setup**: Checkout code, setup Go 1.21
2. **Version Reading**: Read version from VERSION file
3. **Build Process**: 
   - Create necessary directories (`logs`, `dist`)
   - Download dependencies (`go mod tidy`)
   - Build executable (`go build -o dist/UE-Git-Plugin-Manager.exe .`)
   - Verify the executable was created
4. **Release Creation**: Create GitHub release with the version from VERSION file
5. **Asset Upload**: Upload the built executable as a release asset

## Requirements

- Go 1.21 or later
- Windows build environment (GitHub Actions provides this)
- Proper Git permissions for the repository

## Troubleshooting

If the workflow fails:
1. Check the Actions tab for error details
2. Ensure the `VERSION` file exists and contains a valid version number
3. Verify that the Go build process works locally with `build.bat`
4. Check that all dependencies are properly specified in `go.mod`
