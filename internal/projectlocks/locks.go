package projectlocks

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Lock struct {
	ID       string
	Path     string
	Owner    string
	LockedAt string
}

type RepairItem struct {
	Lock   Lock
	Reason string
}

type RepairReport struct {
	ProjectRoot     string
	TotalLocks      int
	CandidateLocks  int
	OwnershipSource string
	Upstream        string
	Unlocked        []Lock
	Skipped         []RepairItem
	Failed          []RepairItem
}

type lfsOwner struct {
	Name string `json:"name"`
}

type lfsLock struct {
	ID       string   `json:"id"`
	Path     string   `json:"path"`
	LockedAt string   `json:"locked_at"`
	Owner    lfsOwner `json:"owner"`
}

type lfsLocksResponse struct {
	Locks      []lfsLock `json:"locks"`
	Ours       []lfsLock `json:"ours"`
	Theirs     []lfsLock `json:"theirs"`
	NextCursor string    `json:"next_cursor"`
}

type fetchedLocks struct {
	All               []Lock
	Ours              []Lock
	OwnershipVerified bool
}

func ListProjectLocks(projectPath string) ([]Lock, error) {
	root, err := resolveGitRoot(projectPath)
	if err != nil {
		return nil, err
	}

	locks, err := fetchLocks(root)
	if err != nil {
		return nil, err
	}

	return locks.All, nil
}

func RepairMyLocks(projectPath string) (*RepairReport, error) {
	root, err := resolveGitRoot(projectPath)
	if err != nil {
		return nil, err
	}

	lockSet, err := fetchLocks(root)
	if err != nil {
		return nil, err
	}

	report := &RepairReport{
		ProjectRoot: root,
		TotalLocks:  len(lockSet.All),
		Unlocked:    []Lock{},
		Skipped:     []RepairItem{},
		Failed:      []RepairItem{},
	}

	if report.TotalLocks == 0 {
		report.OwnershipSource = "none"
		return report, nil
	}

	candidates, ownershipSource := selectCandidateLocks(root, lockSet)
	report.CandidateLocks = len(candidates)
	report.OwnershipSource = ownershipSource

	if len(candidates) == 0 {
		return report, nil
	}

	upstream, err := getUpstreamRef(root)
	if err != nil {
		reason := "No upstream tracking branch is configured; cannot verify pushed changes safely"
		for _, lock := range candidates {
			report.Skipped = append(report.Skipped, RepairItem{Lock: lock, Reason: reason})
		}
		return report, nil
	}
	report.Upstream = upstream

	for _, lock := range candidates {
		changed, err := hasUncommittedChanges(root, lock.Path)
		if err != nil {
			report.Skipped = append(report.Skipped, RepairItem{Lock: lock, Reason: fmt.Sprintf("Could not check working tree status: %v", err)})
			continue
		}
		if changed {
			report.Skipped = append(report.Skipped, RepairItem{Lock: lock, Reason: "File has local uncommitted changes"})
			continue
		}

		unpushed, err := hasUnpushedChangesForPath(root, upstream, lock.Path)
		if err != nil {
			report.Skipped = append(report.Skipped, RepairItem{Lock: lock, Reason: fmt.Sprintf("Could not verify push status: %v", err)})
			continue
		}
		if unpushed {
			report.Skipped = append(report.Skipped, RepairItem{Lock: lock, Reason: "File has local commits that are not pushed yet"})
			continue
		}

		if err := unlockLock(root, lock); err != nil {
			report.Failed = append(report.Failed, RepairItem{Lock: lock, Reason: err.Error()})
			continue
		}

		report.Unlocked = append(report.Unlocked, lock)
	}

	return report, nil
}

func resolveGitRoot(projectPath string) (string, error) {
	output, err := runGit(projectPath, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("path is not a git repository: %w", err)
	}
	return strings.TrimSpace(output), nil
}

func fetchLocks(projectRoot string) (fetchedLocks, error) {
	locks, err := fetchLocksWithMode(projectRoot, true)
	if err == nil {
		return locks, nil
	}

	return fetchLocksWithMode(projectRoot, false)
}

func fetchLocksWithMode(projectRoot string, verifyOwnership bool) (fetchedLocks, error) {
	result := fetchedLocks{
		All:  []Lock{},
		Ours: []Lock{},
	}

	cursor := ""
	for {
		args := []string{"lfs", "locks", "--json", "--limit", "100"}
		if verifyOwnership {
			args = append(args, "--verify")
		}
		if strings.TrimSpace(cursor) != "" {
			args = append(args, "--cursor", cursor)
		}

		output, err := runGit(projectRoot, args...)
		if err != nil {
			return fetchedLocks{}, err
		}

		var payload lfsLocksResponse
		if err := json.Unmarshal([]byte(output), &payload); err != nil {
			return fetchedLocks{}, fmt.Errorf("failed to parse git lfs locks output: %w", err)
		}

		if len(payload.Ours) > 0 || len(payload.Theirs) > 0 {
			result.OwnershipVerified = true
			ours := toLocks(payload.Ours)
			theirs := toLocks(payload.Theirs)
			result.Ours = append(result.Ours, ours...)
			result.All = append(result.All, ours...)
			result.All = append(result.All, theirs...)
		} else {
			result.All = append(result.All, toLocks(payload.Locks)...)
		}

		cursor = strings.TrimSpace(payload.NextCursor)
		if cursor == "" {
			break
		}
	}

	result.All = dedupeLocks(result.All)
	result.Ours = dedupeLocks(result.Ours)
	return result, nil
}

func toLocks(raw []lfsLock) []Lock {
	locks := make([]Lock, 0, len(raw))
	for _, entry := range raw {
		locks = append(locks, Lock{
			ID:       entry.ID,
			Path:     strings.TrimSpace(entry.Path),
			Owner:    strings.TrimSpace(entry.Owner.Name),
			LockedAt: strings.TrimSpace(entry.LockedAt),
		})
	}
	return locks
}

func dedupeLocks(locks []Lock) []Lock {
	seen := map[string]bool{}
	unique := make([]Lock, 0, len(locks))
	for _, lock := range locks {
		key := lock.ID + "|" + lock.Path
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, lock)
	}
	return unique
}

func selectCandidateLocks(projectRoot string, lockSet fetchedLocks) ([]Lock, string) {
	if lockSet.OwnershipVerified {
		return lockSet.Ours, "server-verified ownership"
	}

	identities := getIdentityTokens(projectRoot)
	if len(identities) == 0 {
		return []Lock{}, "no ownership data"
	}

	candidates := make([]Lock, 0)
	for _, lock := range lockSet.All {
		owner := strings.ToLower(strings.TrimSpace(lock.Owner))
		if owner == "" {
			continue
		}
		if identities[owner] {
			candidates = append(candidates, lock)
		}
	}

	return candidates, "git identity match"
}

func getIdentityTokens(projectRoot string) map[string]bool {
	tokens := map[string]bool{}

	userName, _ := runGit(projectRoot, "config", "user.name")
	userName = strings.ToLower(strings.TrimSpace(userName))
	if userName != "" {
		tokens[userName] = true
	}

	userEmail, _ := runGit(projectRoot, "config", "user.email")
	userEmail = strings.ToLower(strings.TrimSpace(userEmail))
	if userEmail != "" {
		tokens[userEmail] = true
		if at := strings.Index(userEmail, "@"); at > 0 {
			tokens[userEmail[:at]] = true
		}
	}

	envUsername := strings.ToLower(strings.TrimSpace(os.Getenv("USERNAME")))
	if envUsername != "" {
		tokens[envUsername] = true
	}

	return tokens
}

func getUpstreamRef(projectRoot string) (string, error) {
	upstream, err := runGit(projectRoot, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if err != nil {
		return "", err
	}
	upstream = strings.TrimSpace(upstream)
	if upstream == "" {
		return "", fmt.Errorf("upstream not found")
	}
	return upstream, nil
}

func hasUncommittedChanges(projectRoot, filePath string) (bool, error) {
	output, err := runGit(projectRoot, "status", "--porcelain", "--", filePath)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(output) != "", nil
}

func hasUnpushedChangesForPath(projectRoot, upstream, filePath string) (bool, error) {
	output, err := runGit(projectRoot, "diff", "--name-only", fmt.Sprintf("%s..HEAD", upstream), "--", filePath)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(output) != "", nil
}

func unlockLock(projectRoot string, lock Lock) error {
	args := []string{"lfs", "unlock"}
	if strings.TrimSpace(lock.ID) != "" {
		args = append(args, "--id", lock.ID)
	} else {
		args = append(args, lock.Path)
	}

	_, err := runGit(projectRoot, args...)
	if err != nil {
		return fmt.Errorf("unlock failed: %w", err)
	}
	return nil
}

func runGit(projectRoot string, args ...string) (string, error) {
	fullArgs := append([]string{"-C", projectRoot}, args...)
	cmd := exec.Command("git", fullArgs...)
	output, err := cmd.CombinedOutput()
	result := strings.TrimSpace(string(output))
	if err != nil {
		if result == "" {
			return "", err
		}
		return "", fmt.Errorf("%v: %s", err, result)
	}
	return result, nil
}
