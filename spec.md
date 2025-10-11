# Spec — UE Git Plugin Manager (Windows, single-file CLI)

> **Note:** This is a technical specification document intended for LLM context and development purposes. For user documentation, see README.md.

## 1) Purpose

Install and maintain Project Borealis' **UEGitPlugin** across **multiple Unreal Engine installs** using:

- one **origin clone** (in user config directory)
- one **worktree per engine version** (also in user config directory)
- a **junction** inside each Engine's `Engine/Plugins/` that points at its worktree
- a simple **menu** that non-technical users can operate

> Note: PB’s README recommends disabling the built-in `GitSourceControl` when installing at the *engine* level; we won’t do that by default. We’ll detect a collision and offer a one-click “Fix plugin collision (recommended)” that performs the rename per engine. [GitHub](https://github.com/ProjectBorealis/UEGitPlugin)

---

## 2) Operating assumptions

- **Windows only**.
- **System Git** must be in `PATH`. If missing, show a friendly message with a button to open Git for Windows download page.
- **Single-file binary** (Go is fine) that shells out to `git`.

---

## 3) On-disk layout (fixed user config directory)

```
%APPDATA%\ue-git-plugin-manager\
  config.json
  repo-origin\           (git clone of https://github.com/ProjectBorealis/UEGitPlugin)
  worktrees\
    UE_5.3\
    UE_5.4\
    UE_5.5\
```

- `repo-origin` = origin clone
- `worktrees\UE_5.x` = per-engine worktree folders (all use same default branch)
- `config.json` = configuration file
- **Fixed location**: Data is stored in user config directory, not relative to executable
- **No relocation needed**: Executable can be moved anywhere without affecting data

---

## 4) Engine discovery

- Scan default: `C:\Program Files\Epic Games\UE_*`
- Plus **user-added custom roots** (persisted in config); recurse depth = 2
- Validate engine by presence of:
  - `Engine\Binaries\Win64\UnrealEditor.exe`
- Extract version from folder name (`UE_5.4`) or `Engine\Build\Build.version` fallback.

---

## 5) Git model (system Git)

- **Clone** (if missing):
  `git clone https://github.com/ProjectBorealis/UEGitPlugin repo-origin`
- **Default remote branch**: parse from `git -C repo-origin remote show origin` (HEAD). Fallback: `dev`.
- **Per engine**:
  - All worktrees use the same default branch (no engine-specific branches)
  - **worktree add** (detached HEAD):
    `git -C repo-origin worktree add --detach "worktrees\UE_5.4" <default-branch>`
- **Update**:
  - `git -C repo-origin fetch --all --prune`
  - For each worktree:
    - local = `git -C worktrees\UE_5.4 rev-parse HEAD`
    - remote = `git -C repo-origin rev-parse origin/<default>`
    - ahead count = `git -C repo-origin rev-list --count <local>..origin/<default>`
    - **Update now** = `git -C worktrees\UE_5.4 merge --ff-only origin/<default>`
      (or `git pull --ff-only` in the worktree)

---

## 6) Plugin linking (no disable by default)

- Create **junction** under each engine:
  `Engine\Plugins\UEGitPlugin_PB` → `%APPDATA%\ue-git-plugin-manager\worktrees\UE_5.x`
  (`cmd /c mklink /J ...`)
- **Collision detection** (non-destructive by default):
  - If both the stock plugin **and** PB plugin share the same plugin **Name** internally (they do—file is `GitSourceControl.uplugin`), UE may load ambiguously. If detected, show **“Potential plugin name collision detected”** with a one-click **Fix** that renames:
    `Engine\Plugins\Developer\GitSourceControl.uplugin` → `.uplugin.disabled`
    (record we did it so uninstall can restore)
  - Users can skip the fix; in that case, we still link PB’s plugin and let users select **“Git LFS 2”** as provider in Editor.

---

## 7) Menu (context-aware; plain language)

### Main menu (always shown)

1. **What is this?** - Learn about the tool and its purpose
2. **Edit Setup** - Manage engines (install, update, repair, uninstall)
3. **Settings** - Configure paths, branches, open data directory
4. **Quit**

### Settings menu

- Add Engine Path (add/remove custom engine paths)
- Change Branch to Track (default stays `origin/<default>`)
- Open Plugin Repository (opens GitHub in browser)
- Open Data Directory (opens config directory in file explorer)
- Back

> We never say “worktree” in the UI. Wording: “Install plugin for UE 5.4”, “Update plugin”, etc.

---

## 8) Engine setup flow (via Edit Setup)

1. **Detect engines** and show status for each (Not Set Up, Setup Complete, Setup Broken)
2. **User selects engine** to manage
3. **Show options** based on status:
   - Not Set Up: "Install Setup"
   - Setup Complete: "Update Setup", "Uninstall Setup"  
   - Setup Broken: "Repair Setup", "Uninstall Setup"
4. For Install/Repair:
   - **Check Git** in PATH; if missing, show error
   - **Clone** origin (if absent) and resolve default branch
   - **Create worktree** (detached HEAD from default branch)
   - **Build plugin** against the specific engine version
   - **Create junction** `Engine\Plugins\UEGitPlugin_PB` → worktree path
   - **Disable stock plugin** (recommended to avoid conflicts)
5. Save `config.json`

---

## 9) Update flow (per engine)

1. **Check for updates** using `GetUpdateInfo()`:
   - `fetch --all --prune` on `repo-origin`
   - Compare local worktree HEAD vs `origin/<default>`
   - Count commits ahead: `git rev-list --count <local>..origin/<default>`
2. **If no updates** (commits ahead = 0):
   - Show "Already up to date" message
   - Skip rebuild
3. **If updates available** (commits ahead > 0):
   - Show "X commits available"
   - Show local and remote commit SHAs
   - Show GitHub compare URL
   - **Update worktree**: `git -C worktrees\UE_5.x merge --ff-only origin/<default>`
   - **Rebuild plugin** against the engine
4. Show final status

---

## 10) Engine management

- **Edit Setup** shows all detected engines with their status
- **Not Set Up**: Shows "Install Setup" option
- **Setup Complete**: Shows "Update Setup" and "Uninstall Setup" options  
- **Setup Broken**: Shows "Repair Setup" and "Uninstall Setup" options
- Each engine managed independently
- No "first run" flow - all functionality accessible from main menu

---

## 11) Uninstall (per engine)

For the selected engine:

- Remove junction `Engine\Plugins\UEGitPlugin_PB` (use `rmdir` on junction)
- Re-enable stock Git plugin (if it was disabled by the tool)
- Delete `worktrees\UE_<ver>`
- Remove engine from config

**Cleanup when last engine uninstalled:**
- Delete `repo-origin` directory
- Keep `config.json` for future use

---

## 12) Configuration (JSON, stored in user config directory)

```
{
  "version": 1,
  "default_remote_branch": "dev",
  "engines": [
    {
      "engine_path": "C:\\Program Files\\Epic Games\\UE_5.4",
      "engine_version": "5.4",
      "worktree_subdir": "UE_5.4",
      "plugin_link_path": "C:\\Program Files\\Epic Games\\UE_5.4\\Engine\\Plugins\\UEGitPlugin_PB",
      "stock_plugin_disabled_by_tool": false
    }
  ],
  "custom_engine_roots": [
    "D:\\Engines",
    "E:\\Unreal\\UE"
  ]
}
```

> Configuration is stored in `%APPDATA%\ue-git-plugin-manager\config.json`. No relocation needed - executable can be moved anywhere.

---

## 13) Permissions / UAC

- **No admin privileges required** on modern Windows for junction creation
- Junction creation works without UAC elevation
- Only requires write access to engine plugin directories

---

## 14) Detection system

- **File-based detection**: Determines setup status by examining actual files and directories
- **Worktree detection**: Checks if worktree directory exists and contains plugin files
- **Junction detection**: Verifies junction exists and points to correct worktree path
- **Binary detection**: Checks for built plugin files (`UnrealEditor-GitSourceControl.dll`, etc.)
- **Stock plugin detection**: Checks if stock Git plugin is disabled
- **Status classification**: Not Set Up, Setup Complete, Setup Broken

---

## 15) UX samples

**Main menu:**

```
🎮 UE Git Plugin Manager - Main Menu

🔍 Detected Engines:

✅ UE 5.3 - Setup Complete
   C:\Program Files\Epic Games\UE_5.3

❌ UE 5.4 - Not Set Up
   C:\Program Files\Epic Games\UE_5.4

⚠️ UE 5.5 - Setup Broken
   C:\Program Files\Epic Games\UE_5.5

? Select an option:
> What is this?
  Edit Setup
  Settings
  Quit
```

**Update flow:**

```
Checking for updates for UE 5.3...
📥 Updates available: 3 commits behind
   Local commit:  a1b2c3d4
   Remote commit: e5f6g7h8
   Compare: https://github.com/ProjectBorealis/UEGitPlugin/compare/a1b2c3d4...e5f6g7h8

Updating worktree...
Rebuilding plugin...
✅ UE 5.3 updated successfully! (3 commits applied)
```

---

## 16) Out-of-scope (v1)

- Cross-platform
- Self-update for the CLI
- Parallel Git operations (can add later)
