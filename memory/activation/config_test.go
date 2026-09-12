package activation_test

import (
	"math"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/memoria-space/meking/knowledge/mas"
	"github.com/memoria-space/meking/memory/activation"
)

func TestConfigOwnsDefaultsAndPromptResolution(t *testing.T) {
	config := activation.Config{}
	resolved, err := config.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	defaults := activation.DefaultPrompts()
	if resolved.Model != mas.DefaultConfig() || resolved.ChinesePrompt != defaults["recall.zh-CN.md"] || resolved.EnglishPrompt != defaults["recall.en.md"] {
		t.Fatal("domain defaults diverged")
	}
	defaults["recall.en.md"] = "changed"
	if activation.DefaultPrompts()["recall.en.md"] == "changed" {
		t.Fatal("default prompts are mutable")
	}
	model := mas.DefaultConfig()
	parameters := append([]float64{}, model.Parameters[:]...)
	parameters[0] += .01
	config = activation.Config{Parameters: parameters, ChinesePrompt: "prompts/custom.zh-CN.md", EnglishPrompt: "prompts/custom.en.md"}
	if got := config.PromptFiles(); !reflect.DeepEqual(got, []string{"prompts/custom.zh-CN.md", "prompts/custom.en.md"}) {
		t.Fatalf("prompt files: %v", got)
	}
	loaded := map[string]string{"prompts/custom.zh-CN.md": "中文评估规则", "prompts/custom.en.md": "English evaluation rules"}
	resolved, err = config.Resolve(loaded)
	if err != nil {
		t.Fatal(err)
	}
	parameters[0] = 99
	loaded["prompts/custom.en.md"] = "mutated"
	if resolved.Model.Parameters[0] != model.Parameters[0]+.01 || resolved.EnglishPrompt != "English evaluation rules" {
		t.Fatal("resolved configuration is not a fixed snapshot")
	}
}

func TestConfigRejectsPartialParametersAndInvalidPromptSources(t *testing.T) {
	for _, parameters := range [][]float64{{}, {1}, make([]float64, 22)} {
		if err := (activation.Config{Parameters: parameters}).Validate(); err == nil {
			t.Fatalf("accepted %d parameters", len(parameters))
		}
	}
	model := mas.DefaultConfig()
	parameters := append([]float64{}, model.Parameters[:]...)
	parameters[0] = math.NaN()
	if err := (activation.Config{Parameters: parameters}).Validate(); err == nil {
		t.Fatal("accepted NaN")
	}
	for _, path := range []string{"../outside", filepath.Join(string(filepath.Separator), "absolute"), ".", " ", "bad\x00name"} {
		if err := (activation.Config{ChinesePrompt: path}).Validate(); err == nil {
			t.Fatalf("accepted prompt path %q", path)
		}
	}
	config := activation.Config{EnglishPrompt: "prompts/missing.md"}
	if _, err := config.Resolve(nil); err == nil {
		t.Fatal("silently replaced missing override with default")
	}
	if _, err := config.Resolve(map[string]string{"prompts/missing.md": " "}); err == nil {
		t.Fatal("accepted empty prompt")
	}
}
