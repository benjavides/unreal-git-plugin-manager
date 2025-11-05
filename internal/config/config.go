package config

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"ue-git-plugin-manager/internal/utils"
)

// Config represents the application configuration
type Config struct {
	Version             int      `json:"version"`
	BaseDir             string   `json:"base_dir"`
	OriginDir           string   `json:"origin_dir"`
	WorktreesDir        string   `json:"worktrees_dir"`
	DefaultRemoteBranch string   `json:"default_remote_branch"`
	Engines             []Engine `json:"engines"`
	CustomEngineRoots   []string `json:"custom_engine_roots"`
	LastRunUTC          string   `json:"last_run_utc"`
}

// Engine represents a managed Unreal Engine installation
type Engine struct {
	EnginePath                string `json:"engine_path"`
	EngineVersion             string `json:"engine_version"`
	WorktreeSubdir            string `json:"worktree_subdir"`
	Branch                    string `json:"branch"`
	PluginLinkPath            string `json:"plugin_link_path"`
	StockPluginDisabledByTool bool   `json:"stock_plugin_disabled_by_tool"`
}

// Manager handles configuration operations
type Manager struct {
	exeDir     string
	baseDir    string
	configPath string
}

// New creates a new configuration manager
func New(exeDir string) *Manager {
	baseDir := getUserConfigDir()
	return &Manager{
		exeDir:     exeDir,
		baseDir:    baseDir,
		configPath: filepath.Join(baseDir, "config.json"),
	}
}

// getUserConfigDir returns the user's config directory for the application
// If the default path contains non-ASCII characters, uses a fallback location
// to prevent UBT/MSVC build failures
func getUserConfigDir() string {
	// Get the current user
	usr, err := user.Current()
	if err != nil {
		// Fallback to executable directory if we can't get user info
		exePath, _ := os.Executable()
		return filepath.Dir(exePath)
	}

	// Use the user's config directory
	// On Windows: %APPDATA%\ue-git-plugin-manager
	// On Linux/macOS: ~/.config/ue-git-plugin-manager
	defaultConfigDir := filepath.Join(usr.HomeDir, "AppData", "Roaming", "ue-git-plugin-manager")

	// Check if the default path contains non-ASCII characters
	// Unreal Build Tool and MSVC compiler fail when paths contain non-ASCII characters
	if utils.HasNonASCIICharacters(defaultConfigDir) {
		// Use fallback location: C:\ProgramData\ue-git-plugin-manager
		// Use filepath.Join with "C:\\" to ensure absolute path on Windows
		fallbackConfigDir := filepath.Join("C:\\", "ProgramData", "ue-git-plugin-manager")
		fmt.Printf("⚠️  Warning: Username contains non-ASCII characters.\n")
		fmt.Printf("   Default path: %s\n", defaultConfigDir)
		fmt.Printf("   Using fallback path: %s\n", fallbackConfigDir)
		fmt.Printf("   (This is required to prevent build failures with UBT/MSVC)\n\n")

		// Create the directory if it doesn't exist
		os.MkdirAll(fallbackConfigDir, 0755)
		return fallbackConfigDir
	}

	// Create the directory if it doesn't exist
	os.MkdirAll(defaultConfigDir, 0755)

	return defaultConfigDir
}

// GetExeDir returns the executable directory
func (m *Manager) GetExeDir() string {
	return m.exeDir
}

// GetBaseDir returns the base directory for the application data
func (m *Manager) GetBaseDir() string {
	return m.baseDir
}

// GetPossibleBaseDirs returns both the default and fallback base directories
// This is used for detection code to check both locations
func GetPossibleBaseDirs() []string {
	usr, err := user.Current()
	if err != nil {
		return []string{}
	}

	// Default path: %APPDATA%\ue-git-plugin-manager
	defaultPath := filepath.Join(usr.HomeDir, "AppData", "Roaming", "ue-git-plugin-manager")

	// Fallback path: C:\ProgramData\ue-git-plugin-manager
	// Use filepath.Join with "C:\\" to ensure absolute path on Windows
	fallbackPath := filepath.Join("C:\\", "ProgramData", "ue-git-plugin-manager")

	return []string{defaultPath, fallbackPath}
}

// Exists checks if the configuration file exists
func (m *Manager) Exists() bool {
	_, err := os.Stat(m.configPath)
	return !os.IsNotExist(err)
}

// Load loads the configuration from file
func (m *Manager) Load() (*Config, error) {
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	// Resolve relative paths
	config.BaseDir = m.resolvePath(config.BaseDir)
	config.OriginDir = m.resolvePath(config.OriginDir)
	config.WorktreesDir = m.resolvePath(config.WorktreesDir)

	return &config, nil
}

// Save saves the configuration to file
func (m *Manager) Save(config *Config) error {
	// Make a copy to avoid modifying the original
	saveConfig := *config

	// Convert absolute paths to relative where possible
	saveConfig.BaseDir = m.makeRelative(saveConfig.BaseDir)
	saveConfig.OriginDir = m.makeRelative(saveConfig.OriginDir)
	saveConfig.WorktreesDir = m.makeRelative(saveConfig.WorktreesDir)

	// Update last run time
	saveConfig.LastRunUTC = time.Now().UTC().Format(time.RFC3339)

	data, err := json.MarshalIndent(saveConfig, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(m.configPath, data, 0644)
}

// CreateDefault creates a default configuration
func (m *Manager) CreateDefault() *Config {
	return &Config{
		Version:             1,
		BaseDir:             m.baseDir,
		OriginDir:           "repo-origin",
		WorktreesDir:        "worktrees",
		DefaultRemoteBranch: "dev",
		Engines:             []Engine{},
		CustomEngineRoots:   []string{},
		LastRunUTC:          time.Now().UTC().Format(time.RFC3339),
	}
}

// AddEngine adds an engine to the configuration
func (m *Manager) AddEngine(config *Config, eng Engine) {
	config.Engines = append(config.Engines, eng)
}

// RemoveEngine removes an engine from the configuration
func (m *Manager) RemoveEngine(config *Config, enginePath string) {
	for i, eng := range config.Engines {
		if eng.EnginePath == enginePath {
			config.Engines = append(config.Engines[:i], config.Engines[i+1:]...)
			break
		}
	}
}

// GetEngineByPath gets an engine by its path
func (m *Manager) GetEngineByPath(config *Config, enginePath string) *Engine {
	for i, eng := range config.Engines {
		if eng.EnginePath == enginePath {
			return &config.Engines[i]
		}
	}
	return nil
}

// resolvePath resolves a path relative to the base directory
func (m *Manager) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(m.baseDir, path)
}

// makeRelative makes a path relative to the base directory if possible
func (m *Manager) makeRelative(path string) string {
	rel, err := filepath.Rel(m.baseDir, path)
	if err != nil || len(rel) >= len(path) {
		return path // Return original if can't make relative or if relative is longer
	}
	return rel
}
