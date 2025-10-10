package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// EngineInfo represents information about a discovered Unreal Engine installation
type EngineInfo struct {
	Path    string `json:"path"`
	Version string `json:"version"`
	Valid   bool   `json:"valid"`
}

// Manager handles engine discovery and validation
type Manager struct{}

// New creates a new engine manager
func New() *Manager {
	return &Manager{}
}

// DiscoverEngines discovers all Unreal Engine installations
func (m *Manager) DiscoverEngines(customRoots []string) ([]EngineInfo, error) {
	var engines []EngineInfo

	// Default Epic Games installation path
	defaultPath := `C:\Program Files\Epic Games`
	if _, err := os.Stat(defaultPath); err == nil {
		engines = append(engines, m.scanDirectory(defaultPath)...)
	}

	// Custom engine roots
	for _, root := range customRoots {
		if _, err := os.Stat(root); err == nil {
			engines = append(engines, m.scanDirectory(root)...)
		}
	}

	// Remove duplicates and validate
	uniqueEngines := make(map[string]EngineInfo)
	for _, eng := range engines {
		if eng.Valid {
			uniqueEngines[eng.Path] = eng
		}
	}

	var result []EngineInfo
	for _, eng := range uniqueEngines {
		result = append(result, eng)
	}

	// Sort engines by version (alphabetically/numerically)
	sort.Slice(result, func(i, j int) bool {
		return compareVersions(result[i].Version, result[j].Version) < 0
	})

	return result, nil
}

// scanDirectory recursively scans a directory for Unreal Engine installations
func (m *Manager) scanDirectory(root string) []EngineInfo {
	var engines []EngineInfo

	// Limit recursion depth to 2 as per spec
	m.scanDirectoryRecursive(root, 0, 2, &engines)
	return engines
}

// scanDirectoryRecursive recursively scans directories with depth limit
func (m *Manager) scanDirectoryRecursive(dir string, currentDepth, maxDepth int, engines *[]EngineInfo) {
	if currentDepth > maxDepth {
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		entryPath := filepath.Join(dir, entry.Name())

		// Check if this looks like an Unreal Engine directory
		if m.isUnrealEngineDirectory(entryPath) {
			version := m.extractVersion(entryPath)
			valid := m.validateEngine(entryPath)

			*engines = append(*engines, EngineInfo{
				Path:    entryPath,
				Version: version,
				Valid:   valid,
			})
		}

		// Continue scanning subdirectories
		m.scanDirectoryRecursive(entryPath, currentDepth+1, maxDepth, engines)
	}
}

// isUnrealEngineDirectory checks if a directory looks like an Unreal Engine installation
func (m *Manager) isUnrealEngineDirectory(path string) bool {
	// Check for UE_* pattern in directory name
	dirName := filepath.Base(path)
	matched, _ := regexp.MatchString(`^UE_\d+\.\d+`, dirName)
	return matched
}

// extractVersion extracts the version from the directory name or Build.version file
func (m *Manager) extractVersion(path string) string {
	// First try to extract from directory name
	dirName := filepath.Base(path)
	re := regexp.MustCompile(`UE_(\d+\.\d+)`)
	matches := re.FindStringSubmatch(dirName)
	if len(matches) > 1 {
		return matches[1]
	}

	// Fallback to Build.version file
	buildVersionPath := filepath.Join(path, "Engine", "Build", "Build.version")
	if data, err := os.ReadFile(buildVersionPath); err == nil {
		var buildInfo struct {
			MajorVersion int `json:"MajorVersion"`
			MinorVersion int `json:"MinorVersion"`
		}
		if json.Unmarshal(data, &buildInfo) == nil {
			return fmt.Sprintf("%d.%d", buildInfo.MajorVersion, buildInfo.MinorVersion)
		}
	}

	return "unknown"
}

// validateEngine validates that a directory is a proper Unreal Engine installation
func (m *Manager) validateEngine(path string) bool {
	// Check for the required UnrealEditor.exe
	editorPath := filepath.Join(path, "Engine", "Binaries", "Win64", "UnrealEditor.exe")
	_, err := os.Stat(editorPath)
	return err == nil
}

// GetPluginPath returns the plugins directory path for an engine
func (m *Manager) GetPluginPath(enginePath string) string {
	return filepath.Join(enginePath, "Engine", "Plugins")
}

// GetStockGitPluginPath returns the path to the stock Git source control plugin
func (m *Manager) GetStockGitPluginPath(enginePath string) string {
	return filepath.Join(enginePath, "Engine", "Plugins", "Developer", "GitSourceControl")
}

// CheckPluginCollision checks if there's a collision between stock and PB Git plugins
func (m *Manager) CheckPluginCollision(enginePath string) bool {
	stockPluginPath := m.GetStockGitPluginPath(enginePath)
	stockUPluginPath := filepath.Join(stockPluginPath, "GitSourceControl.uplugin")

	// Check if stock plugin exists and has the same name as PB plugin
	_, err := os.Stat(stockUPluginPath)
	return err == nil
}

// DisableStockPlugin disables the stock Git plugin by renaming its .uplugin file
func (m *Manager) DisableStockPlugin(enginePath string) error {
	stockPluginPath := m.GetStockGitPluginPath(enginePath)
	stockUPluginPath := filepath.Join(stockPluginPath, "GitSourceControl.uplugin")
	disabledPath := filepath.Join(stockPluginPath, "GitSourceControl.uplugin.disabled")

	return os.Rename(stockUPluginPath, disabledPath)
}

// EnableStockPlugin re-enables the stock Git plugin by restoring its .uplugin file
func (m *Manager) EnableStockPlugin(enginePath string) error {
	stockPluginPath := m.GetStockGitPluginPath(enginePath)
	stockUPluginPath := filepath.Join(stockPluginPath, "GitSourceControl.uplugin")
	disabledPath := filepath.Join(stockPluginPath, "GitSourceControl.uplugin.disabled")

	// Check if the disabled file exists
	if _, err := os.Stat(disabledPath); os.IsNotExist(err) {
		return fmt.Errorf("disabled plugin file not found")
	}

	return os.Rename(disabledPath, stockUPluginPath)
}

// IsStockPluginDisabled checks if the stock Git plugin is disabled
func (m *Manager) IsStockPluginDisabled(enginePath string) bool {
	stockPluginPath := m.GetStockGitPluginPath(enginePath)
	disabledPath := filepath.Join(stockPluginPath, "GitSourceControl.uplugin.disabled")
	_, err := os.Stat(disabledPath)
	return err == nil
}

// GetStockPluginStatus returns the current status of the stock Git plugin
func (m *Manager) GetStockPluginStatus(enginePath string) string {
	stockPluginPath := m.GetStockGitPluginPath(enginePath)
	stockUPluginPath := filepath.Join(stockPluginPath, "GitSourceControl.uplugin")
	disabledPath := filepath.Join(stockPluginPath, "GitSourceControl.uplugin.disabled")

	// Check if stock plugin is enabled
	if _, err := os.Stat(stockUPluginPath); err == nil {
		return "enabled"
	}

	// Check if stock plugin is disabled
	if _, err := os.Stat(disabledPath); err == nil {
		return "disabled"
	}

	// Plugin not found at all
	return "not_found"
}

// compareVersions compares two version strings (e.g., "5.3", "5.4", "5.5")
// Returns -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
func compareVersions(v1, v2 string) int {
	// Split versions by dots
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	// Compare each part numerically
	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var num1, num2 int
		var err1, err2 error

		if i < len(parts1) {
			num1, err1 = strconv.Atoi(parts1[i])
		}
		if i < len(parts2) {
			num2, err2 = strconv.Atoi(parts2[i])
		}

		// If either conversion failed, fall back to string comparison
		if err1 != nil || err2 != nil {
			if i < len(parts1) && i < len(parts2) {
				return strings.Compare(parts1[i], parts2[i])
			}
			if i < len(parts1) {
				return 1
			}
			return -1
		}

		if num1 < num2 {
			return -1
		}
		if num1 > num2 {
			return 1
		}
	}

	return 0
}
