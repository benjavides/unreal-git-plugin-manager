package projectconfig

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"ue-git-plugin-manager/internal/utils"

	"github.com/manifoldco/promptui"
)

// RunWizard orchestrates the Configure project flow
func RunWizard() error {
	fmt.Println("🔧 Configure Unreal Project")
	fmt.Println()
	fmt.Println("This wizard will help set up .gitattributes, .gitignore, and Unreal INI settings for your project.")
	fmt.Println()

	// Ask for project path
	projectPath, err := promptForPath()
	if err != nil {
		return err
	}

	root, err := DetectProjectRoot(projectPath)
	if err != nil {
		return fmt.Errorf("invalid project path: %w", err)
	}

	// Explain plugin binaries choice
	fmt.Println("Git handling for compiled plugin binaries:")
	fmt.Println("- Include binaries: helpful for artists without build tools, increases repo size")
	fmt.Println("- Ignore binaries: leaner repo; requires local compilation")
	includeBinaries, err := promptIncludeBinaries()
	if err != nil {
		return err
	}

	// .gitattributes
	if err := handleGitattributes(root); err != nil {
		return err
	}

	// .gitignore
	if err := handleGitignore(root, includeBinaries); err != nil {
		return err
	}

	// Git HTTP version configuration (required for Azure LFS)
	if err := configureGitHttpVersion(root); err != nil {
		return err
	}

	// INI settings
	answers, err := promptIniAnswers()
	if err != nil {
		return err
	}
	if err := ApplyIniSettings(root, answers); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("✅ Project configuration completed.")
	return nil
}

func promptForPath() (string, error) {
	fmt.Print("Enter or paste the project folder path: ")
	reader := bufio.NewReader(os.Stdin)
	p, _ := reader.ReadString('\n')
	p = filepath.Clean(strings.TrimSpace(p))
	if p == "." { // allow current dir quickly
		cwd, _ := os.Getwd()
		p = cwd
	}
	return p, nil
}

func promptIncludeBinaries() (bool, error) {
	prompt := promptui.Select{
		Label:  "Include compiled plugin binaries in Git?",
		Items:  []string{"Include (easier for artists)", "Ignore (leaner repo)"},
		Size:   5,
		Stdout: &utils.BellSkipper{},
	}
	_, choice, err := prompt.Run()
	if err != nil {
		return false, err
	}
	return strings.HasPrefix(choice, "Include"), nil
}

// configureGitHttpVersion sets git http.version to HTTP/1.1 (required for Azure LFS)
func configureGitHttpVersion(root string) error {
	// Check if this is a git repository
	gitDir := filepath.Join(root, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		// Not a git repository, skip this step
		return nil
	}

	// Run git config --local http.version HTTP/1.1
	cmd := exec.Command("git", "config", "--local", "http.version", "HTTP/1.1")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to configure git http.version: %v\nOutput: %s", err, string(output))
	}

	fmt.Println("✅ Configured git http.version to HTTP/1.1 (required for Azure LFS)")
	return nil
}
