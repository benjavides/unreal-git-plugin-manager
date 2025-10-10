package main

import (
	"fmt"
	"os"
	"path/filepath"

	"ue-git-plugin-manager/internal/config"
	"ue-git-plugin-manager/internal/detection"
	"ue-git-plugin-manager/internal/engine"
	"ue-git-plugin-manager/internal/git"
	"ue-git-plugin-manager/internal/menu"
	"ue-git-plugin-manager/internal/plugin"
	"ue-git-plugin-manager/internal/utils"
)

func main() {
	// Get the directory where the executable is located
	exePath, err := os.Executable()
	if err != nil {
		fmt.Printf("Error getting executable path: %v\n", err)
		os.Exit(1)
	}
	exeDir := filepath.Dir(exePath)

	// Change to the executable directory to ensure all relative paths work correctly
	originalDir, err := os.Getwd()
	if err != nil {
		fmt.Printf("Warning: Could not get current directory: %v\n", err)
	} else {
		// Only change directory if we're not already in the executable directory
		if originalDir != exeDir {
			if err := os.Chdir(exeDir); err != nil {
				fmt.Printf("Warning: Could not change to executable directory: %v\n", err)
				fmt.Printf("Current directory: %s\n", originalDir)
				fmt.Printf("Executable directory: %s\n", exeDir)
			}
		}
	}

	// Initialize the application
	configMgr := config.New(exeDir)
	baseDir := configMgr.GetBaseDir()

	app := &Application{
		ExeDir:    exeDir,
		Config:    configMgr,
		Git:       git.NewWithBaseDir(exeDir, baseDir),
		Engine:    engine.New(),
		Plugin:    plugin.New(exeDir),
		Utils:     utils.New(),
		Detection: detection.NewWithBaseDir(exeDir, baseDir),
	}

	// Note: Admin privileges are not required for junction creation on modern Windows

	// Note: No relocation check needed since we now use a fixed base directory
	// based on the user's config directory, which doesn't change with executable location

	// Run the main menu
	if err := menu.Run(app); err != nil {
		fmt.Printf("Error running application: %v\n", err)
		os.Exit(1)
	}
}

// Application holds all the components
type Application struct {
	ExeDir    string
	Config    *config.Manager
	Git       *git.Manager
	Engine    *engine.Manager
	Plugin    *plugin.Manager
	Utils     *utils.Manager
	Detection *detection.Detector
}

// GetConfig returns the config manager
func (app *Application) GetConfig() *config.Manager {
	return app.Config
}

// GetGit returns the git manager
func (app *Application) GetGit() *git.Manager {
	return app.Git
}

// GetEngine returns the engine manager
func (app *Application) GetEngine() *engine.Manager {
	return app.Engine
}

// GetPlugin returns the plugin manager
func (app *Application) GetPlugin() *plugin.Manager {
	return app.Plugin
}

// GetUtils returns the utils manager
func (app *Application) GetUtils() *utils.Manager {
	return app.Utils
}

// GetDetection returns the detection manager
func (app *Application) GetDetection() *detection.Detector {
	return app.Detection
}

// GetBaseDir returns the base directory for application data
func (app *Application) GetBaseDir() string {
	return app.Config.GetBaseDir()
}
