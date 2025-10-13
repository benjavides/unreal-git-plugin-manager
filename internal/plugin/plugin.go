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

	// Check what's actually at the path using dir command (more reliable for junctions)
	fmt.Printf("  Checking for existing junction at: %s\n", pluginLinkPath)

	// Use dir command to check if the junction exists (more reliable than os.Stat for broken junctions)
	pluginsDir := filepath.Join(enginePath, "Engine", "Plugins")
	dirCmd := exec.Command("cmd", "/c", "dir", pluginsDir)
	dirOutput, dirErr := dirCmd.Output()

	junctionExists := false
	if dirErr == nil {
		dirStr := string(dirOutput)
		fmt.Printf("  📁 Dir output analysis:\n")
		fmt.Printf("    Contains <JUNCTION>: %t\n", strings.Contains(dirStr, "<JUNCTION>"))
		fmt.Printf("    Contains UEGitPlugin_PB: %t\n", strings.Contains(dirStr, "UEGitPlugin_PB"))

		// Look for the specific junction in the dir output
		if strings.Contains(dirStr, "<JUNCTION>") && strings.Contains(dirStr, "UEGitPlugin_PB") {
			junctionExists = true
			fmt.Printf("  📁 Junction found in dir output: UEGitPlugin_PB\n")
		} else {
			fmt.Printf("  📁 No junction found in dir output\n")
		}
	} else {
		fmt.Printf("  ❌ Dir command failed: %v\n", dirErr)
	}

	// Also try our detection methods
	if fileInfo, err := os.Stat(pluginLinkPath); err == nil {
		fmt.Printf("  📁 Path exists via os.Stat: isDir=%t, mode=%s\n", fileInfo.IsDir(), fileInfo.Mode())
	} else {
		fmt.Printf("  📁 Path does not exist via os.Stat: %v\n", err)
	}

	// Use the more reliable detection method
	if junctionExists || m.JunctionExists(pluginLinkPath) {
		fmt.Printf("  ✅ Existing junction found, removing...\n")
		if err := m.RemoveJunction(pluginLinkPath); err != nil {
			return fmt.Errorf("failed to remove existing junction: %v", err)
		}
		fmt.Printf("  ✅ Existing junction removed\n")
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
	if _, err := os.Stat(pluginLinkPath); err == nil {
		return fmt.Errorf("path still exists after removal attempts: %s", pluginLinkPath)
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

	// Check if the command actually succeeded by looking at the output
	// mklink returns exit code 0 and shows "Junction created for..." when successful
	if err != nil {
		// Combine stdout and stderr for error analysis
		combinedOutput := outputStr + errorStr

		// Provide more specific error messages
		if strings.Contains(combinedOutput, "Access is denied") || strings.Contains(combinedOutput, "access denied") {
			return fmt.Errorf("access denied - please run as administrator to create junctions in %s", enginePath)
		}
		if strings.Contains(combinedOutput, "The system cannot find the path specified") {
			return fmt.Errorf("target path not found: %s", worktreePath)
		}
		if strings.Contains(combinedOutput, "Cannot create a file when that file already exists") {
			return fmt.Errorf("junction already exists at %s - this should have been removed first", pluginLinkPath)
		}
		return fmt.Errorf("failed to create junction: %v, output: %s, error: %s", err, outputStr, errorStr)
	}

	// Verify the junction was actually created
	// Check for both junction and symbolic link success messages
	successMessages := []string{
		"Junction created for",
		"symbolic link created for",
		"created for",
	}

	success := false
	for _, msg := range successMessages {
		if strings.Contains(outputStr, msg) {
			success = true
			break
		}
	}

	if !success {
		return fmt.Errorf("junction creation may have failed - unexpected output: %s", outputStr)
	}

	return nil
}

// JunctionExists checks if a junction exists at the given path
func (m *Manager) JunctionExists(path string) bool {
	// First check if the path exists at all
	fileInfo, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}

	// If it exists, check if it's a directory (junctions appear as directories)
	if !fileInfo.IsDir() {
		return false
	}

	// Try the simple method first (works better with broken junctions)
	isJunction := m.IsJunctionSimple(path)

	// If simple method fails, try the Windows API method
	if !isJunction {
		isJunction = m.IsJunction(path)
	}

	return isJunction
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
