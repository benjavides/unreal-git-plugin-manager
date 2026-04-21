# Automated Releases

This project uses GitHub Actions to build and release only when a version tag is pushed.

## How it works

1. **Trigger**: Only pushes of tags that match `v*` trigger the workflow
2. **Version Control**: The release version comes from the Git tag itself (for example `v1.0.8`)
3. **Building**: The application is built with `go build` on Windows
4. **Release**: A GitHub release is created for the pushed tag and the executable is uploaded as an asset

## Version Management (Manual Tags)

- The version is defined by your tag (for example `v1.0.8`)
- **Developer controls releases manually** by creating and pushing tags
- Recommended version format follows semantic versioning: `vMAJOR.MINOR.PATCH`

### Release Process

**Before creating a release:**
1. Commit and push your changes to `main`
2. Create the release tag locally
3. Push the tag
4. The GitHub Action creates the release for that tag

**Example workflow:**
```bash
# Commit and push your code changes
git add .
git commit -m "Prepare release v1.0.8"
git push origin main

# Create and push release tag
git tag v1.0.8
git push origin v1.0.8

# GitHub Action automatically creates release v1.0.8
```

## Release Assets

Each release includes:
- `UE-Git-Plugin-Manager-vX.Y.Z.exe` - The built executable ready to run

## Workflow Details

The GitHub Action workflow (`/.github/workflows/build-and-release.yml`) performs these steps:

1. **Setup**: Checkout code, setup Go 1.21
2. **Tag Reading**: Read the pushed tag from `github.ref_name`
3. **Build Process**: 
   - Create output directory (`dist`)
   - Download dependencies (`go mod tidy`)
   - Build executable (`go build -o dist/UE-Git-Plugin-Manager-vX.Y.Z.exe .`)
   - Verify the executable was created
4. **Release Creation**: Create GitHub release for the pushed tag
5. **Asset Upload**: Upload the built executable as a release asset

## Requirements

- Go 1.21 or later
- Windows build environment (GitHub Actions provides this)
- Proper Git permissions for the repository

## Troubleshooting

If the workflow fails:
1. Check the Actions tab for error details
2. Ensure the pushed tag matches `v*` and points to the expected commit
3. Verify that the Go build process works locally with `build.bat`
4. Check that all dependencies are properly specified in `go.mod`
