package plugin

import (
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

	// Remove existing junction if it exists
	if m.JunctionExists(pluginLinkPath) {
		if err := m.RemoveJunction(pluginLinkPath); err != nil {
			return fmt.Errorf("failed to remove existing junction: %v", err)
		}
	}

	// Debug: print detailed information before attempting junction creation
	fmt.Printf("Debug - Junction creation details:\n")
	fmt.Printf("  Engine path: %s\n", enginePath)
	fmt.Printf("  Worktree path: %s\n", worktreePath)
	fmt.Printf("  Plugin link path: %s\n", pluginLinkPath)

	// Verify worktree exists
	if _, err := os.Stat(worktreePath); err != nil {
		return fmt.Errorf("worktree path does not exist: %s", worktreePath)
	}
	fmt.Printf("  ✅ Worktree exists\n")

	// Verify plugins directory exists
	pluginsDir := filepath.Join(enginePath, "Engine", "Plugins")
	if _, err := os.Stat(pluginsDir); err != nil {
		return fmt.Errorf("plugins directory does not exist: %s", pluginsDir)
	}
	fmt.Printf("  ✅ Plugins directory exists\n")

	// Test write access
	if !m.CheckWriteAccess(pluginsDir) {
		return fmt.Errorf("no write access to plugins directory: %s", pluginsDir)
	}
	fmt.Printf("  ✅ Write access confirmed\n")

	// Create the junction using mklink
	fmt.Printf("  Executing: mklink /J \"%s\" \"%s\"\n", pluginLinkPath, worktreePath)
	cmd := exec.Command("cmd", "/c", "mklink", "/J", pluginLinkPath, worktreePath)
	output, err := cmd.Output()
	outputStr := string(output)

	// Debug: print command result
	fmt.Printf("  Command exit code: %v\n", err)
	fmt.Printf("  Command output: %s\n", outputStr)

	// Check if the command actually succeeded by looking at the output
	// mklink returns exit code 0 and shows "Junction created for..." when successful
	if err != nil {
		// Provide more specific error messages
		if strings.Contains(outputStr, "Access is denied") || strings.Contains(outputStr, "access denied") {
			return fmt.Errorf("access denied - please run as administrator to create junctions in %s", enginePath)
		}
		if strings.Contains(outputStr, "The system cannot find the path specified") {
			return fmt.Errorf("target path not found: %s", worktreePath)
		}
		return fmt.Errorf("failed to create junction: %v, output: %s", err, outputStr)
	}

	// Verify the junction was actually created
	if !strings.Contains(outputStr, "Junction created for") {
		return fmt.Errorf("junction creation may have failed - unexpected output: %s", outputStr)
	}

	return nil
}

// JunctionExists checks if a junction exists at the given path
func (m *Manager) JunctionExists(path string) bool {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}

	// Check if it's actually a junction by trying to read its target
	return m.IsJunction(path)
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

// RemoveJunction removes a junction
func (m *Manager) RemoveJunction(path string) error {
	if !m.JunctionExists(path) {
		return nil // Already removed
	}

	// Use rmdir to remove the junction
	cmd := exec.Command("cmd", "/c", "rmdir", path)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to remove junction: %v, output: %s", err, string(output))
	}

	return nil
}

// GetJunctionTarget gets the target path of a junction
func (m *Manager) GetJunctionTarget(path string) (string, error) {
	if !m.IsJunction(path) {
		return "", fmt.Errorf("path is not a junction")
	}

	// Use fsutil to get junction target (more reliable than dir command)
	cmd := exec.Command("fsutil", "reparsepoint", "query", path)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	// Parse the output to extract the target path
	// The output format includes "Substitute Name: <target_path>"
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "Substitute Name:") {
			// Extract the path after "Substitute Name:"
			parts := strings.SplitN(line, "Substitute Name:", 2)
			if len(parts) > 1 {
				target := strings.TrimSpace(parts[1])
				// Remove the \??\ prefix if present
				if strings.HasPrefix(target, "\\??\\") {
					target = target[4:]
				}
				return target, nil
			}
		}
	}

	return "", fmt.Errorf("could not parse junction target")
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
	tempFile := filepath.Join(path, ".ue-git-manager-test")
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
	cmd := exec.Command(uat, "BuildPlugin",
		fmt.Sprintf("-Plugin=%s", uplugin),
		fmt.Sprintf("-Package=%s", buildOut),
		"-Rocket",
		"-TargetPlatforms=Win64")

	// Set working directory to the engine directory for proper UAT execution
	cmd.Dir = enginePath

	// Debug: print the command being executed
	fmt.Printf("Executing: \"%s\" BuildPlugin -Plugin=\"%s\" -Package=\"%s\" -Rocket -TargetPlatforms=Win64\n",
		uat, uplugin, buildOut)
	fmt.Printf("Working directory: %s\n", enginePath)

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
