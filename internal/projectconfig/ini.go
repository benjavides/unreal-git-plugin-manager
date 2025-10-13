package projectconfig

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ue-git-plugin-manager/internal/utils"

	"github.com/manifoldco/promptui"
)

type IniAnswers struct {
	AutoAddNewFiles bool
	AutoCheckout    bool
	PromptCheckout  bool
	AutoloadChecked bool
	SkipEditableSC  bool
}

func promptIniAnswers() (IniAnswers, error) {
	ans := IniAnswers{}
	// Q1
	q1 := promptui.Select{Label: "Automatically track new files?", Items: []string{"Yes", "No"}, Stdout: &utils.BellSkipper{}}
	_, r1, err := q1.Run()
	if err != nil {
		return ans, err
	}
	ans.AutoAddNewFiles = r1 == "Yes"
	// Default per user brief: Yes default; we'll set either way later

	// Q2
	q2 := promptui.Select{Label: "Checkout style", Items: []string{"Automatically check on modification", "Ask user to check on modification"}, Stdout: &utils.BellSkipper{}}
	_, r2, err := q2.Run()
	if err != nil {
		return ans, err
	}
	ans.AutoCheckout = r2 == "Automatically check on modification"
	ans.PromptCheckout = !ans.AutoCheckout

	// Q3
	q3 := promptui.Select{Label: "Load checked packages for faster loading", Items: []string{"Yes", "No"}, Stdout: &utils.BellSkipper{}}
	_, r3, err := q3.Run()
	if err != nil {
		return ans, err
	}
	ans.AutoloadChecked = r3 == "Yes"

	// Q4
	q4 := promptui.Select{Label: "Skip Source Control check for editable packages", Items: []string{"Skip", "Do not skip"}, Stdout: &utils.BellSkipper{}}
	_, r4, err := q4.Run()
	if err != nil {
		return ans, err
	}
	ans.SkipEditableSC = r4 == "Skip"

	return ans, nil
}

func ApplyIniSettings(root string, ans IniAnswers) error {
	userIni := filepath.Join(root, "Config", "DefaultEditorPerProjectUserSettings.ini")
	engineIni := filepath.Join(root, "Config", "DefaultEngine.ini")

	if err := upsertIni(userIni, "/Script/UnrealEd.EditorLoadingSavingSettings", "bSCCAutoAddNewFiles", boolToUE(ans.AutoAddNewFiles)); err != nil {
		return err
	}
	if ans.AutoCheckout {
		if err := upsertIni(userIni, "/Script/UnrealEd.EditorLoadingSavingSettings", "bAutomaticallyCheckoutOnAssetModification", "True"); err != nil {
			return err
		}
		if err := upsertIni(userIni, "/Script/UnrealEd.EditorLoadingSavingSettings", "bPromptForCheckoutOnAssetModification", "False"); err != nil {
			return err
		}
	} else {
		if err := upsertIni(userIni, "/Script/UnrealEd.EditorLoadingSavingSettings", "bAutomaticallyCheckoutOnAssetModification", "False"); err != nil {
			return err
		}
		if err := upsertIni(userIni, "/Script/UnrealEd.EditorLoadingSavingSettings", "bPromptForCheckoutOnAssetModification", "True"); err != nil {
			return err
		}
	}
	if err := upsertIni(userIni, "/Script/UnrealEd.EditorPerProjectUserSettings", "bAutoloadCheckedOutPackages", boolToUE(ans.AutoloadChecked)); err != nil {
		return err
	}

	val := "0"
	if ans.SkipEditableSC {
		val = "1"
	}
	if err := upsertIni(engineIni, "SystemSettingsEditor", "r.Editor.SkipSourceControlCheckForEditablePackages", val); err != nil {
		return err
	}

	return nil
}

func boolToUE(v bool) string {
	if v {
		return "True"
	}
	return "False"
}

func upsertIni(path string, section string, key string, value string) error {
	// ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	// read if exists
	var lines []string
	if data, err := os.ReadFile(path); err == nil {
		s := bufio.NewScanner(strings.NewReader(string(data)))
		for s.Scan() {
			lines = append(lines, strings.TrimRight(s.Text(), "\r"))
		}
	}

	sectionHeader := fmt.Sprintf("[%s]", section)
	if strings.HasPrefix(section, "/") { // normalize UE script section headers
		sectionHeader = fmt.Sprintf("[%s]", section)
	}

	foundSection := false
	sectionStart := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == sectionHeader {
			foundSection = true
			sectionStart = i
			break
		}
	}

	if !foundSection {
		if len(lines) > 0 && lines[len(lines)-1] != "" {
			lines = append(lines, "")
		}
		lines = append(lines, sectionHeader)
		lines = append(lines, fmt.Sprintf("%s=%s", key, value))
		content := strings.Join(lines, "\n") + "\n"
		return os.WriteFile(path, []byte(content), 0644)
	}

	// upsert within section until next [
	inserted := false
	for i := sectionStart + 1; i <= len(lines); i++ {
		if i == len(lines) || (strings.HasPrefix(strings.TrimSpace(lines[i]), "[") && strings.HasSuffix(strings.TrimSpace(lines[i]), "]")) {
			// reached end of section; append if not inserted
			if !inserted {
				lines = append(lines[:i], append([]string{fmt.Sprintf("%s=%s", key, value)}, lines[i:]...)...)
			}
			break
		}
		kv := strings.TrimSpace(lines[i])
		if kv == "" || strings.HasPrefix(kv, ";") || strings.HasPrefix(kv, "#") {
			continue
		}
		if kvp := strings.SplitN(kv, "=", 2); len(kvp) == 2 && strings.EqualFold(strings.TrimSpace(kvp[0]), key) {
			lines[i] = fmt.Sprintf("%s=%s", key, value)
			inserted = true
			break
		}
	}
	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(path, []byte(content), 0644)
}
