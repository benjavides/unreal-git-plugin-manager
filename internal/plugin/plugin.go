package plugin

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// Manager handles plugin linking and junction management
type Manager struct {
	exeDir string
}

// New creates a new plugin manager
func New(exeDir string) *Manager {
	return &Manager{
		exeDir: exeDir,
	}
}

// CreateJunction creates a junction from the engine's plugin directory to the worktree
func (m *Manager) CreateJunction(enginePath, worktreePath string) error {
	pluginLinkPath := filepath.Join(enginePath, "Engine", "Plugins", "UEGitPlugin_PB")

	// Check if we have write access to the engine directory
	if !m.CheckWriteAccess(filepath.Join(enginePath, "Engine", "Plugins")) {
		return fmt.Errorf("insufficient permissions to create junction in %s - please run as administrator", filepath.Join(enginePath, "Engine", "Plugins"))
	}

	// Check for existing junction using language-independent methods
	fmt.Printf("  Checking for existing junction at: %s\n", pluginLinkPath)

	// First check if the symlink itself exists using Lstat (doesn't follow symlinks)
	// This is critical - Stat() follows symlinks, so if target doesn't exist, it returns "does not exist"
	// Lstat() checks if the symlink itself exists, regardless of target
	var lstatInfo os.FileInfo
	var lstatErr error
	var readlinkTarget string
	pathExists := false
	junctionExists := false

	if lstatInfo, lstatErr = os.Lstat(pluginLinkPath); lstatErr == nil {
		pathExists = true
		fmt.Printf("  📁 Path exists (via Lstat): isDir=%t\n", lstatInfo.IsDir())

		// Try to read it as a link (works even if target doesn't exist)
		if target, readErr := os.Readlink(pluginLinkPath); readErr == nil && target != "" {
			readlinkTarget = target
			fmt.Printf("  🔗 Path is readable as link, target: %s\n", target)
			junctionExists = true
		}
	} else {
		fmt.Printf("  📁 Path does not exist (via Lstat): %v\n", lstatErr)
	}

	// If Lstat didn't find it, try JunctionExists (which also uses Lstat now)
	if !junctionExists {
		junctionExists = m.JunctionExists(pluginLinkPath)
		if junctionExists {
			fmt.Printf("  ✅ Junction detected via JunctionExists()\n")
			// Try to get the target
			if target, readErr := os.Readlink(pluginLinkPath); readErr == nil && target != "" {
				readlinkTarget = target
			}
		}
	}

	// If junction exists, check if it points to the correct location
	if junctionExists && readlinkTarget != "" {
		expectedAbs, _ := filepath.Abs(worktreePath)
		targetAbs, _ := filepath.Abs(readlinkTarget)
		if expectedAbs != targetAbs {
			fmt.Printf("  ⚠️  Junction exists but points to wrong location (%s, expected %s)\n", readlinkTarget, worktreePath)
			fmt.Printf("  Removing old junction to recreate with correct target...\n")
			// Try multiple removal methods
			removed := false
			// First try Go's os.Remove (works well for symlinks)
			if err := os.Remove(pluginLinkPath); err == nil {
				fmt.Printf("  ✅ Junction removed using os.Remove()\n")
				removed = true
			} else {
				fmt.Printf("  ⚠️  os.Remove() failed: %v, trying RemoveJunction()...\n", err)
				// If that fails, try RemoveJunction
				if err := m.RemoveJunction(pluginLinkPath); err == nil {
					fmt.Printf("  ✅ Junction removed using RemoveJunction()\n")
					removed = true
				} else {
					fmt.Printf("  ⚠️  RemoveJunction() failed: %v, trying ForceRemovePath()...\n", err)
					// If that fails, try force removal
					if err := m.ForceRemovePath(pluginLinkPath); err == nil {
						fmt.Printf("  ✅ Junction removed using ForceRemovePath()\n")
						removed = true
					} else {
						fmt.Printf("  ⚠️  ForceRemovePath() failed: %v\n", err)
					}
				}
			}

			// Verify it's gone
			if _, statErr := os.Lstat(pluginLinkPath); statErr == nil {
				return fmt.Errorf("old junction still exists after removal attempts: %s", pluginLinkPath)
			}

			if removed {
				fmt.Printf("  ✅ Old junction removed successfully\n")
			} else {
				return fmt.Errorf("failed to remove old junction pointing to wrong location: all removal methods failed")
			}

			junctionExists = false
			pathExists = false
		} else {
			fmt.Printf("  ✅ Junction exists and points to correct location\n")
			// Junction is valid, no need to recreate
			return nil
		}
	}

	// If junction exists or path exists, try to remove it
	if junctionExists || pathExists {
		fmt.Printf("  ✅ Existing junction or path found, removing...\n")
		removed := false
		// First try Go's os.Remove (works well for symlinks)
		if err := os.Remove(pluginLinkPath); err == nil {
			fmt.Printf("  ✅ Junction removed using os.Remove()\n")
			removed = true
		} else {
			osRemoveErr := err
			fmt.Printf("  ⚠️  os.Remove() failed: %v, trying RemoveJunction()...\n", osRemoveErr)
			// If that fails, try RemoveJunction
			if err := m.RemoveJunction(pluginLinkPath); err == nil {
				fmt.Printf("  ✅ Junction removed using RemoveJunction()\n")
				removed = true
			} else {
				removeJunctionErr := err
				fmt.Printf("  ⚠️  RemoveJunction() failed: %v, trying ForceRemovePath()...\n", removeJunctionErr)
				// If that fails, try force removal
				if err := m.ForceRemovePath(pluginLinkPath); err == nil {
					fmt.Printf("  ✅ Junction removed using ForceRemovePath()\n")
					removed = true
				} else {
					forceRemoveErr := err
					return fmt.Errorf("failed to remove existing junction: all removal methods failed (os.Remove: %v, RemoveJunction: %v, ForceRemovePath: %v)", osRemoveErr, removeJunctionErr, forceRemoveErr)
				}
			}
		}

		// Verify it's gone
		if _, statErr := os.Lstat(pluginLinkPath); statErr == nil {
			return fmt.Errorf("junction still exists after removal attempts: %s", pluginLinkPath)
		}

		if removed {
			fmt.Printf("  ✅ Existing junction removed successfully\n")
		}
	} else {
		fmt.Printf("  ✅ No existing junction found\n")
	}

	// Verify worktree exists
	if _, err := os.Stat(worktreePath); err != nil {
		return fmt.Errorf("worktree path does not exist: %s", worktreePath)
	}

	// Verify plugins directory exists
	pluginsDirForStat := filepath.Join(enginePath, "Engine", "Plugins")
	if _, err := os.Stat(pluginsDirForStat); err != nil {
		return fmt.Errorf("plugins directory does not exist: %s", pluginsDirForStat)
	}

	// Test write access
	if !m.CheckWriteAccess(pluginsDirForStat) {
		return fmt.Errorf("no write access to plugins directory: %s", pluginsDirForStat)
	}

	// Double-check the path right before creating the junction
	// If it still exists, try force removal one more time
	if _, err := os.Stat(pluginLinkPath); err == nil {
		fmt.Printf("  ⚠️  Path still exists, attempting force removal...\n")
		if removeErr := m.ForceRemovePath(pluginLinkPath); removeErr != nil {
			return fmt.Errorf("path still exists after removal attempts: %s (force removal failed: %v)", pluginLinkPath, removeErr)
		}
		// Verify it's actually gone
		if _, err := os.Stat(pluginLinkPath); err == nil {
			return fmt.Errorf("path still exists after force removal: %s", pluginLinkPath)
		}
	}

	// Create the junction using mklink
	// Try /D first (directory symbolic link), fall back to /J (junction) if that fails
	mklinkCmd := exec.Command("cmd", "/c", "mklink", "/D", pluginLinkPath, worktreePath)

	// Capture both stdout and stderr
	var stdout, stderr bytes.Buffer
	mklinkCmd.Stdout = &stdout
	mklinkCmd.Stderr = &stderr

	err := mklinkCmd.Run()
	outputStr := stdout.String()
	errorStr := stderr.String()
	needsRetry := false

	// Check if the command actually succeeded
	// If mklink failed, check if the path now exists (it might have succeeded despite error)
	if err != nil {
		// First check if the path exists (might be "already exists" error)
		if _, statErr := os.Stat(pluginLinkPath); statErr == nil {
			// Path exists - check if it's a valid junction/symlink
			fmt.Printf("  ⚠️  mklink failed but path exists, checking if it's a valid junction...\n")

			// Try multiple methods to detect junction
			isJunction := m.JunctionExists(pluginLinkPath)
			isSymlink := false
			canReadlink := false
			var readlinkTarget string

			if fi, lstatErr := os.Lstat(pluginLinkPath); lstatErr == nil {
				isSymlink = fi.Mode()&os.ModeSymlink != 0
			}

			// Try os.Readlink as fallback (works even if other detection fails)
			if target, readErr := os.Readlink(pluginLinkPath); readErr == nil && target != "" {
				canReadlink = true
				readlinkTarget = target
				// If we can read it as a link, it's definitely a junction/symlink
				if !isJunction && !isSymlink {
					fmt.Printf("  🔗 Path is readable as link (target: %s), treating as junction\n", target)
					isJunction = true
				}
			}

			if isJunction || isSymlink || canReadlink {
				// It's a valid junction/symlink - verify it points to the right place
				var target string
				var targetErr error
				if canReadlink {
					target = readlinkTarget
				} else {
					target, targetErr = m.GetJunctionTarget(pluginLinkPath)
				}

				if targetErr == nil {
					expectedAbs, _ := filepath.Abs(worktreePath)
					targetAbs, _ := filepath.Abs(target)
					if expectedAbs == targetAbs {
						// Junction exists and points to the right place - this is success!
						fmt.Printf("  ✅ Junction already exists and points to correct target (despite mklink error)\n")
						// Continue to verification below (which will pass)
					} else {
						// Junction exists but points to wrong place - need to recreate
						fmt.Printf("  ⚠️  Junction exists but points to wrong target (%s, expected %s), removing...\n", target, worktreePath)
						if removeErr := m.ForceRemovePath(pluginLinkPath); removeErr != nil {
							return fmt.Errorf("junction exists at %s but points to wrong target and could not be removed: %v", pluginLinkPath, removeErr)
						}
						// Retry creation below
						needsRetry = true
					}
				} else {
					// Can't read target, but it's a junction - assume it's valid
					fmt.Printf("  ✅ Junction exists (could not read target, but appears valid)\n")
					// Continue to verification below
				}
			} else {
				// Path exists but is not a junction - this is the "already exists" case
				// Try to remove it and retry creation once
				fmt.Printf("  ⚠️  Path exists but is not a junction, attempting removal...\n")
				if removeErr := m.ForceRemovePath(pluginLinkPath); removeErr != nil {
					return fmt.Errorf("path exists at %s but is not a junction and could not be removed: %v", pluginLinkPath, removeErr)
				}
				// Verify it's gone
				if _, statErr := os.Stat(pluginLinkPath); statErr == nil {
					return fmt.Errorf("path still exists after force removal: %s", pluginLinkPath)
				}
				// Retry creation once
				needsRetry = true
			}
		} else {
			// Path doesn't exist, mklink actually failed
			// Check if the path exists now (might have been created despite error)
			if _, statErr := os.Stat(pluginLinkPath); statErr == nil {
				// Path exists but mklink reported error - this is unusual
				// Try to verify if it's a valid junction
				if m.JunctionExists(pluginLinkPath) {
					fmt.Printf("  ✅ Junction was created despite error message\n")
					// Continue to verification below
				} else {
					return fmt.Errorf("path was created but is not a valid junction: %s", pluginLinkPath)
				}
			} else {
				// Path truly doesn't exist, mklink failed
				// Check error code for common issues
				if exitError, ok := err.(*exec.ExitError); ok {
					exitCode := exitError.ExitCode()
					// Exit code 1 can mean different things - don't assume it's access denied
					if exitCode == 1 {
						// Check if worktree path exists
						if _, statErr := os.Stat(worktreePath); statErr != nil {
							return fmt.Errorf("target path not found: %s", worktreePath)
						}
						// Check if we have write access (might be permission issue)
						if !m.CheckWriteAccess(filepath.Join(enginePath, "Engine", "Plugins")) {
							return fmt.Errorf("access denied - please run as administrator to create junctions in %s (exit code: %d)", enginePath, exitCode)
						}
						// Could be other issues - provide more context
						return fmt.Errorf("failed to create junction (exit code: %d). Output: %s, Error: %s. Check if path exists or if there are permission issues.", exitCode, outputStr, errorStr)
					}
				}
				return fmt.Errorf("failed to create junction: %v, output: %s, error: %s", err, outputStr, errorStr)
			}
		}
	}

	// If we need to retry creation (path was removed)
	if needsRetry {
		fmt.Printf("  Retrying junction creation...\n")
		mklinkCmd = exec.Command("cmd", "/c", "mklink", "/D", pluginLinkPath, worktreePath)
		mklinkCmd.Stdout = &stdout
		mklinkCmd.Stderr = &stderr
		stdout.Reset()
		stderr.Reset()
		err = mklinkCmd.Run()
		outputStr = stdout.String()
		errorStr = stderr.String()

		if err != nil {
			// Check if it was created despite error
			if _, statErr := os.Stat(pluginLinkPath); statErr == nil {
				if m.JunctionExists(pluginLinkPath) {
					fmt.Printf("  ✅ Junction created successfully on retry (despite error message)\n")
					// Continue to verification below
				} else {
					return fmt.Errorf("path was created but is not a valid junction: %s", pluginLinkPath)
				}
			} else {
				return fmt.Errorf("failed to create junction on retry: %v, output: %s, error: %s", err, outputStr, errorStr)
			}
		}
	}

	// Locale-agnostic verification: inspect filesystem instead of parsing localized output
	// 1) Path must now exist
	fi, lerr := os.Lstat(pluginLinkPath)
	if lerr != nil {
		return fmt.Errorf("link not created at %s: %v", pluginLinkPath, lerr)
	}

	// 2) If it's a symlink, ensure it points to the expected worktree
	if fi.Mode()&os.ModeSymlink != 0 {
		target, rerr := os.Readlink(pluginLinkPath)
		if rerr != nil {
			return fmt.Errorf("could not read symlink target: %v", rerr)
		}
		expectedAbs, _ := filepath.Abs(worktreePath)
		targetAbs, _ := filepath.Abs(target)
		if expectedAbs != targetAbs {
			return fmt.Errorf("symlink target mismatch: got %s, want %s", targetAbs, expectedAbs)
		}
		return nil
	}

	// 3) Otherwise verify it's a junction/reparse point and points to the worktree
	if !m.JunctionExists(pluginLinkPath) {
		return fmt.Errorf("created path is not a junction or symlink: %s", pluginLinkPath)
	}

	if !m.VerifyJunction(enginePath, worktreePath) {
		return fmt.Errorf("junction does not point to expected target: %s", worktreePath)
	}

	return nil
}

// JunctionExists checks if a junction or symlink exists at the given path
// This function accepts both junctions and directory symlinks (created with mklink /D)
func (m *Manager) JunctionExists(path string) bool {
	// First check if the path exists at all using Lstat (doesn't follow symlinks)
	// This is critical - Stat() follows symlinks, so if target doesn't exist, it returns "does not exist"
	// Lstat() checks if the symlink itself exists, regardless of target
	fileInfo, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false
	}

	// Check if it's a symlink first (works for both file and directory symlinks)
	if fileInfo.Mode()&os.ModeSymlink != 0 {
		// It's a symlink - try to read it to confirm
		if target, err := os.Readlink(path); err == nil && target != "" {
			return true
		}
	}

	// If it exists, check if it's a directory (junctions appear as directories)
	// But also check if it can be read as a link (might be a directory symlink)
	if fileInfo.IsDir() {
		// Try os.Readlink first (works for both symlinks and junctions in Go)
		// This is the most reliable method and works across locales
		if target, err := os.Readlink(path); err == nil && target != "" {
			return true
		}

		// Try the simple method (fsutil) - works better with broken junctions
		isJunction := m.IsJunctionSimple(path)

		// If simple method fails, try the Windows API method
		if !isJunction {
			isJunction = m.IsJunction(path)
		}

		return isJunction
	}

	// If it's not a directory and not a symlink, it's not a junction/symlink
	return false
}

// IsJunction checks if a path is a junction (reparse point)
func (m *Manager) IsJunction(path string) bool {
	// Use Windows API to check if the path is a reparse point
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false
	}

	handle, err := syscall.CreateFile(
		pathPtr,
		syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_OPEN_REPARSE_POINT|syscall.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(handle)

	var reparseData [1024]byte
	var bytesReturned uint32

	err = syscall.DeviceIoControl(
		handle,
		FSCTL_GET_REPARSE_POINT,
		nil,
		0,
		&reparseData[0],
		uint32(len(reparseData)),
		&bytesReturned,
		nil,
	)

	return err == nil
}

// IsJunctionSimple uses a simpler method to detect junctions
func (m *Manager) IsJunctionSimple(path string) bool {
	// Use fsutil to check if it's a junction
	cmd := exec.Command("fsutil", "reparsepoint", "query", path)
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	// If fsutil returns output, it's a reparse point
	return len(output) > 0
}

// RemoveJunction removes a junction
func (m *Manager) RemoveJunction(path string) error {
	if !m.JunctionExists(path) {
		return nil // Already removed
	}

	// Use rmdir to remove the junction
	cmd := exec.Command("cmd", "/c", "rmdir", path)

	// Capture both stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	outputStr := stdout.String()
	errorStr := stderr.String()

	if err != nil {
		return fmt.Errorf("failed to remove junction: %v, output: %s, error: %s", err, outputStr, errorStr)
	}

	return nil
}

// ForceRemovePath attempts to remove a path using multiple methods
func (m *Manager) ForceRemovePath(path string) error {
	// Try rmdir first (for junctions)
	cmd := exec.Command("cmd", "/c", "rmdir", path)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	if err == nil {
		return nil
	}

	// Try rmdir /s /q (for directories with contents)
	cmd = exec.Command("cmd", "/c", "rmdir", "/s", "/q", path)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()

	if err == nil {
		return nil
	}

	return fmt.Errorf("all removal methods failed")
}

// GetJunctionTarget gets the target path of a junction or symbolic link
func (m *Manager) GetJunctionTarget(path string) (string, error) {
	// Check if it's either a junction or symbolic link
	if !m.IsJunction(path) && !m.IsJunctionSimple(path) {
		return "", fmt.Errorf("path is not a junction or symbolic link")
	}

	// Try using Go's built-in Readlink function first (simpler approach)
	target, err := os.Readlink(path)
	if err == nil {
		return target, nil
	}

	// Fallback: try using dir command to get the target
	cmd := exec.Command("cmd", "/c", "dir", path)
	output, err := cmd.Output()
	dirStr := string(output)

	if err == nil {
		// Look for the target in the dir output (format: <JUNCTION> UEGitPlugin_PB [target_path])
		lines := strings.Split(dirStr, "\n")
		for _, line := range lines {
			if strings.Contains(line, "UEGitPlugin_PB") && strings.Contains(line, "[") && strings.Contains(line, "]") {
				// Extract the target path between [ and ]
				start := strings.Index(line, "[")
				end := strings.Index(line, "]")
				if start != -1 && end != -1 && end > start {
					target := strings.TrimSpace(line[start+1 : end])
					return target, nil
				}
			}
		}
	}

	return "", fmt.Errorf("could not parse junction/symbolic link target")
}

// VerifyJunction verifies that a junction points to the correct worktree
func (m *Manager) VerifyJunction(enginePath, expectedWorktreePath string) bool {
	pluginLinkPath := filepath.Join(enginePath, "Engine", "Plugins", "UEGitPlugin_PB")

	if !m.JunctionExists(pluginLinkPath) {
		return false
	}

	target, err := m.GetJunctionTarget(pluginLinkPath)
	if err != nil {
		return false
	}

	// Normalize paths for comparison
	expectedAbs, _ := filepath.Abs(expectedWorktreePath)
	targetAbs, _ := filepath.Abs(target)

	return expectedAbs == targetAbs
}

// GetPluginLinkPath returns the plugin link path for an engine
func (m *Manager) GetPluginLinkPath(enginePath string) string {
	return filepath.Join(enginePath, "Engine", "Plugins", "UEGitPlugin_PB")
}

// CheckWriteAccess checks if we have write access to a directory
func (m *Manager) CheckWriteAccess(path string) bool {
	// Try to create a temporary file in the directory
	tempFile := filepath.Join(path, ".ue-git-plugin-manager-test")
	file, err := os.Create(tempFile)
	if err != nil {
		return false
	}
	file.Close()
	os.Remove(tempFile)
	return true
}

// Windows API constants for reparse point handling
const (
	FSCTL_GET_REPARSE_POINT = 0x900a8
)

// BuildForEngine compiles the plugin against a specific UE engine and
// copies the produced Binaries back into the worktree so the engine
// can load them via the junction.
func (m *Manager) BuildForEngine(enginePath, worktreePath string) error {
	uat := filepath.Join(enginePath, "Engine", "Build", "BatchFiles", "RunUAT.bat")
	if _, err := os.Stat(uat); err != nil {
		return fmt.Errorf("RunUAT not found at %s", uat)
	}

	uplugin := filepath.Join(worktreePath, "GitSourceControl.uplugin")
	if _, err := os.Stat(uplugin); err != nil {
		return fmt.Errorf("uplugin not found at %s", uplugin)
	}

	buildOut := filepath.Join(worktreePath, "_Built")
	_ = os.RemoveAll(buildOut) // clean previous packaged output

	// Build: call UAT directly with proper working directory
	// On Windows, use cmd /c to properly handle paths with spaces
	var cmd *exec.Cmd
	if strings.Contains(uat, " ") {
		// Path contains spaces, use cmd /c with proper argument handling
		// First change to the engine directory, then execute the batch file
		cmd = exec.Command("cmd", "/c",
			"cd", "/d", enginePath, "&&",
			uat, "BuildPlugin",
			fmt.Sprintf("-Plugin=%s", uplugin),
			fmt.Sprintf("-Package=%s", buildOut),
			"-Rocket",
			"-TargetPlatforms=Win64")
	} else {
		// Path has no spaces, can execute directly
		cmd = exec.Command(uat, "BuildPlugin",
			fmt.Sprintf("-Plugin=%s", uplugin),
			fmt.Sprintf("-Package=%s", buildOut),
			"-Rocket",
			"-TargetPlatforms=Win64")
		// Set working directory to the engine directory for proper UAT execution
		cmd.Dir = enginePath
	}

	// Debug: print the command being executed
	if strings.Contains(uat, " ") {
		fmt.Printf("Executing: cmd /c cd /d \"%s\" && \"%s\" BuildPlugin -Plugin=\"%s\" -Package=\"%s\" -Rocket -TargetPlatforms=Win64\n",
			enginePath, uat, uplugin, buildOut)
	} else {
		fmt.Printf("Executing: \"%s\" BuildPlugin -Plugin=\"%s\" -Package=\"%s\" -Rocket -TargetPlatforms=Win64\n",
			uat, uplugin, buildOut)
		fmt.Printf("Working directory: %s\n", enginePath)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("BuildPlugin failed (see output above): %w", err)
	}

	// Debug: explore the build output structure
	fmt.Printf("Debug - Build output structure:\n")
	fmt.Printf("  Build output directory: %s\n", buildOut)

	// List contents of build output directory
	if entries, err := os.ReadDir(buildOut); err == nil {
		fmt.Printf("  Build output contents:\n")
		for _, entry := range entries {
			fmt.Printf("    - %s (%s)\n", entry.Name(), func() string {
				if entry.IsDir() {
					return "directory"
				}
				return "file"
			}())
		}
	} else {
		fmt.Printf("  Could not read build output directory: %v\n", err)
	}

	// Try to find the actual binaries location
	// Based on the actual UAT output structure, binaries are at _Built/Binaries/Win64/
	src := filepath.Join(buildOut, "Binaries", "Win64")
	fmt.Printf("  Looking for binaries at: %s\n", src)

	if _, err := os.Stat(src); err != nil {
		fmt.Printf("  ❌ Expected path does not exist: %v\n", err)

		// Try alternative paths as fallback
		altPaths := []string{
			filepath.Join(buildOut, "Binaries"),
			filepath.Join(buildOut, "GitSourceControl", "Binaries"),
			filepath.Join(buildOut, "GitSourceControl"),
			filepath.Join(buildOut, "Plugins", "GitSourceControl", "Binaries"),
		}

		for _, altPath := range altPaths {
			fmt.Printf("  Trying alternative path: %s\n", altPath)
			if _, err := os.Stat(altPath); err == nil {
				fmt.Printf("  ✅ Found binaries at: %s\n", altPath)
				src = altPath
				break
			} else {
				fmt.Printf("  ❌ Not found: %v\n", err)
			}
		}
	} else {
		fmt.Printf("  ✅ Found binaries at expected path\n")
	}

	dst := filepath.Join(worktreePath, "Binaries", "Win64")
	fmt.Printf("  Copying from: %s\n", src)
	fmt.Printf("  Copying to: %s\n", dst)

	if err := copyDir(src, dst); err != nil {
		return fmt.Errorf("failed to copy built binaries: %w", err)
	}

	// Debug: verify the final structure
	fmt.Printf("  ✅ Binaries copied successfully\n")
	fmt.Printf("  Final plugin structure:\n")
	fmt.Printf("    Plugin file: %s\n", filepath.Join(worktreePath, "GitSourceControl.uplugin"))
	fmt.Printf("    Binaries: %s\n", dst)

	// List the copied binaries
	if entries, err := os.ReadDir(dst); err == nil {
		fmt.Printf("    Copied files:\n")
		for _, entry := range entries {
			fmt.Printf("      - %s\n", entry.Name())
		}
	}

	return nil
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
