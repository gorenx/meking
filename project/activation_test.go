package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/memoria-space/meking/knowledge/mas"
	"github.com/memoria-space/meking/memory/activation"
)

func TestProjectLoadsDomainOwnedActivationConfiguration(t *testing.T) {
	t.Setenv("MEKING_API_KEY", "test-only-credential")
	root := filepath.Join(t.TempDir(), "project")
	if _, err := NewLocalProjectService().Initialize(t.Context(), InitializeProject{Root: root, CompletionModel: "test-completion", EmbeddingModel: "test-embedding"}); err != nil {
		t.Fatal(err)
	}
	// Older Projects do not contain the new prompt files. Their omitted
	// activation configuration resolves entirely through domain defaults.
	for name := range activation.DefaultPrompts() {
		if err := os.Remove(filepath.Join(root, "prompts", name)); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := LoadConfiguration(root)
	if err != nil {
		t.Fatal(err)
	}
	defaults, err := (activation.Config{}).Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Activation != defaults {
		t.Fatal("Project changed domain defaults")
	}
	settingsBytes, err := os.ReadFile(filepath.Join(root, "settings.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	settings, err := ParseSettings(settingsBytes)
	if err != nil {
		t.Fatal(err)
	}
	parameters := mas.DefaultConfig().Parameters
	parameters[0] += .01
	settings.Activation = activation.Config{Parameters: parameters[:], ChinesePrompt: "prompts/custom.zh-CN.md", EnglishPrompt: "prompts/custom.en.md"}
	for path, text := range map[string]string{settings.Activation.ChinesePrompt: "中文规则", settings.Activation.EnglishPrompt: "English rules"} {
		if err := os.WriteFile(filepath.Join(root, path), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	settingsBytes, err = MarshalSettings(settings)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "settings.yaml"), settingsBytes, 0600); err != nil {
		t.Fatal(err)
	}
	custom, err := LoadConfiguration(root)
	if err != nil {
		t.Fatal(err)
	}
	if custom.Activation.Model.Parameters != parameters || custom.Activation.ChinesePrompt != "中文规则" || custom.Activation.EnglishPrompt != "English rules" {
		t.Fatal("Project did not delegate loaded configuration")
	}
	if err := os.WriteFile(filepath.Join(root, settings.Activation.EnglishPrompt), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if custom.Activation.EnglishPrompt != "English rules" {
		t.Fatal("loaded configuration changed without restart")
	}
	if err := os.Remove(filepath.Join(root, settings.Activation.EnglishPrompt)); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfiguration(root); err == nil {
		t.Fatal("missing configured prompt silently fell back")
	}
}

func TestProjectRejectsInvalidActivationSettings(t *testing.T) {
	settings, err := DefaultSettings("test-completion", "test-embedding")
	if err != nil {
		t.Fatal(err)
	}
	base, err := MarshalSettings(settings)
	if err != nil {
		t.Fatal(err)
	}
	for _, extra := range []string{
		"activation:\n  unknown: true\n",
		"activation:\n  parameters: [0.2]\n",
		"activation:\n  parameters: []\n",
		"activation:\n  chinese_prompt: ../outside.md\n",
	} {
		if _, err := ParseSettings(append(append([]byte{}, base...), []byte(extra)...)); err == nil {
			t.Fatalf("invalid configuration accepted: %s", extra)
		}
	}
}
