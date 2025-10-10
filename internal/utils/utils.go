package utils

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/fatih/color"
)

// Manager handles utility functions
type Manager struct{}

// IsRunningAsAdmin checks if the application is running with administrator privileges
func (m *Manager) IsRunningAsAdmin() bool {
	return IsRunningAsAdmin()
}

// RequestAdminElevation shows a message asking the user to run as administrator
func (m *Manager) RequestAdminElevation() {
	RequestAdminElevation()
}

// New creates a new utils manager
func New() *Manager {
	return &Manager{}
}

// Confirm asks the user for confirmation
func Confirm(message string) bool {
	fmt.Printf("%s (y/N): ", message)
	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}

// OpenURL opens a URL in the default browser
func OpenURL(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "linux":
		cmd = "xdg-open"
		args = []string{url}
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	return exec.Command(cmd, args...).Start()
}

// IsWindows checks if running on Windows
func IsWindows() bool {
	return runtime.GOOS == "windows"
}

// FormatPath formats a path for display
func FormatPath(path string) string {
	// Convert backslashes to forward slashes for consistency
	return strings.ReplaceAll(path, "\\", "/")
}

// TruncateString truncates a string to the specified length
func TruncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// PadString pads a string to the specified width
func PadString(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// Pause waits for user input
func Pause() {
	fmt.Print("Press Enter to continue...")
	bufio.NewReader(os.Stdin).ReadLine()
}

// IsRunningAsAdmin checks if the application is running with administrator privileges
func IsRunningAsAdmin() bool {
	// On Windows, we can check if we can write to a system directory
	// or check for specific privileges
	if !IsWindows() {
		return false // Non-Windows systems don't have UAC
	}

	// Try to create a file in a system directory that requires admin rights
	testPath := "C:\\Windows\\Temp\\ue-git-manager-admin-test.tmp"
	file, err := os.Create(testPath)
	if err != nil {
		return false
	}
	file.Close()
	os.Remove(testPath)
	return true
}

// RequestAdminElevation shows a message asking the user to run as administrator
func RequestAdminElevation() {
	fmt.Println()
	fmt.Println(color.New(color.FgRed, color.Bold).Sprint("⚠️  Administrator privileges required!"))
	fmt.Println()
	fmt.Println("This application needs administrator privileges to:")
	fmt.Println("• Create junctions in Unreal Engine directories")
	fmt.Println("• Modify plugin files in system locations")
	fmt.Println("• Access protected directories")
	fmt.Println()
	fmt.Println("Please:")
	fmt.Println("1. Close this application")
	fmt.Println("2. Right-click on UE-Git-Manager.exe")
	fmt.Println("3. Select 'Run as administrator'")
	fmt.Println("4. Try again")
	fmt.Println()
	Pause()
	os.Exit(1)
}

// ClearScreen clears the terminal screen
func (m *Manager) ClearScreen() {
	ClearScreen()
}

// ClearScreen clears the terminal screen
func ClearScreen() {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "cls")
	} else {
		cmd = exec.Command("clear")
	}
	cmd.Stdout = os.Stdout
	cmd.Run()
}
