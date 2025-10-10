# UE Git Plugin Manager

A Windows CLI tool for managing the UEGitPlugin across multiple Unreal Engine installations.

## Features

- **One-click setup** for multiple Unreal Engine versions
- **Automatic plugin linking** using Windows junctions
- **Update management** with commit tracking and browser integration
- **Collision detection** and resolution for stock Git plugins
- **Relocation support** when moving the executable
- **Comprehensive diagnostics** and error handling

## Requirements

- Windows 10/11
- Git for Windows (must be in PATH)
- Administrator privileges (for plugin installation)
- Unreal Engine 5.3+ installations

## Installation

1. Download the latest release or build from source
2. Extract `UE-Git-Plugin-Manager.exe` to a permanent location
3. **Right-click `UE-Git-Plugin-Manager.exe` and select "Run as administrator"**
   - This is required for creating junctions in Unreal Engine directories

## Building from Source

### Prerequisites
1. **Install Go 1.21+**: Download from https://golang.org/dl/
2. **Install Git for Windows**: Download from https://git-scm.com/download/win
3. **Restart Command Prompt** after installing Go

### Build Options

#### Option 1: Automated Build (Recommended)
```cmd
build.bat
```

#### Option 2: PowerShell Build
```powershell
.\build.ps1
```

#### Option 3: Manual Build
```cmd
go mod init ue-git-plugin-manager
go mod tidy
go build -o UE-Git-Plugin-Manager.exe .
```

### Troubleshooting
- **"Go not recognized"**: Install Go and restart Command Prompt
- **"missing go.sum entry"**: Run `go mod tidy`
- **"git: command not found"**: Install Git for Windows
- **Build fails**: Try running as Administrator

See [SETUP_INSTRUCTIONS.md](SETUP_INSTRUCTIONS.md) for detailed troubleshooting.

## Usage

### First Time Setup

1. Run `UE-Git-Manager.exe`
2. The tool will detect your Unreal Engine installations
3. Select which engines to set up
4. The tool will clone the UEGitPlugin repository and create worktrees
5. Plugin junctions will be created automatically

### Updating Plugins

1. Run the tool and select "Update"
2. View available updates with commit information
3. Click "Update now" to apply updates

### Managing Engines

- **Set up a new engine version**: Add support for newly installed engines
- **Uninstall**: Remove all plugin links and worktrees
- **Advanced**: Configure scan roots, change tracked branch, run diagnostics

## How It Works

The tool uses a sophisticated Git worktree system:

1. **Origin Repository**: Single clone of the UEGitPlugin repository
2. **Worktrees**: Separate working directories for each engine version
3. **Junctions**: Windows symbolic links connecting engines to worktrees
4. **Branch Management**: Each engine gets its own branch tracking the remote

## Configuration

Configuration is stored in `config.json` next to the executable:

```json
{
  "version": 1,
  "base_dir": ".",
  "origin_dir": "repo-origin",
  "worktrees_dir": "worktrees",
  "default_remote_branch": "dev",
  "engines": [...],
  "custom_engine_roots": [...]
}
```

## Troubleshooting

### Git Not Found
- Install Git for Windows from https://git-scm.com/download/win
- Ensure Git is in your system PATH

### Permission Denied
- Run the tool as Administrator
- Check that Unreal Engine directories are writable

### Plugin Not Loading
- Check the Diagnostics menu
- Verify junctions are pointing to correct worktrees
- Ensure no plugin name collisions

### Relocation Issues
- If you move the executable, use the Relocate option
- This will update all paths and junctions automatically

## Project Structure

```
UE-Git-Manager.exe
config.json
logs/
repo-origin/          # Git repository
worktrees/
  UE_5.3/            # Worktree for UE 5.3
  UE_5.4/            # Worktree for UE 5.4
  UE_5.5/            # Worktree for UE 5.5
```

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Test thoroughly on Windows
5. Submit a pull request

## Acknowledgments

- [Project Borealis](https://github.com/ProjectBorealis/UEGitPlugin) for the UEGitPlugin
- The Unreal Engine community for feedback and testing
