package menu

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ue-git-plugin-manager/internal/config"
	"ue-git-plugin-manager/internal/detection"
	"ue-git-plugin-manager/internal/engine"
	"ue-git-plugin-manager/internal/git"
	"ue-git-plugin-manager/internal/plugin"
	"ue-git-plugin-manager/internal/projectconfig"
	"ue-git-plugin-manager/internal/utils"

	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
)

// Application interface for dependency injection
type Application interface {
	GetConfig() *config.Manager
	GetGit() *git.Manager
	GetEngine() *engine.Manager
	GetPlugin() *plugin.Manager
	GetUtils() *utils.Manager
	GetDetection() *detection.Detector
}

// Run starts the main menu system
func Run(app Application) error {
	for {
		config, err := app.GetConfig().Load()
		if err != nil {
			// If no config exists, create a default one
			if !app.GetConfig().Exists() {
				config = app.GetConfig().CreateDefault()
				if err := app.GetConfig().Save(config); err != nil {
					return fmt.Errorf("failed to create default config: %v", err)
				}
			} else {
				return fmt.Errorf("failed to load config: %v", err)
			}
		}

		choice, err := showMainMenu(app, config)
		if err != nil {
			if err == promptui.ErrInterrupt {
				return nil // User pressed Ctrl+C
			}
			return err
		}

		switch choice {
		case "What is this?":
			app.GetUtils().ClearScreen()
			ShowWhatIsThis()
			utils.Pause()
			app.GetUtils().ClearScreen()
		case "Edit Setup":
			app.GetUtils().ClearScreen()
			if err := runEditSetup(app, config); err != nil {
				fmt.Printf("Error in edit setup: %v\n", err)
				utils.Pause()
			}
			app.GetUtils().ClearScreen()
		case "Configure project":
			app.GetUtils().ClearScreen()
			if err := runProjectConfigurator(app); err != nil {
				fmt.Printf("Error configuring project: %v\n", err)
				utils.Pause()
			}
			app.GetUtils().ClearScreen()
		case "Settings":
			app.GetUtils().ClearScreen()
			if err := runSettings(app, config); err != nil {
				fmt.Printf("Error in settings: %v\n", err)
				utils.Pause()
			}
			app.GetUtils().ClearScreen()
		case "Quit":
			return nil
		}
	}
}

// showMainMenu displays the main menu
func showMainMenu(app Application, config *config.Config) (string, error) {
	// Show status of managed engines
	fmt.Println(color.New(color.FgCyan, color.Bold).Sprint("🎮 UE Git Plugin Manager - Main Menu"))
	fmt.Println()

	// Use detection system to show current status
	summary, err := app.GetDetection().GetSimpleSetupSummary(config.CustomEngineRoots, config.DefaultRemoteBranch)
	if err != nil {
		fmt.Printf("Warning: Could not detect setup status: %v\n", err)
		fmt.Println()
	} else {
		fmt.Println(summary)
	}

	items := []string{
		"What is this?",
		"Edit Setup",
		"Configure project",
		"Settings",
		"Quit",
	}

	prompt := promptui.Select{
		Label:    "Select an option",
		Items:    items,
		Size:     10,
		HideHelp: true,
		Stdout:   &utils.BellSkipper{},
	}

	_, result, err := prompt.Run()
	return result, err
}

// runCheckSetupStatus shows detailed setup status
func runCheckSetupStatus(app Application, config *config.Config) error {
	fmt.Println(color.New(color.FgCyan, color.Bold).Sprint("🔍 Checking Setup Status"))
	fmt.Println()

	// Get detailed setup status
	statuses, err := app.GetDetection().DetectSetupStatus(config.CustomEngineRoots)
	if err != nil {
		return fmt.Errorf("failed to detect setup status: %v", err)
	}

	if len(statuses) == 0 {
		fmt.Println("No Unreal Engine installations found.")
		utils.Pause()
		return nil
	}

	// Show detailed status for each engine
	for _, status := range statuses {
		fmt.Printf("Engine %s (%s):\n", status.EngineVersion, status.EnginePath)

		if status.IsSetupComplete {
			fmt.Println(color.New(color.FgGreen).Sprint("  ✅ Setup Complete"))
		} else if status.IsNeverSetUp {
			fmt.Println(color.New(color.FgBlue).Sprint("  ℹ️  Not Set Up"))
		} else if status.IsBroken {
			fmt.Println(color.New(color.FgYellow).Sprint("  ⚠️  Setup Broken"))
		} else {
			fmt.Println(color.New(color.FgRed).Sprint("  ❌ Setup Incomplete"))
		}

		// Show individual status
		fmt.Printf("  - Worktree: %s\n", getStatusIcon(status.WorktreeExists))
		fmt.Printf("  - Junction: %s\n", getStatusIcon(status.JunctionExists))
		if status.JunctionExists {
			fmt.Printf("  - Junction Valid: %s\n", getStatusIcon(status.JunctionValid))
		}
		fmt.Printf("  - Binaries: %s\n", getStatusIcon(status.BinariesExist))
		fmt.Printf("  - Stock Plugin: %s\n", GetStockPluginStatusIcon(status.StockPluginStatus))

		// Only show issues for broken setups, not for engines that were never set up
		if status.IsBroken && len(status.Issues) > 0 {
			fmt.Println("  Issues:")
			for _, issue := range status.Issues {
				fmt.Printf("    - %s\n", issue)
			}
		}
		fmt.Println()
	}

	// Show engines that need setup
	needingSetup, err := app.GetDetection().FindEnginesNeedingSetup(config.CustomEngineRoots)
	if err == nil && len(needingSetup) > 0 {
		fmt.Println(color.New(color.FgYellow).Sprint("⚠️  Engines needing setup:"))
		for _, status := range needingSetup {
			fmt.Printf("  - UE %s: %s\n", status.EngineVersion, strings.Join(status.Issues, ", "))
		}
		fmt.Println()
	}

	utils.Pause()
	return nil
}

// getStatusIcon returns an icon for a boolean status
func getStatusIcon(status bool) string {
	if status {
		return "✅ Yes"
	}
	return "❌ No"
}

// GetStockPluginStatusIcon returns an icon for stock plugin status
func GetStockPluginStatusIcon(status string) string {
	switch status {
	case "enabled":
		return "❌ Enabled (conflict risk)"
	case "disabled":
		return "✅ Disabled (correct)"
	case "not_found":
		return "❌ Not found"
	default:
		return "❓ Unknown"
	}
}

// runUpdate handles the update flow
func runUpdate(app Application, config *config.Config) error {
	fmt.Println(color.New(color.FgCyan, color.Bold).Sprint("🔄 Checking for updates..."))
	fmt.Println()

	// Fetch latest changes
	if err := app.GetGit().FetchAll(); err != nil {
		return fmt.Errorf("failed to fetch updates: %v", err)
	}

	// Check each managed engine for updates
	var updatesAvailable []git.UpdateInfo
	for _, eng := range config.Engines {
		updateInfo, err := app.GetGit().GetUpdateInfo(eng.EngineVersion, config.DefaultRemoteBranch)
		if err != nil {
			fmt.Printf("❌ Failed to check updates for UE %s: %v\n", eng.EngineVersion, err)
			continue
		}

		if updateInfo.CommitsAhead > 0 {
			updatesAvailable = append(updatesAvailable, *updateInfo)
		}
	}

	if len(updatesAvailable) == 0 {
		fmt.Println("✅ All engines are up to date!")
		utils.Pause()
		return nil
	}

	// Show available updates
	fmt.Printf("📦 %d engine(s) have updates available:\n\n", len(updatesAvailable))
	for _, update := range updatesAvailable {
		fmt.Printf("UE %s — %d commits available\n", update.EngineVersion, update.CommitsAhead)
		fmt.Printf("Latest: %s  [Open in browser]\n", update.RemoteSHA[:8])
		fmt.Printf("Compare: %s...%s  [Open diff]\n", update.LocalSHA[:8], update.RemoteSHA[:8])
		fmt.Println()
	}

	if !utils.Confirm("Would you like to update now?") {
		return nil
	}

	// Perform updates
	fmt.Println("🔄 Updating engines...")
	for _, update := range updatesAvailable {
		fmt.Printf("Updating UE %s... ", update.EngineVersion)
		if err := app.GetGit().UpdateWorktree(update.EngineVersion, config.DefaultRemoteBranch); err != nil {
			fmt.Printf("❌ Failed: %v\n", err)
			continue
		}
		fmt.Printf("✅ Done\n")

		// Ensure stock plugin is disabled before rebuild
		// Find engine path for this version
		var enginePath string
		for _, e := range config.Engines {
			if e.EngineVersion == update.EngineVersion {
				enginePath = e.EnginePath
				break
			}
		}
		if app.GetEngine().CheckPluginCollision(enginePath) {
			if err := app.GetEngine().DisableStockPlugin(enginePath); err != nil {
				fmt.Printf("❌ %v\n", err)
				continue
			}
		}

		// Rebuild binaries for this engine
		wt := app.GetGit().GetWorktreePath(update.EngineVersion)
		fmt.Printf("Compiling plugin for UE %s... ", update.EngineVersion)
		if err := app.GetPlugin().BuildForEngine(enginePath, wt); err != nil {
			fmt.Printf("❌ %v\n", err)
		} else {
			fmt.Printf("✅\n")
		}
	}

	fmt.Println()
	fmt.Println("🎉 Updates completed!")
	utils.Pause()
	return nil
}

// runUninstall handles the uninstall flow
func runUninstall(app Application, config *config.Config) error {
	fmt.Println(color.New(color.FgRed, color.Bold).Sprint("🗑️  Uninstall UE Git Plugin Manager"))
	fmt.Println()
	fmt.Println("This will remove all plugin links and worktrees.")
	fmt.Println("Your Unreal Engine installations will not be affected.")
	fmt.Println()

	if !utils.Confirm("Are you sure you want to uninstall?") {
		return nil
	}

	fmt.Println("🔄 Uninstalling...")

	// Remove junctions and restore stock plugins
	for _, eng := range config.Engines {
		fmt.Printf("Cleaning up UE %s...\n", eng.EngineVersion)

		// Remove junction
		fmt.Printf("  Removing junction... ")
		pluginLinkPath := app.GetPlugin().GetPluginLinkPath(eng.EnginePath)
		if err := app.GetPlugin().RemoveJunction(pluginLinkPath); err != nil {
			fmt.Printf("❌ Failed: %v\n", err)
		} else {
			fmt.Printf("✅ Done\n")
		}

		// Restore stock plugin if we disabled it
		if eng.StockPluginDisabledByTool {
			fmt.Printf("  Restoring stock Git plugin... ")
			if err := app.GetEngine().EnableStockPlugin(eng.EnginePath); err != nil {
				fmt.Printf("❌ Failed: %v\n", err)
			} else {
				fmt.Printf("✅ Done\n")
			}
		} else {
			fmt.Printf("  Stock plugin was not disabled by tool, skipping restoration\n")
		}

		// Remove worktree
		fmt.Printf("  Removing worktree... ")
		if err := app.GetGit().RemoveWorktree(eng.EngineVersion); err != nil {
			fmt.Printf("❌ Failed: %v\n", err)
		} else {
			fmt.Printf("✅ Done\n")
		}

		fmt.Printf("✅ UE %s cleanup completed\n", eng.EngineVersion)
	}

	// Remove origin repository if no worktrees remain
	if err := app.GetGit().RemoveOrigin(); err != nil {
		fmt.Printf("Warning: Failed to remove origin repository: %v\n", err)
	}

	// Remove configuration
	configMgr := app.GetConfig()
	configPath := filepath.Join(configMgr.GetExeDir(), "config.json")
	if err := os.Remove(configPath); err != nil {
		fmt.Printf("Warning: Failed to remove configuration: %v\n", err)
	}

	fmt.Println()
	fmt.Println("🎉 Uninstall completed!")
	utils.Pause()
	return nil
}

// runAdvancedMenu handles the advanced menu
func runAdvancedMenu(app Application, config *config.Config) error {
	for {
		choice, err := showAdvancedMenu(app, config)
		if err != nil {
			if err == promptui.ErrInterrupt {
				return nil
			}
			return err
		}

		switch choice {
		case "Show configuration":
			app.GetUtils().ClearScreen()
			showConfiguration(config)
			utils.Pause()
			app.GetUtils().ClearScreen()
		case "Detailed Setup Status":
			app.GetUtils().ClearScreen()
			runDetailedSetupStatus(app, config)
		case "Change scan roots":
			app.GetUtils().ClearScreen()
			runManageCustomEnginePaths(app, config)
			app.GetUtils().ClearScreen()
		case "Change branch to track":
			app.GetUtils().ClearScreen()
			changeBranch(app, config)
			app.GetUtils().ClearScreen()
		case "Rescan engines":
			app.GetUtils().ClearScreen()
			rescanEngines(app, config)
			app.GetUtils().ClearScreen()
		case "Fix plugin collision":
			app.GetUtils().ClearScreen()
			fixPluginCollision(app, config)
			app.GetUtils().ClearScreen()
		case "Re-enable stock Git plugin":
			app.GetUtils().ClearScreen()
			reEnableStockPlugin(app, config)
			app.GetUtils().ClearScreen()
		case "Rebuild plugin for engine":
			app.GetUtils().ClearScreen()
			rebuildPluginForEngine(app, config)
			app.GetUtils().ClearScreen()
		case "Repair broken setup":
			app.GetUtils().ClearScreen()
			repairBrokenSetup(app, config)
			app.GetUtils().ClearScreen()
		case "Diagnostics":
			app.GetUtils().ClearScreen()
			runDiagnostics(app, config)
			app.GetUtils().ClearScreen()
		case "Open plugin repo in browser":
			utils.OpenURL("https://github.com/ProjectBorealis/UEGitPlugin")
		case "Back":
			return nil
		}
	}
}

// showAdvancedMenu displays the advanced menu
func showAdvancedMenu(app Application, config *config.Config) (string, error) {
	items := []string{
		"Show configuration",
		"Detailed Setup Status",
		"Change scan roots",
		"Change branch to track",
		"Rescan engines",
		"Fix plugin collision",
		"Re-enable stock Git plugin",
		"Rebuild plugin for engine",
		"Repair broken setup",
		"Diagnostics",
		"Open plugin repo in browser",
		"Back",
	}

	prompt := promptui.Select{
		Label: "Advanced Options",
		Items: items,
		Size:  10,
	}

	_, result, err := prompt.Run()
	return result, err
}

// runDetailedSetupStatus shows detailed setup status with debugging info
func runDetailedSetupStatus(app Application, config *config.Config) error {
	fmt.Println(color.New(color.FgCyan, color.Bold).Sprint("🔍 Detailed Setup Status"))
	fmt.Println()

	// Get detailed setup status
	statuses, err := app.GetDetection().DetectSetupStatus(config.CustomEngineRoots)
	if err != nil {
		return fmt.Errorf("failed to detect setup status: %v", err)
	}

	if len(statuses) == 0 {
		fmt.Println("No Unreal Engine installations found.")
		utils.Pause()
		return nil
	}

	// Show detailed status for each engine with debugging info
	for _, status := range statuses {
		fmt.Printf("Engine %s (%s):\n", status.EngineVersion, status.EnginePath)

		if status.IsSetupComplete {
			fmt.Println(color.New(color.FgGreen).Sprint("  ✅ Setup Complete"))
		} else if status.IsNeverSetUp {
			fmt.Println(color.New(color.FgBlue).Sprint("  ℹ️  Not Set Up"))
		} else if status.IsBroken {
			fmt.Println(color.New(color.FgYellow).Sprint("  ⚠️  Setup Broken"))
		} else {
			fmt.Println(color.New(color.FgRed).Sprint("  ❌ Setup Incomplete"))
		}

		// Show individual status with debugging
		fmt.Printf("  - Worktree: %s", getStatusIcon(status.WorktreeExists))
		if status.WorktreeExists {
			worktreePath := app.GetGit().GetWorktreePath(status.EngineVersion)
			fmt.Printf(" (%s)", worktreePath)
		}
		fmt.Println()

		fmt.Printf("  - Junction: %s", getStatusIcon(status.JunctionExists))
		if status.JunctionExists {
			pluginLinkPath := app.GetPlugin().GetPluginLinkPath(status.EnginePath)
			fmt.Printf(" (%s)", pluginLinkPath)
		}
		fmt.Println()

		if status.JunctionExists {
			fmt.Printf("  - Junction Valid: %s", getStatusIcon(status.JunctionValid))
			if !status.JunctionValid {
				// Show what the junction actually points to
				pluginLinkPath := app.GetPlugin().GetPluginLinkPath(status.EnginePath)
				if target, err := app.GetPlugin().GetJunctionTarget(pluginLinkPath); err == nil {
					fmt.Printf(" (points to: %s)", target)
				} else {
					fmt.Printf(" (could not get target: %v)", err)
				}
			}
			fmt.Println()
		}

		fmt.Printf("  - Binaries: %s", getStatusIcon(status.BinariesExist))
		if status.WorktreeExists {
			worktreePath := app.GetGit().GetWorktreePath(status.EngineVersion)
			binariesPath := filepath.Join(worktreePath, "Binaries", "Win64")
			fmt.Printf(" (%s)", binariesPath)
		}
		fmt.Println()

		fmt.Printf("  - Stock Plugin: %s\n", GetStockPluginStatusIcon(status.StockPluginStatus))

		// Show issues for broken setups
		if status.IsBroken && len(status.Issues) > 0 {
			fmt.Println("  Issues:")
			for _, issue := range status.Issues {
				fmt.Printf("    - %s\n", issue)
			}
		}
		fmt.Println()
	}

	utils.Pause()
	return nil
}

// runEditSetup shows detailed status and allows editing each engine setup
func runEditSetup(app Application, config *config.Config) error {
	fmt.Println(color.New(color.FgCyan, color.Bold).Sprint("🔧 Edit Setup"))
	fmt.Println()

	// Get detailed setup status
	statuses, err := app.GetDetection().DetectSetupStatus(config.CustomEngineRoots)
	if err != nil {
		return fmt.Errorf("failed to detect setup status: %v", err)
	}

	if len(statuses) == 0 {
		fmt.Println("No Unreal Engine installations found.")
		utils.Pause()
		return nil
	}

	// Show detailed status for each engine
	for _, status := range statuses {
		fmt.Printf("Engine %s (%s):\n", status.EngineVersion, status.EnginePath)

		if status.IsSetupComplete {
			fmt.Println(color.New(color.FgGreen).Sprint("  ✅ Setup Complete"))
		} else if status.IsNeverSetUp {
			fmt.Println(color.New(color.FgBlue).Sprint("  ℹ️  Not Set Up"))
		} else if status.IsBroken {
			fmt.Println(color.New(color.FgYellow).Sprint("  ⚠️  Setup Broken"))
		} else {
			fmt.Println(color.New(color.FgRed).Sprint("  ❌ Setup Incomplete"))
		}

		// Show individual status with debugging
		fmt.Printf("  - Worktree: %s", getStatusIcon(status.WorktreeExists))
		if status.WorktreeExists {
			worktreePath := app.GetGit().GetWorktreePath(status.EngineVersion)
			fmt.Printf(" (%s)", worktreePath)
		}
		fmt.Println()

		fmt.Printf("  - Junction: %s", getStatusIcon(status.JunctionExists))
		if status.JunctionExists {
			pluginLinkPath := app.GetPlugin().GetPluginLinkPath(status.EnginePath)
			fmt.Printf(" (%s)", pluginLinkPath)
		}
		fmt.Println()

		if status.JunctionExists {
			fmt.Printf("  - Junction Valid: %s", getStatusIcon(status.JunctionValid))
			if !status.JunctionValid {
				// Show what the junction actually points to
				pluginLinkPath := app.GetPlugin().GetPluginLinkPath(status.EnginePath)
				if target, err := app.GetPlugin().GetJunctionTarget(pluginLinkPath); err == nil {
					fmt.Printf(" (points to: %s)", target)
				} else {
					fmt.Printf(" (could not get target: %v)", err)
				}
			}
			fmt.Println()
		}

		fmt.Printf("  - Binaries: %s", getStatusIcon(status.BinariesExist))
		if status.WorktreeExists {
			worktreePath := app.GetGit().GetWorktreePath(status.EngineVersion)
			binariesPath := filepath.Join(worktreePath, "Binaries", "Win64")
			fmt.Printf(" (%s)", binariesPath)
		}
		fmt.Println()

		fmt.Printf("  - Stock Plugin: %s\n", GetStockPluginStatusIcon(status.StockPluginStatus))

		// Show issues for broken setups
		if status.IsBroken && len(status.Issues) > 0 {
			fmt.Println("  Issues:")
			for _, issue := range status.Issues {
				fmt.Printf("    - %s\n", issue)
			}
		}
		fmt.Println()
	}

	// Create menu options for each engine
	var engineOptions []string
	for _, status := range statuses {
		statusText := "Not Set Up"
		if status.IsSetupComplete {
			statusText = "Setup Complete"
		} else if status.IsBroken {
			statusText = "Setup Broken"
		}
		engineOptions = append(engineOptions, fmt.Sprintf("UE %s - %s", status.EngineVersion, statusText))
	}
	engineOptions = append(engineOptions, "Back")

	// Let user select an engine to edit
	prompt := promptui.Select{
		Label:    "Select an engine to edit",
		Items:    engineOptions,
		Size:     10,
		HideHelp: true,
		Stdout:   &utils.BellSkipper{},
	}

	_, selectedEngine, err := prompt.Run()
	if err != nil {
		if err == promptui.ErrInterrupt {
			return nil
		}
		return err
	}

	if selectedEngine == "Back" {
		return nil
	}

	// Find the selected engine
	var selectedStatus *detection.SetupStatus
	for i, status := range statuses {
		expectedText := fmt.Sprintf("UE %s -", status.EngineVersion)
		if strings.Contains(selectedEngine, expectedText) {
			selectedStatus = &statuses[i]
			break
		}
	}

	if selectedStatus == nil {
		return fmt.Errorf("selected engine not found")
	}

	// Show options for the selected engine
	return runEngineEditOptions(app, config, *selectedStatus)
}

// runEngineEditOptions shows options for editing a specific engine
func runEngineEditOptions(app Application, config *config.Config, status detection.SetupStatus) error {
	fmt.Printf("\nEditing UE %s:\n", status.EngineVersion)
	fmt.Printf("Path: %s\n", status.EnginePath)
	fmt.Println()

	var options []string
	if status.IsSetupComplete {
		options = []string{
			"Update Setup",
			"Uninstall Setup",
			"Back",
		}
	} else if status.IsBroken {
		options = []string{
			"Repair Setup",
			"Uninstall Setup",
			"Back",
		}
	} else {
		options = []string{
			"Install Setup",
			"Back",
		}
	}

	prompt := promptui.Select{
		Label:    "What would you like to do?",
		Items:    options,
		Size:     10,
		HideHelp: true,
		Stdout:   &utils.BellSkipper{},
	}

	_, choice, err := prompt.Run()
	if err != nil {
		if err == promptui.ErrInterrupt {
			return nil
		}
		return err
	}

	switch choice {
	case "Install Setup":
		return runSetupForEngine(app, config, status.EnginePath, status.EngineVersion)
	case "Update Setup":
		return runUpdateForEngine(app, config, status.EnginePath, status.EngineVersion)
	case "Repair Setup":
		return runRepairForEngine(app, config, status.EnginePath, status.EngineVersion)
	case "Uninstall Setup":
		return runUninstallForEngine(app, config, status.EnginePath, status.EngineVersion)
	case "Back":
		return nil
	}

	return nil
}

// runSettings shows the settings menu
func runSettings(app Application, config *config.Config) error {
	items := []string{
		"Manage Custom Engine Paths",
		"Change Branch to Track",
		"Open Plugin Repository",
		"Open Data Directory",
		"Back",
	}

	prompt := promptui.Select{
		Label:    "Settings",
		Items:    items,
		Size:     10,
		HideHelp: true,
		Stdout:   &utils.BellSkipper{},
	}

	_, choice, err := prompt.Run()
	if err != nil {
		if err == promptui.ErrInterrupt {
			return nil
		}
		return err
	}

	switch choice {
	case "Manage Custom Engine Paths":
		runManageCustomEnginePaths(app, config)
		return nil
	case "Change Branch to Track":
		changeBranch(app, config)
		return nil
	case "Open Plugin Repository":
		utils.OpenURL("https://github.com/ProjectBorealis/UEGitPlugin")
		return nil
	case "Open Data Directory":
		baseDir := app.GetConfig().GetBaseDir()
		utils.OpenURL("file:///" + strings.ReplaceAll(baseDir, "\\", "/"))
		return nil
	case "Back":
		return nil
	}

	return nil
}

// runManageCustomEnginePaths shows options to manage custom engine paths
func runManageCustomEnginePaths(app Application, config *config.Config) error {
	for {
		choice, err := showManageCustomEnginePathsMenu(app, config)
		if err != nil {
			if err == promptui.ErrInterrupt {
				return nil
			}
			return err
		}

		switch choice {
		case "Add Custom Engine Path":
			addCustomEnginePath(app, config)
		case "Delete Custom Engine Path":
			deleteCustomEnginePath(app, config)
		case "Back":
			return nil
		}
	}
}

// showManageCustomEnginePathsMenu displays the manage custom engine paths menu
func showManageCustomEnginePathsMenu(app Application, config *config.Config) (string, error) {
	fmt.Println(color.New(color.FgCyan, color.Bold).Sprint("🔍 Manage Custom Engine Paths"))
	fmt.Println()

	// Show current custom engine paths
	if len(config.CustomEngineRoots) == 0 {
		fmt.Println("No custom engine paths configured.")
	} else {
		fmt.Println("Current custom engine paths:")
		for i, root := range config.CustomEngineRoots {
			fmt.Printf("  %d. %s\n", i+1, root)
		}
	}
	fmt.Println()

	items := []string{
		"Add Custom Engine Path",
		"Delete Custom Engine Path",
		"Back",
	}

	prompt := promptui.Select{
		Label:    "Select an option",
		Items:    items,
		Size:     10,
		HideHelp: true,
		Stdout:   &utils.BellSkipper{},
	}

	_, result, err := prompt.Run()
	return result, err
}

// addCustomEnginePath allows the user to add a new custom engine path
func addCustomEnginePath(app Application, config *config.Config) {
	fmt.Println(color.New(color.FgCyan, color.Bold).Sprint("➕ Add Custom Engine Path"))
	fmt.Println()

	fmt.Print("Enter path to scan: ")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	newRoot := strings.TrimSpace(scanner.Text())

	// Handle quoted paths by removing quotes if present
	newRoot = strings.Trim(newRoot, "\"")

	if newRoot != "" {
		// Check if path already exists
		for _, existingRoot := range config.CustomEngineRoots {
			if existingRoot == newRoot {
				fmt.Printf("⚠️  Path '%s' is already configured.\n", newRoot)
				utils.Pause()
				return
			}
		}

		config.CustomEngineRoots = append(config.CustomEngineRoots, newRoot)
		if err := app.GetConfig().Save(config); err != nil {
			fmt.Printf("❌ Failed to save configuration: %v\n", err)
		} else {
			fmt.Printf("✅ Custom engine path added: %s\n", newRoot)
		}
	} else {
		fmt.Println("❌ Empty path not allowed.")
	}

	utils.Pause()
}

// deleteCustomEnginePath allows the user to delete an existing custom engine path
func deleteCustomEnginePath(app Application, config *config.Config) {
	fmt.Println(color.New(color.FgRed, color.Bold).Sprint("🗑️  Delete Custom Engine Path"))
	fmt.Println()

	if len(config.CustomEngineRoots) == 0 {
		fmt.Println("No custom engine paths to delete.")
		utils.Pause()
		return
	}

	fmt.Println("Select a custom engine path to delete:")
	fmt.Println()

	// Show current paths with numbers
	for i, root := range config.CustomEngineRoots {
		fmt.Printf("  %d. %s\n", i+1, root)
	}
	fmt.Println()

	fmt.Print("Enter path number to delete (or 0 to cancel): ")
	var choice int
	fmt.Scanln(&choice)

	if choice == 0 {
		return
	}

	if choice < 1 || choice > len(config.CustomEngineRoots) {
		fmt.Println("❌ Invalid selection.")
		utils.Pause()
		return
	}

	// Confirm deletion
	pathToDelete := config.CustomEngineRoots[choice-1]
	if !utils.Confirm(fmt.Sprintf("Are you sure you want to delete '%s'?", pathToDelete)) {
		return
	}

	// Remove the path from the slice
	config.CustomEngineRoots = append(config.CustomEngineRoots[:choice-1], config.CustomEngineRoots[choice:]...)

	if err := app.GetConfig().Save(config); err != nil {
		fmt.Printf("❌ Failed to save configuration: %v\n", err)
	} else {
		fmt.Printf("✅ Custom engine path deleted: %s\n", pathToDelete)
	}

	utils.Pause()
}

// runSetupForEngine sets up a specific engine
func runSetupForEngine(app Application, config *config.Config, enginePath, engineVersion string) error {
	fmt.Printf("Setting up UE %s...\n", engineVersion)

	// Ensure origin repository exists
	if !app.GetGit().IsOriginCloned() {
		fmt.Println("Cloning origin repository...")
		if err := app.GetGit().CloneOrigin(); err != nil {
			return fmt.Errorf("failed to clone origin repository: %v", err)
		}
	}

	// Create worktree
	if err := app.GetGit().CreateWorktree(engineVersion); err != nil {
		return fmt.Errorf("failed to create worktree: %v", err)
	}

	// Create junction (needed before building)
	worktreePath := app.GetGit().GetWorktreePath(engineVersion)
	if err := app.GetPlugin().CreateJunction(enginePath, worktreePath); err != nil {
		return fmt.Errorf("failed to create junction: %v", err)
	}

	// Always disable stock plugin before building to avoid name collision
	if app.GetEngine().CheckPluginCollision(enginePath) {
		if err := app.GetEngine().DisableStockPlugin(enginePath); err != nil {
			return fmt.Errorf("failed to disable stock plugin: %v", err)
		}
	}

	// Build plugin
	if err := app.GetPlugin().BuildForEngine(enginePath, worktreePath); err != nil {
		return fmt.Errorf("failed to build plugin: %v", err)
	}

	fmt.Printf("✅ UE %s setup complete!\n", engineVersion)
	utils.Pause()
	return nil
}

// runUpdateForEngine updates a specific engine
func runUpdateForEngine(app Application, config *config.Config, enginePath, engineVersion string) error {
	fmt.Printf("Checking for updates for UE %s...\n", engineVersion)

	// Check if there are updates available
	updateInfo, err := app.GetGit().GetUpdateInfo(engineVersion, config.DefaultRemoteBranch)
	if err != nil {
		return fmt.Errorf("failed to check for updates: %v", err)
	}

	if updateInfo.CommitsAhead == 0 {
		fmt.Printf("✅ UE %s is already up to date!\n", engineVersion)
		fmt.Printf("   Local commit: %s\n", updateInfo.LocalSHA[:8])
		utils.Pause()
		return nil
	}

	fmt.Printf("📥 Updates available: %d commits behind\n", updateInfo.CommitsAhead)
	fmt.Printf("   Local commit:  %s\n", updateInfo.LocalSHA[:8])
	fmt.Printf("   Remote commit: %s\n", updateInfo.RemoteSHA[:8])
	fmt.Printf("   Compare: %s\n", updateInfo.CompareURL)
	fmt.Println()

	// Update worktree
	fmt.Println("Updating worktree...")
	if err := app.GetGit().UpdateWorktree(engineVersion, config.DefaultRemoteBranch); err != nil {
		return fmt.Errorf("failed to update worktree: %v", err)
	}

	// Ensure stock plugin is disabled before rebuilding
	if app.GetEngine().CheckPluginCollision(enginePath) {
		if err := app.GetEngine().DisableStockPlugin(enginePath); err != nil {
			return fmt.Errorf("failed to disable stock plugin: %v", err)
		}
	}

	// Rebuild plugin
	fmt.Println("Rebuilding plugin...")
	worktreePath := app.GetGit().GetWorktreePath(engineVersion)
	if err := app.GetPlugin().BuildForEngine(enginePath, worktreePath); err != nil {
		return fmt.Errorf("failed to rebuild plugin: %v", err)
	}

	fmt.Printf("✅ UE %s updated successfully! (%d commits applied)\n", engineVersion, updateInfo.CommitsAhead)
	utils.Pause()
	return nil
}

// runRepairForEngine repairs a specific engine
func runRepairForEngine(app Application, config *config.Config, enginePath, engineVersion string) error {
	fmt.Printf("Repairing UE %s...\n", engineVersion)

	// Check what needs repair
	status := app.GetDetection().DetectEngineSetupStatus(enginePath, engineVersion)

	// Recreate worktree if missing
	if !status.WorktreeExists {
		if err := app.GetGit().CreateWorktree(engineVersion); err != nil {
			return fmt.Errorf("failed to create worktree: %v", err)
		}
	}

	// Recreate junction if missing or invalid (needed before building)
	if !status.JunctionExists || !status.JunctionValid {
		// Remove existing junction first
		pluginLinkPath := app.GetPlugin().GetPluginLinkPath(enginePath)
		app.GetPlugin().RemoveJunction(pluginLinkPath)

		// Create new junction
		if err := app.GetPlugin().CreateJunction(enginePath, app.GetGit().GetWorktreePath(engineVersion)); err != nil {
			return fmt.Errorf("failed to create junction: %v", err)
		}
	}

	// Ensure stock plugin is disabled before any rebuild
	if app.GetEngine().CheckPluginCollision(enginePath) {
		if err := app.GetEngine().DisableStockPlugin(enginePath); err != nil {
			return fmt.Errorf("failed to disable stock plugin: %v", err)
		}
	}

	// Rebuild plugin if binaries missing
	if !status.BinariesExist {
		worktreePath := app.GetGit().GetWorktreePath(engineVersion)
		if err := app.GetPlugin().BuildForEngine(enginePath, worktreePath); err != nil {
			return fmt.Errorf("failed to build plugin: %v", err)
		}
	}
	// Stock plugin already ensured disabled above

	fmt.Printf("✅ UE %s repaired successfully!\n", engineVersion)
	utils.Pause()
	return nil
}

// runUninstallForEngine uninstalls a specific engine
func runUninstallForEngine(app Application, config *config.Config, enginePath, engineVersion string) error {
	fmt.Printf("Uninstalling UE %s...\n", engineVersion)

	// Remove junction
	pluginLinkPath := app.GetPlugin().GetPluginLinkPath(enginePath)
	if err := app.GetPlugin().RemoveJunction(pluginLinkPath); err != nil {
		return fmt.Errorf("failed to remove junction: %v", err)
	}

	// Remove worktree
	if err := app.GetGit().RemoveWorktree(engineVersion); err != nil {
		return fmt.Errorf("failed to remove worktree: %v", err)
	}

	// Re-enable stock plugin
	if err := app.GetEngine().EnableStockPlugin(enginePath); err != nil {
		return fmt.Errorf("failed to re-enable stock plugin: %v", err)
	}

	fmt.Printf("✅ UE %s uninstalled successfully!\n", engineVersion)

	// Check if this was the last engine, and if so, remove origin repo
	statuses, err := app.GetDetection().DetectSetupStatus(config.CustomEngineRoots)
	if err == nil {
		remainingSetups := 0
		for _, status := range statuses {
			if status.IsSetupComplete {
				remainingSetups++
			}
		}

		if remainingSetups == 0 {
			fmt.Println("This was the last engine setup. Removing origin repository...")
			if err := app.GetGit().RemoveOrigin(); err != nil {
				fmt.Printf("Warning: Failed to remove origin repository: %v\n", err)
			} else {
				fmt.Println("✅ Origin repository removed.")
			}
		}
	}

	utils.Pause()
	return nil
}

// ShowWhatIsThis displays information about what this tool does
func ShowWhatIsThis() {
	fmt.Println(color.New(color.FgCyan, color.Bold).Sprint("ℹ️  What is UE Git Plugin Manager?"))
	fmt.Println()

	fmt.Println("This tool automates the setup of Git source control for multiple Unreal Engine installations.")
	fmt.Println("It was created because manually setting up Git source control in UE is complex and error-prone.")
	fmt.Println()

	fmt.Println(color.New(color.FgYellow, color.Bold).Sprint("Why was this tool created?"))
	fmt.Println("• Manual setup involves multiple complex steps that are easy to forget")
	fmt.Println("• This setup is typically done infrequently, so the process needs to be re-learned each time")
	fmt.Println("• Teams constantly change Unreal Engine versions, requiring updates and rebuilds")
	fmt.Println("• Setting up on multiple team members' computers is time-consuming and error-prone")
	fmt.Println()

	fmt.Println(color.New(color.FgYellow, color.Bold).Sprint("What does the setup do?"))
	fmt.Println("The setup process performs the following technical steps:")
	fmt.Println()
	fmt.Println("1. Git Repository Management:")
	fmt.Println("   • Clones the UEGitPlugin repository from GitHub")
	fmt.Println("   • Creates Git worktrees for each UE version (separate working directories)")
	fmt.Println("   • Fetches updates from the remote repository")
	fmt.Println("   • Why separate directories? Each UE version may need different plugin versions")
	fmt.Println("     or configurations, and worktrees allow us to maintain version-specific")
	fmt.Println("     branches while sharing the same Git history and updates.")
	fmt.Println()
	fmt.Println("2. Plugin Integration:")
	fmt.Println("   • Builds the plugin for each specific UE version")
	fmt.Println("   • Creates Windows junctions (symbolic links) from UE's plugin directory to the worktree")
	fmt.Println("   • Links: UE_5.X/Engine/Plugins/UEGitPlugin_PB → worktrees/UE_5.X")
	fmt.Println()
	fmt.Println("3. Plugin Configuration:")
	fmt.Println("   • Disables the stock Git plugin to prevent conflicts")
	fmt.Println("   • Ensures the custom plugin is properly registered")
	fmt.Println()
	fmt.Println("4. Multi-Version Support:")
	fmt.Println("   • Manages separate worktrees for each UE version")
	fmt.Println("   • Handles different binary requirements per UE version")
	fmt.Println("   • Maintains version-specific configurations")
	fmt.Println()

	fmt.Println(color.New(color.FgRed, color.Bold).Sprint("⚠️  Important Assumptions"))
	fmt.Println("This tool makes several assumptions that may change in the future:")
	fmt.Println()
	fmt.Println("1. Unreal Engine Structure:")
	fmt.Println("   • UE installs in 'C:\\Program Files\\Epic Games\\UE_X.X' format")
	fmt.Println("   • Plugin directory is at 'Engine/Plugins/UEGitPlugin_PB'")
	fmt.Println("   • Stock Git plugin is located at 'Engine/Plugins/Developer/GitSourceControl'")
	fmt.Println()
	fmt.Println("2. Plugin Repository:")
	fmt.Println("   • UEGitPlugin repository structure remains consistent")
	fmt.Println("   • Plugin builds with standard UE build system")
	fmt.Println("   • Binary output goes to 'Binaries/Win64/' directory")
	fmt.Println("   • Main DLL is named 'UnrealEditor-GitSourceControl.dll'")
	fmt.Println()
	fmt.Println("3. Windows System:")
	fmt.Println("   • 'fsutil reparsepoint query' command is available")
	fmt.Println("   • Git is installed and accessible via command line")
	fmt.Println("   • Junction creation works without admin privileges on modern Windows")
	fmt.Println()
	fmt.Println("4. File System:")
	fmt.Println("   • User has write access to UE installation directories")
	fmt.Println("   • User config directory is accessible (%APPDATA%\\ue-git-plugin-manager)")
	fmt.Println("   • No antivirus interference with junction creation")
	fmt.Println()
	fmt.Println("If any of these assumptions change, the tool may need updates.")
	fmt.Println("Check this list when troubleshooting setup issues.")
}

// showConfiguration displays the current configuration
func showConfiguration(config *config.Config) {
	fmt.Println(color.New(color.FgCyan, color.Bold).Sprint("📋 Current Configuration"))
	fmt.Println()
	fmt.Printf("Default Remote Branch: %s\n", config.DefaultRemoteBranch)
	fmt.Printf("Origin Directory: %s\n", config.OriginDir)
	fmt.Printf("Worktrees Directory: %s\n", config.WorktreesDir)
	fmt.Printf("Custom Engine Roots: %v\n", config.CustomEngineRoots)
	fmt.Printf("Managed Engines: %d\n", len(config.Engines))
	fmt.Println()

	for i, eng := range config.Engines {
		fmt.Printf("  %d. UE %s at %s\n", i+1, eng.EngineVersion, eng.EnginePath)
		fmt.Printf("     Worktree: %s\n", eng.WorktreeSubdir)
		fmt.Printf("     Branch: %s\n", eng.Branch)
		fmt.Printf("     Stock Plugin Disabled: %t\n", eng.StockPluginDisabledByTool)
		fmt.Println()
	}

	utils.Pause()
}

// changeBranch allows the user to change the tracked branch
func changeBranch(app Application, config *config.Config) {
	fmt.Println(color.New(color.FgCyan, color.Bold).Sprint("🌿 Change Tracked Branch"))
	fmt.Println()

	fmt.Printf("Current branch: %s\n", config.DefaultRemoteBranch)
	fmt.Print("Enter new branch name: ")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	newBranch := strings.TrimSpace(scanner.Text())

	if newBranch != "" {
		config.DefaultRemoteBranch = newBranch
		app.GetConfig().Save(config)
		fmt.Println("✅ Branch updated!")
	}

	utils.Pause()
}

// rescanEngines rescans for engines
func rescanEngines(app Application, config *config.Config) {
	fmt.Println(color.New(color.FgCyan, color.Bold).Sprint("🔍 Rescanning for Engines"))
	fmt.Println()

	engines, err := app.GetEngine().DiscoverEngines(config.CustomEngineRoots)
	if err != nil {
		fmt.Printf("❌ Failed to rescan engines: %v\n", err)
		utils.Pause()
		return
	}

	fmt.Printf("Found %d engines:\n", len(engines))
	for _, eng := range engines {
		status := ""
		if !eng.Valid {
			status = "❌ "
		}
		fmt.Printf("  %sUE %s at %s\n", status, eng.Version, eng.Path)
	}

	utils.Pause()
}

// reEnableStockPlugin re-enables the stock Git plugin for managed engines
func reEnableStockPlugin(app Application, config *config.Config) {
	fmt.Println(color.New(color.FgCyan, color.Bold).Sprint("🔧 Re-enable Stock Git Plugin"))
	fmt.Println()

	if len(config.Engines) == 0 {
		fmt.Println("No managed engines found.")
		utils.Pause()
		return
	}

	fmt.Println("Select an engine to re-enable the stock Git plugin:")
	fmt.Println()

	// Show engines with their current stock plugin status
	for i, eng := range config.Engines {
		status := app.GetEngine().GetStockPluginStatus(eng.EnginePath)
		statusIcon := "❌"
		statusText := "Unknown"

		switch status {
		case "enabled":
			statusIcon = "✅"
			statusText = "Enabled"
		case "disabled":
			statusIcon = "⚠️"
			statusText = "Disabled"
		case "not_found":
			statusIcon = "❌"
			statusText = "Not found"
		}

		fmt.Printf("  %d. UE %s at %s\n", i+1, eng.EngineVersion, eng.EnginePath)
		fmt.Printf("     Stock Git Plugin: %s %s\n", statusIcon, statusText)
		fmt.Println()
	}

	// Let user select an engine
	fmt.Print("Enter engine number (or 0 to cancel): ")
	var choice int
	fmt.Scanln(&choice)

	if choice < 1 || choice > len(config.Engines) {
		fmt.Println("Invalid selection.")
		utils.Pause()
		return
	}

	selectedEngine := config.Engines[choice-1]
	status := app.GetEngine().GetStockPluginStatus(selectedEngine.EnginePath)

	if status == "enabled" {
		fmt.Printf("Stock Git plugin for UE %s is already enabled.\n", selectedEngine.EngineVersion)
		utils.Pause()
		return
	}

	if status == "not_found" {
		fmt.Printf("Stock Git plugin for UE %s was not found. It may have been removed or never existed.\n", selectedEngine.EngineVersion)
		utils.Pause()
		return
	}

	// Re-enable the stock plugin
	fmt.Printf("Re-enabling stock Git plugin for UE %s... ", selectedEngine.EngineVersion)
	if err := app.GetEngine().EnableStockPlugin(selectedEngine.EnginePath); err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
	} else {
		fmt.Printf("✅ Done\n")

		// Update the config to reflect that we no longer disabled it
		selectedEngine.StockPluginDisabledByTool = false
		config.Engines[choice-1] = selectedEngine
		if err := app.GetConfig().Save(config); err != nil {
			fmt.Printf("Warning: Failed to update configuration: %v\n", err)
		}
	}

	utils.Pause()
}

// fixPluginCollision fixes plugin collisions
func fixPluginCollision(app Application, config *config.Config) {
	fmt.Println(color.New(color.FgCyan, color.Bold).Sprint("🔧 Fix Plugin Collision"))
	fmt.Println()

	for _, eng := range config.Engines {
		if app.GetEngine().CheckPluginCollision(eng.EnginePath) {
			fmt.Printf("UE %s: Collision detected\n", eng.EngineVersion)
			if utils.Confirm("Disable stock Git plugin?") {
				if err := app.GetEngine().DisableStockPlugin(eng.EnginePath); err != nil {
					fmt.Printf("❌ Failed: %v\n", err)
				} else {
					fmt.Printf("✅ Stock plugin disabled\n")
				}
			}
		} else {
			fmt.Printf("UE %s: No collision\n", eng.EngineVersion)
		}
	}

	utils.Pause()
}

// repairBrokenSetup attempts to repair broken setups
func repairBrokenSetup(app Application, config *config.Config) {
	fmt.Println(color.New(color.FgYellow, color.Bold).Sprint("🔧 Repair Broken Setup"))
	fmt.Println()

	// Find engines that need repair
	needingSetup, err := app.GetDetection().FindEnginesNeedingSetup(config.CustomEngineRoots)
	if err != nil {
		fmt.Printf("❌ Failed to detect engines needing repair: %v\n", err)
		utils.Pause()
		return
	}

	if len(needingSetup) == 0 {
		fmt.Println("✅ No engines need repair!")
		utils.Pause()
		return
	}

	fmt.Printf("Found %d engine(s) that need repair:\n\n", len(needingSetup))
	for i, status := range needingSetup {
		fmt.Printf("%d. UE %s (%s)\n", i+1, status.EngineVersion, status.EnginePath)
		fmt.Printf("   Issues: %s\n", strings.Join(status.Issues, ", "))
		fmt.Println()
	}

	if !utils.Confirm("Would you like to attempt to repair these engines?") {
		return
	}

	// Attempt to repair each engine
	for _, status := range needingSetup {
		fmt.Printf("Repairing UE %s...\n", status.EngineVersion)

		// Check if worktree exists, if not create it
		if !status.WorktreeExists {
			fmt.Printf("  Creating worktree... ")
			if err := app.GetGit().CreateWorktree(status.EngineVersion); err != nil {
				fmt.Printf("❌ Failed: %v\n", err)
				continue
			}
			fmt.Printf("✅ Done\n")
		}

		// Check if junction exists and is valid, if not create/fix it
		if !status.JunctionExists || !status.JunctionValid {
			fmt.Printf("  Creating/fixing junction... ")
			worktreePath := app.GetGit().GetWorktreePath(status.EngineVersion)
			if err := app.GetPlugin().CreateJunction(status.EnginePath, worktreePath); err != nil {
				fmt.Printf("❌ Failed: %v\n", err)
				continue
			}
			fmt.Printf("✅ Done\n")
		}

		// Check if binaries exist, if not rebuild them
		if !status.BinariesExist {
			fmt.Printf("  Rebuilding plugin... ")
			worktreePath := app.GetGit().GetWorktreePath(status.EngineVersion)
			if err := app.GetPlugin().BuildForEngine(status.EnginePath, worktreePath); err != nil {
				fmt.Printf("❌ Failed: %v\n", err)
				continue
			}
			fmt.Printf("✅ Done\n")
		}

		// Check if stock plugin needs to be disabled
		if status.StockPluginStatus == "enabled" {
			fmt.Printf("  Disabling stock plugin... ")
			if err := app.GetEngine().DisableStockPlugin(status.EnginePath); err != nil {
				fmt.Printf("❌ Failed: %v\n", err)
				continue
			}
			fmt.Printf("✅ Done\n")
		}

		fmt.Printf("✅ UE %s repair completed\n", status.EngineVersion)
		fmt.Println()
	}

	fmt.Println("🎉 Repair process completed!")
	utils.Pause()
}

// runDiagnostics runs system diagnostics
func runDiagnostics(app Application, config *config.Config) {
	fmt.Println(color.New(color.FgCyan, color.Bold).Sprint("🔍 System Diagnostics"))
	fmt.Println()

	// Check Git availability
	if app.GetGit().IsGitAvailable() {
		version, _ := app.GetGit().GetGitVersion()
		fmt.Printf("✅ Git: %s\n", version)
	} else {
		fmt.Println("❌ Git: Not available")
	}

	// Check origin repository
	if app.GetGit().IsOriginCloned() {
		fmt.Println("✅ Origin repository: Cloned")
	} else {
		fmt.Println("❌ Origin repository: Not cloned")
	}

	// Use detection system for comprehensive status
	fmt.Println()
	fmt.Println("Engine Setup Status:")
	statuses, err := app.GetDetection().DetectSetupStatus(config.CustomEngineRoots)
	if err != nil {
		fmt.Printf("❌ Failed to detect setup status: %v\n", err)
	} else {
		for _, status := range statuses {
			fmt.Printf("UE %s (%s):\n", status.EngineVersion, status.EnginePath)
			fmt.Printf("  Worktree: %s\n", getStatusIcon(status.WorktreeExists))
			fmt.Printf("  Junction: %s\n", getStatusIcon(status.JunctionExists))
			if status.JunctionExists {
				fmt.Printf("  Junction Valid: %s\n", getStatusIcon(status.JunctionValid))
			}
			fmt.Printf("  Binaries: %s\n", getStatusIcon(status.BinariesExist))
			fmt.Printf("  Stock Plugin: %s\n", GetStockPluginStatusIcon(status.StockPluginStatus))

			if len(status.Issues) > 0 {
				fmt.Println("  Issues:")
				for _, issue := range status.Issues {
					fmt.Printf("    - %s\n", issue)
				}
			}
			fmt.Println()
		}
	}

	// Show engines that need attention
	needingSetup, err := app.GetDetection().FindEnginesNeedingSetup(config.CustomEngineRoots)
	if err == nil && len(needingSetup) > 0 {
		fmt.Println("⚠️  Engines needing setup:")
		for _, status := range needingSetup {
			fmt.Printf("  - UE %s: %s\n", status.EngineVersion, strings.Join(status.Issues, ", "))
		}
		fmt.Println()
	}

	utils.Pause()
}

// rebuildPluginForEngine rebuilds the plugin for a selected engine
func rebuildPluginForEngine(app Application, config *config.Config) {
	fmt.Println(color.New(color.FgYellow, color.Bold).Sprint("🔨 Rebuild Plugin for Engine"))
	fmt.Println()

	if len(config.Engines) == 0 {
		fmt.Println("No managed engines found.")
		utils.Pause()
		return
	}

	fmt.Println("Select an engine to rebuild the plugin for:")
	fmt.Println()

	// Show engines
	for i, eng := range config.Engines {
		fmt.Printf("%d. UE %s at %s\n", i+1, eng.EngineVersion, eng.EnginePath)
	}

	fmt.Println()
	fmt.Print("Enter engine number (or 0 to cancel): ")
	var choice int
	fmt.Scanln(&choice)

	if choice < 1 || choice > len(config.Engines) {
		fmt.Println("Invalid selection.")
		utils.Pause()
		return
	}

	selectedEngine := config.Engines[choice-1]
	worktreePath := app.GetGit().GetWorktreePath(selectedEngine.EngineVersion)

	fmt.Printf("Rebuilding plugin for UE %s...\n", selectedEngine.EngineVersion)
	fmt.Printf("  Engine path: %s\n", selectedEngine.EnginePath)
	fmt.Printf("  Worktree path: %s\n", worktreePath)

	// Ensure stock plugin is disabled before manual rebuild
	if app.GetEngine().CheckPluginCollision(selectedEngine.EnginePath) {
		if err := app.GetEngine().DisableStockPlugin(selectedEngine.EnginePath); err != nil {
			fmt.Printf("❌ Failed to disable stock plugin: %v\n", err)
			utils.Pause()
			return
		}
	}

	if err := app.GetPlugin().BuildForEngine(selectedEngine.EnginePath, worktreePath); err != nil {
		fmt.Printf("❌ Failed to rebuild plugin: %v\n", err)
	} else {
		fmt.Printf("✅ Plugin rebuilt successfully for UE %s\n", selectedEngine.EngineVersion)
	}

	utils.Pause()
}

// runProjectConfigurator starts the Configure project wizard
func runProjectConfigurator(app Application) error {
	return projectconfig.RunWizard()
}
