package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// UpdateInfo represents information about available updates
type UpdateInfo struct {
	EngineVersion   string `json:"engine_version"`
	CommitsAhead    int    `json:"commits_ahead"`
	LocalSHA        string `json:"local_sha"`
	RemoteSHA       string `json:"remote_sha"`
	LatestCommitURL string `json:"latest_commit_url"`
	CompareURL      string `json:"compare_url"`
}

// Manager handles Git operations
type Manager struct {
	exeDir       string
	baseDir      string
	originDir    string
	worktreesDir string
}

// New creates a new Git manager
func New(exeDir string) *Manager {
	// For backward compatibility, we'll still accept exeDir but use it as baseDir
	// In practice, this should be called with the base directory from config
	return &Manager{
		exeDir:       exeDir,
		baseDir:      exeDir,
		originDir:    filepath.Join(exeDir, "repo-origin"),
		worktreesDir: filepath.Join(exeDir, "worktrees"),
	}
}

// NewWithBaseDir creates a new Git manager with a specific base directory
func NewWithBaseDir(exeDir, baseDir string) *Manager {
	return &Manager{
		exeDir:       exeDir,
		baseDir:      baseDir,
		originDir:    filepath.Join(baseDir, "repo-origin"),
		worktreesDir: filepath.Join(baseDir, "worktrees"),
	}
}

// IsGitAvailable checks if Git is available in PATH
func (m *Manager) IsGitAvailable() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// GetGitVersion returns the Git version
func (m *Manager) GetGitVersion() (string, error) {
	cmd := exec.Command("git", "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// CloneOrigin clones the UEGitPlugin repository
func (m *Manager) CloneOrigin() error {
	if m.IsOriginCloned() {
		return nil
	}

	cmd := exec.Command("git", "clone", "https://github.com/ProjectBorealis/UEGitPlugin", m.originDir)
	cmd.Dir = m.exeDir
	return cmd.Run()
}

// IsOriginCloned checks if the origin repository is cloned
func (m *Manager) IsOriginCloned() bool {
	gitDir := filepath.Join(m.originDir, ".git")
	_, err := os.Stat(gitDir)
	return err == nil
}

// GetDefaultBranch gets the default branch from the origin repository
func (m *Manager) GetDefaultBranch() (string, error) {
	cmd := exec.Command("git", "-C", m.originDir, "remote", "show", "origin")
	output, err := cmd.Output()
	if err != nil {
		return "dev", err // Fallback to dev
	}

	// Parse the output to find the HEAD branch
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "HEAD branch:") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				return parts[2], nil
			}
		}
	}

	return "dev", nil // Fallback to dev
}

// FetchAll fetches all remote changes
func (m *Manager) FetchAll() error {
	cmd := exec.Command("git", "-C", m.originDir, "fetch", "--all", "--prune")
	return cmd.Run()
}

// CreateEngineBranch creates a branch for a specific engine version
func (m *Manager) CreateEngineBranch(version, defaultBranch string) error {
	branchName := fmt.Sprintf("engine-%s", version)
	cmd := exec.Command("git", "-C", m.originDir, "branch", "--force", branchName, fmt.Sprintf("origin/%s", defaultBranch))
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to create engine branch: %v, output: %s", err, string(output))
	}
	return nil
}

// CreateWorktree creates a worktree for an engine version
func (m *Manager) CreateWorktree(version string) error {
	branchName := fmt.Sprintf("engine-%s", version)
	worktreePath := filepath.Join(m.worktreesDir, fmt.Sprintf("UE_%s", version))

	// Create the worktrees directory if it doesn't exist
	if err := os.MkdirAll(m.worktreesDir, 0755); err != nil {
		return fmt.Errorf("failed to create worktrees directory: %v", err)
	}

	cmd := exec.Command("git", "-C", m.originDir, "worktree", "add", worktreePath, branchName)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to create worktree: %v, output: %s", err, string(output))
	}

	// Verify the worktree was created
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		return fmt.Errorf("worktree directory was not created: %s", worktreePath)
	}

	return nil
}

// WorktreeExists checks if a worktree exists for the given version
func (m *Manager) WorktreeExists(version string) bool {
	worktreePath := filepath.Join(m.worktreesDir, fmt.Sprintf("UE_%s", version))
	_, err := os.Stat(worktreePath)
	return err == nil
}

// GetWorktreePath returns the path to a worktree
func (m *Manager) GetWorktreePath(version string) string {
	return filepath.Join(m.worktreesDir, fmt.Sprintf("UE_%s", version))
}

// GetUpdateInfo gets update information for a worktree
func (m *Manager) GetUpdateInfo(version, defaultBranch string) (*UpdateInfo, error) {
	worktreePath := m.GetWorktreePath(version)
	if !m.WorktreeExists(version) {
		return nil, fmt.Errorf("worktree does not exist for version %s", version)
	}

	// Get local HEAD
	localCmd := exec.Command("git", "-C", worktreePath, "rev-parse", "HEAD")
	localOutput, err := localCmd.Output()
	if err != nil {
		return nil, err
	}
	localSHA := strings.TrimSpace(string(localOutput))

	// Get remote HEAD
	remoteCmd := exec.Command("git", "-C", m.originDir, "rev-parse", fmt.Sprintf("origin/%s", defaultBranch))
	remoteOutput, err := remoteCmd.Output()
	if err != nil {
		return nil, err
	}
	remoteSHA := strings.TrimSpace(string(remoteOutput))

	// Get commits ahead
	aheadCmd := exec.Command("git", "-C", m.originDir, "rev-list", "--count", fmt.Sprintf("%s..origin/%s", localSHA, defaultBranch))
	aheadOutput, err := aheadCmd.Output()
	if err != nil {
		return nil, err
	}
	commitsAhead := 0
	fmt.Sscanf(strings.TrimSpace(string(aheadOutput)), "%d", &commitsAhead)

	// Generate URLs
	latestCommitURL := fmt.Sprintf("https://github.com/ProjectBorealis/UEGitPlugin/commit/%s", remoteSHA)
	compareURL := fmt.Sprintf("https://github.com/ProjectBorealis/UEGitPlugin/compare/%s...%s", localSHA, remoteSHA)

	return &UpdateInfo{
		EngineVersion:   version,
		CommitsAhead:    commitsAhead,
		LocalSHA:        localSHA,
		RemoteSHA:       remoteSHA,
		LatestCommitURL: latestCommitURL,
		CompareURL:      compareURL,
	}, nil
}

// UpdateWorktree updates a worktree to the latest version
func (m *Manager) UpdateWorktree(version, defaultBranch string) error {
	worktreePath := m.GetWorktreePath(version)
	if !m.WorktreeExists(version) {
		return fmt.Errorf("worktree does not exist for version %s", version)
	}

	// Fast-forward merge
	cmd := exec.Command("git", "-C", worktreePath, "merge", "--ff-only", fmt.Sprintf("origin/%s", defaultBranch))
	return cmd.Run()
}

// RemoveWorktree removes a worktree
func (m *Manager) RemoveWorktree(version string) error {
	worktreePath := m.GetWorktreePath(version)
	if !m.WorktreeExists(version) {
		return nil // Already removed
	}

	// First, try to remove the worktree normally
	cmd := exec.Command("git", "-C", m.originDir, "worktree", "remove", worktreePath)
	if err := cmd.Run(); err != nil {
		// If normal removal fails, try force removal
		fmt.Printf("  Normal worktree removal failed, trying force removal...\n")
		cmd = exec.Command("git", "-C", m.originDir, "worktree", "remove", "--force", worktreePath)
		if err := cmd.Run(); err != nil {
			// If Git worktree remove still fails, manually remove the directory
			fmt.Printf("  Git worktree remove failed, manually removing directory...\n")
			if err := os.RemoveAll(worktreePath); err != nil {
				return fmt.Errorf("failed to remove worktree directory: %v", err)
			}
			fmt.Printf("  ✅ Manually removed worktree directory\n")
		} else {
			fmt.Printf("  ✅ Force removed worktree\n")
		}
	} else {
		fmt.Printf("  ✅ Removed worktree\n")
	}

	// Remove the branch
	branchName := fmt.Sprintf("engine-%s", version)
	cmd = exec.Command("git", "-C", m.originDir, "branch", "-D", branchName)
	if err := cmd.Run(); err != nil {
		// Branch removal failure is not critical, just log it
		fmt.Printf("  Warning: Failed to remove branch %s: %v\n", branchName, err)
	} else {
		fmt.Printf("  ✅ Removed branch %s\n", branchName)
	}

	return nil
}

// RemoveOrigin removes the origin repository
func (m *Manager) RemoveOrigin() error {
	if !m.IsOriginCloned() {
		return nil
	}
	return os.RemoveAll(m.originDir)
}

// GetOriginDir returns the origin directory path
func (m *Manager) GetOriginDir() string {
	return m.originDir
}

// GetWorktreesDir returns the worktrees directory path
func (m *Manager) GetWorktreesDir() string {
	return m.worktreesDir
}
