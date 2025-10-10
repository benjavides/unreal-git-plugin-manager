package detection

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ue-git-manager/internal/engine"
	"ue-git-manager/internal/git"
	"ue-git-manager/internal/plugin"
)

// SetupStatus represents the current state of the setup for a specific engine
type SetupStatus struct {
	EngineVersion     string   `json:"engine_version"`
	EnginePath        string   `json:"engine_path"`
	IsSetupComplete   bool     `json:"is_setup_complete"`
	JunctionExists    bool     `json:"junction_exists"`
	JunctionValid     bool     `json:"junction_valid"`
	BinariesExist     bool     `json:"binaries_exist"`
	WorktreeExists    bool     `json:"worktree_exists"`
	StockPluginStatus string   `json:"stock_plugin_status"` // "enabled", "disabled", "not_found"
	Issues            []string `json:"issues"`
	IsNeverSetUp      bool     `json:"is_never_set_up"` // True if this engine was never set up
	IsBroken          bool     `json:"is_broken"`       // True if it was set up but is now broken
}

// Detector handles detection of current setup state
type Detector struct {
	exeDir  string
	baseDir string
	engine  *engine.Manager
	git     *git.Manager
	plugin  *plugin.Manager
}

// New creates a new detector
func New(exeDir string) *Detector {
	return &Detector{
		exeDir:  exeDir,
		baseDir: exeDir, // For backward compatibility
		engine:  engine.New(),
		git:     git.New(exeDir),
		plugin:  plugin.New(exeDir),
	}
}

// NewWithBaseDir creates a new detector with a specific base directory
func NewWithBaseDir(exeDir, baseDir string) *Detector {
	return &Detector{
		exeDir:  exeDir,
		baseDir: baseDir,
		engine:  engine.New(),
		git:     git.NewWithBaseDir(exeDir, baseDir),
		plugin:  plugin.New(exeDir),
	}
}

// DetectSetupStatus detects the current setup status for all discovered engines
func (d *Detector) DetectSetupStatus(customEngineRoots []string) ([]SetupStatus, error) {
	// Discover all engines
	engines, err := d.engine.DiscoverEngines(customEngineRoots)
	if err != nil {
		return nil, fmt.Errorf("failed to discover engines: %w", err)
	}

	var statuses []SetupStatus
	for _, eng := range engines {
		status := d.detectEngineSetupStatus(eng.Path, eng.Version)
		statuses = append(statuses, status)
	}

	return statuses, nil
}

// DetectEngineSetupStatus detects the setup status for a specific engine
func (d *Detector) DetectEngineSetupStatus(enginePath, engineVersion string) SetupStatus {
	return d.detectEngineSetupStatus(enginePath, engineVersion)
}

// detectEngineSetupStatus performs the actual detection for a single engine
func (d *Detector) detectEngineSetupStatus(enginePath, engineVersion string) SetupStatus {
	status := SetupStatus{
		EngineVersion:   engineVersion,
		EnginePath:      enginePath,
		IsSetupComplete: false,
		Issues:          []string{},
		IsNeverSetUp:    false,
		IsBroken:        false,
	}

	// Check if worktree exists
	worktreePath := d.git.GetWorktreePath(engineVersion)
	status.WorktreeExists = d.git.WorktreeExists(engineVersion)
	if !status.WorktreeExists {
		status.Issues = append(status.Issues, "Worktree does not exist")
	}

	// Check if junction exists
	pluginLinkPath := d.plugin.GetPluginLinkPath(enginePath)
	status.JunctionExists = d.plugin.JunctionExists(pluginLinkPath)
	if !status.JunctionExists {
		status.Issues = append(status.Issues, "Plugin junction does not exist")
	} else {
		// Check if junction is valid (points to correct worktree)
		status.JunctionValid = d.plugin.VerifyJunction(enginePath, worktreePath)
		if !status.JunctionValid {
			status.Issues = append(status.Issues, "Plugin junction points to incorrect location")
		}
	}

	// Check if binaries exist in worktree
	if status.WorktreeExists {
		binariesPath := filepath.Join(worktreePath, "Binaries", "Win64")
		status.BinariesExist = d.checkBinariesExist(binariesPath)
		if !status.BinariesExist {
			status.Issues = append(status.Issues, "Plugin binaries not found in worktree")
		}
	}

	// Check stock plugin status
	status.StockPluginStatus = d.engine.GetStockPluginStatus(enginePath)
	if status.StockPluginStatus == "enabled" {
		status.Issues = append(status.Issues, "Stock Git plugin is still enabled (may cause conflicts)")
	}

	// Determine if setup is complete
	status.IsSetupComplete = status.WorktreeExists &&
		status.JunctionExists &&
		status.JunctionValid &&
		status.BinariesExist &&
		status.StockPluginStatus != "enabled"

	// Determine if this engine was never set up vs. is broken
	// If nothing exists (no worktree, no junction), it was never set up
	if !status.WorktreeExists && !status.JunctionExists {
		status.IsNeverSetUp = true
	} else if !status.IsSetupComplete {
		// If some things exist but setup is incomplete, it's broken
		status.IsBroken = true
	}

	return status
}

// checkBinariesExist checks if the required plugin binaries exist
func (d *Detector) checkBinariesExist(binariesPath string) bool {
	// Check if the directory exists
	if _, err := os.Stat(binariesPath); err != nil {
		return false
	}

	// Check for the main plugin DLL (UE builds it as UnrealEditor-GitSourceControl.dll)
	mainDLL := filepath.Join(binariesPath, "UnrealEditor-GitSourceControl.dll")
	if _, err := os.Stat(mainDLL); err != nil {
		return false
	}

	// Check for other required files
	requiredFiles := []string{
		"UnrealEditor-GitSourceControl.dll",
		"UnrealEditor.modules",
		// Add other required files here
	}

	for _, file := range requiredFiles {
		filePath := filepath.Join(binariesPath, file)
		if _, err := os.Stat(filePath); err != nil {
			return false
		}
	}

	return true
}

// GetSetupSummary returns a summary of the current setup state
func (d *Detector) GetSetupSummary(customEngineRoots []string) (string, error) {
	statuses, err := d.DetectSetupStatus(customEngineRoots)
	if err != nil {
		return "", err
	}

	var summary strings.Builder
	summary.WriteString("🔍 Current Setup Status:\n\n")

	if len(statuses) == 0 {
		summary.WriteString("No Unreal Engine installations found.\n")
		return summary.String(), nil
	}

	for _, status := range statuses {
		summary.WriteString(fmt.Sprintf("Engine %s (%s):\n", status.EngineVersion, status.EnginePath))

		if status.IsSetupComplete {
			summary.WriteString("  ✅ Setup Complete\n")
		} else if status.IsNeverSetUp {
			summary.WriteString("  ℹ️  Not Set Up\n")
		} else if status.IsBroken {
			summary.WriteString("  ⚠️  Setup Broken\n")
		} else {
			summary.WriteString("  ❌ Setup Incomplete\n")
		}

		// Show individual status
		summary.WriteString(fmt.Sprintf("  - Worktree: %s\n", d.boolToStatus(status.WorktreeExists)))
		summary.WriteString(fmt.Sprintf("  - Junction: %s\n", d.boolToStatus(status.JunctionExists)))
		if status.JunctionExists {
			summary.WriteString(fmt.Sprintf("  - Junction Valid: %s\n", d.boolToStatus(status.JunctionValid)))
		}
		summary.WriteString(fmt.Sprintf("  - Binaries: %s\n", d.boolToStatus(status.BinariesExist)))
		summary.WriteString(fmt.Sprintf("  - Stock Plugin: %s\n", strings.Title(status.StockPluginStatus)))

		// Only show issues for broken setups, not for engines that were never set up
		if status.IsBroken && len(status.Issues) > 0 {
			summary.WriteString("  Issues:\n")
			for _, issue := range status.Issues {
				summary.WriteString(fmt.Sprintf("    - %s\n", issue))
			}
		}
		summary.WriteString("\n")
	}

	return summary.String(), nil
}

// GetSimpleSetupSummary returns a simplified summary for the main menu
func (d *Detector) GetSimpleSetupSummary(customEngineRoots []string) (string, error) {
	statuses, err := d.DetectSetupStatus(customEngineRoots)
	if err != nil {
		return "", err
	}

	var summary strings.Builder
	summary.WriteString("🔍 Detected Engines:\n\n")

	if len(statuses) == 0 {
		summary.WriteString("No Unreal Engine installations found.\n")
		return summary.String(), nil
	}

	for _, status := range statuses {
		statusIcon := "❌"
		statusText := "Not Set Up"

		if status.IsSetupComplete {
			statusIcon = "✅"
			statusText = "Setup Complete"
		} else if status.IsBroken {
			statusIcon = "⚠️"
			statusText = "Setup Broken"
		}

		summary.WriteString(fmt.Sprintf("%s UE %s - %s\n", statusIcon, status.EngineVersion, statusText))
		summary.WriteString(fmt.Sprintf("   %s\n\n", status.EnginePath))
	}

	return summary.String(), nil
}

// boolToStatus converts a boolean to a status string
func (d *Detector) boolToStatus(b bool) string {
	if b {
		return "✅ Yes"
	}
	return "❌ No"
}

// FindEnginesNeedingSetup returns engines that need setup or repair
func (d *Detector) FindEnginesNeedingSetup(customEngineRoots []string) ([]SetupStatus, error) {
	statuses, err := d.DetectSetupStatus(customEngineRoots)
	if err != nil {
		return nil, err
	}

	var needingSetup []SetupStatus
	for _, status := range statuses {
		if !status.IsSetupComplete {
			needingSetup = append(needingSetup, status)
		}
	}

	return needingSetup, nil
}

// FindEnginesWithIssues returns engines that have specific issues
func (d *Detector) FindEnginesWithIssues(customEngineRoots []string) ([]SetupStatus, error) {
	statuses, err := d.DetectSetupStatus(customEngineRoots)
	if err != nil {
		return nil, err
	}

	var withIssues []SetupStatus
	for _, status := range statuses {
		if len(status.Issues) > 0 {
			withIssues = append(withIssues, status)
		}
	}

	return withIssues, nil
}

// ValidateExistingSetup validates that an existing setup is still working
func (d *Detector) ValidateExistingSetup(enginePath, engineVersion string) error {
	status := d.DetectEngineSetupStatus(enginePath, engineVersion)

	if !status.IsSetupComplete {
		return fmt.Errorf("setup validation failed: %s", strings.Join(status.Issues, "; "))
	}

	return nil
}
