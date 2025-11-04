package projectconfig

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	templates "ue-git-plugin-manager/internal/new_project_example_config_files"
)

// DetectProjectRoot validates the project dir by presence of a .uproject or Content dir
func DetectProjectRoot(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("path is not a directory: %s", path)
	}
	entries, _ := os.ReadDir(path)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".uproject") {
			return path, nil
		}
	}
	if _, err := os.Stat(filepath.Join(path, "Content")); err == nil {
		return path, nil
	}

	// Allow initializing before the Unreal project exists if this is a Git repo
	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		fmt.Println("Warning: No .uproject or Content/ folder found. Proceeding because a .git folder was detected.")
		return path, nil
	}
	return "", errors.New("not an Unreal project folder (no .uproject or Content/)")
}

func handleGitattributes(root string) error {
	templateLines, err := readEmbeddedLines(".gitattributes")
	if err != nil {
		return err
	}
	dest := filepath.Join(root, ".gitattributes")
	if _, err := os.Stat(dest); errors.Is(err, os.ErrNotExist) {
		return writeLines(dest, templateLines)
	}

	// Merge with conflict detection per rule 1.a
	existingLines, _ := readNonEmptyLines(dest)

	conflicts := detectGitattributesConflicts(existingLines, templateLines)
	if len(conflicts) > 0 {
		printConflictSummary(".gitattributes", conflicts)
		writeConflictsLog(root, ".gitattributes", conflicts)
		return nil
	}

	merged := mergeUniqueLines(existingLines, templateLines)
	return writeWithBackup(dest, merged, "# Added by UE Git Plugin Manager: .gitattributes")
}

func handleGitignore(root string, includeBinaries bool) error {
	commonLines, err := readEmbeddedLines("common.gitignore")
	if err != nil {
		return err
	}
	variant := "without_plugin_binaries.gitignore"
	if includeBinaries {
		variant = "with_plugin_binaries.gitignore"
	}
	variantLines, err := readEmbeddedLines(variant)
	if err != nil {
		return err
	}
	// Place variant-specific rules first (more important), common rules last
	templateLines := append([]string{}, variantLines...)
	templateLines = append(templateLines, commonLines...)

	dest := filepath.Join(root, ".gitignore")
	if _, err := os.Stat(dest); errors.Is(err, os.ErrNotExist) {
		return writeLines(dest, templateLines)
	}

	existingLines, _ := readNonEmptyLines(dest)
	conflicts := detectGitignoreConflicts(existingLines, templateLines)
	if len(conflicts) > 0 {
		printConflictSummary(".gitignore", conflicts)
		writeConflictsLog(root, ".gitignore", conflicts)
		return nil
	}

	merged := mergeUniqueLines(existingLines, templateLines)
	return writeWithBackup(dest, merged, "# Added by UE Git Plugin Manager: .gitignore")
}

func readEmbeddedLines(name string) ([]string, error) {
	b, err := templates.FS.ReadFile(name)
	if err != nil {
		return nil, err
	}
	s := bufio.NewScanner(strings.NewReader(string(b)))
	var lines []string
	for s.Scan() {
		line := strings.TrimRight(s.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines, nil
}

func readNonEmptyLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	var lines []string
	for s.Scan() {
		line := strings.TrimRight(s.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines, nil
}

func writeLines(dst string, lines []string) error {
	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(dst, []byte(content), 0644)
}

func writeWithBackup(dst string, lines []string, banner string) error {
	bak := dst + ".bak"
	if data, err := os.ReadFile(dst); err == nil {
		_ = os.WriteFile(bak, data, 0644)
	}
	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(dst, []byte(content), 0644)
}

func mergeUniqueLines(existing, tmpl []string) []string {
	set := map[string]bool{}
	for _, l := range existing {
		set[l] = true
	}
	merged := make([]string, 0, len(existing)+len(tmpl))
	merged = append(merged, existing...)
	for _, l := range tmpl {
		if !set[l] {
			merged = append(merged, l)
			set[l] = true
		}
	}
	return merged
}

// .gitattributes conflict: same pattern with different attributes
func detectGitattributesConflicts(existing, tmpl []string) []string {
	ex := parseAttributes(existing)
	tm := parseAttributes(tmpl)
	var conflicts []string
	for pattern, attrs := range tm {
		if eattrs, ok := ex[pattern]; ok && eattrs != attrs {
			conflicts = append(conflicts, fmt.Sprintf("%s -> existing: [%s], template: [%s]", pattern, eattrs, attrs))
		}
	}
	return conflicts
}

func parseAttributes(lines []string) map[string]string {
	m := map[string]string{}
	for _, l := range lines {
		line := strings.TrimSpace(l)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		pattern := parts[0]
		attrs := strings.Join(parts[1:], " ")
		m[pattern] = attrs
	}
	return m
}

// .gitignore conflict: pattern ignored vs negated across existing + template
func detectGitignoreConflicts(existing, tmpl []string) []string {
	ex := effectiveIgnoreMap(existing)
	tm := effectiveIgnoreMap(tmpl)
	var conflicts []string
	for pat, tneg := range tm {
		if eneg, ok := ex[pat]; ok {
			if eneg != tneg {
				state := func(b bool) string {
					if b {
						return "negated"
					} else {
						return "ignored"
					}
				}
				conflicts = append(conflicts, fmt.Sprintf("%s -> existing: %s, template: %s", pat, state(eneg), state(tneg)))
			}
		}
	}
	return conflicts
}

func effectiveIgnoreMap(lines []string) map[string]bool {
	m := map[string]bool{}
	for _, l := range lines {
		line := strings.TrimSpace(l)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		neg := strings.HasPrefix(line, "!")
		pat := strings.TrimPrefix(line, "!")
		m[pat] = neg
	}
	return m
}

func printConflictSummary(name string, conflicts []string) {
	fmt.Printf("⚠️  Conflicts detected in %s (%d):\n", name, len(conflicts))
	for i, c := range conflicts {
		if i >= 5 {
			break
		}
		fmt.Printf("  - %s\n", c)
	}
	fmt.Println("This file was not modified. Review and resolve conflicts manually.")
}

func writeConflictsLog(root string, name string, conflicts []string) {
	_ = os.MkdirAll("logs", 0755)
	fname := fmt.Sprintf("logs/%s_conflicts_%d.txt", strings.TrimPrefix(name, "."), time.Now().Unix())
	_ = os.WriteFile(fname, []byte(strings.Join(conflicts, "\n")), 0644)
}
