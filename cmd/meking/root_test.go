package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/memoria-space/meking/project"
	"github.com/spf13/cobra"
)

func TestRootCommandOnlyExposesProjectLifecycle(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	command := newRootCommand(&stdout, &bytes.Buffer{})
	command.SetArgs([]string{"--help"})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var names []string
	for _, child := range command.Commands() {
		if child.IsAvailableCommand() {
			names = append(names, child.Name())
		}
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"init", "mcp", "start"}) {
		t.Fatalf("commands = %v, want [init mcp start]", names)
	}
	for _, removed := range []string{"completion", "model-smoke", "query"} {
		if strings.Contains(stdout.String(), removed) {
			t.Fatalf("help contains removed command %q: %s", removed, stdout.String())
		}
	}
}

func TestResolveProjectRootFlagUsesProcessWorkingDirectory(t *testing.T) {
	t.Parallel()

	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	var root string
	command := &cobra.Command{Use: "test"}
	command.Flags().StringVar(&root, "root", "", "")
	if err := command.Flags().Set("root", filepath.Join("..", "project")); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := resolveProjectRootFlag(command); err != nil {
		t.Fatalf("resolveProjectRootFlag() error = %v", err)
	}
	want := filepath.Clean(filepath.Join(workingDirectory, "..", "project"))
	if root != want {
		t.Fatalf("root = %q, want %q", root, want)
	}
}

func TestResolveProjectRootFlagPreservesAbsolutePath(t *testing.T) {
	t.Parallel()

	var root string
	command := &cobra.Command{Use: "test"}
	command.Flags().StringVar(&root, "root", "", "")
	want := filepath.Clean(t.TempDir())
	if err := command.Flags().Set("root", want); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := resolveProjectRootFlag(command); err != nil {
		t.Fatalf("resolveProjectRootFlag() error = %v", err)
	}
	if root != want {
		t.Fatalf("root = %q, want %q", root, want)
	}
}

func TestInitCommandCreatesProject(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "project")
	var stdout bytes.Buffer
	command := newRootCommand(&stdout, &bytes.Buffer{})
	command.SetArgs([]string{
		"init", "--root", root,
		"--model", "gpt-test",
		"--embedding", "embedding-test",
		"--model-base-url", "https://completion.example/v1",
		"--embedding-base-url", "https://embedding.example/v1",
	})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), root) {
		t.Fatalf("stdout = %q, want Project path", stdout.String())
	}

	data, err := os.ReadFile(filepath.Join(root, "settings.yaml"))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	settings, err := project.ParseSettings(data)
	if err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	if got := settings.CompletionModels["default_completion_model"].Model; got != "gpt-test" {
		t.Fatalf("completion model = %q, want %q", got, "gpt-test")
	}
	if got := settings.CompletionModels["default_completion_model"].BaseURL; got != "https://completion.example/v1" {
		t.Fatalf("completion base URL = %q", got)
	}
	if got := settings.EmbeddingModels["default_embedding_model"].BaseURL; got != "https://embedding.example/v1" {
		t.Fatalf("embedding base URL = %q", got)
	}
}

func TestInitCommandAcceptsProjectPathArgument(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "project")
	var stdout bytes.Buffer
	command := newRootCommand(&stdout, &bytes.Buffer{})
	command.SetArgs([]string{"init", root})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), root) {
		t.Fatalf("stdout = %q, want Project path", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(root, "settings.yaml")); err != nil {
		t.Fatalf("stat settings: %v", err)
	}
}

func TestInitCommandRejectsProjectPathArgumentWithRootFlag(t *testing.T) {
	t.Parallel()

	command := newRootCommand(&bytes.Buffer{}, &bytes.Buffer{})
	command.SetArgs([]string{"init", t.TempDir(), "--root", t.TempDir()})
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "cannot be used together") {
		t.Fatalf("Execute() error = %v, want conflicting Project paths", err)
	}
}

func TestInitCommandRejectsDuplicateWithoutForce(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "project")
	first := newRootCommand(&bytes.Buffer{}, &bytes.Buffer{})
	first.SetArgs([]string{"init", "--root", root})
	if err := first.Execute(); err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}

	second := newRootCommand(&bytes.Buffer{}, &bytes.Buffer{})
	second.SetArgs([]string{"init", "--root", root})
	if err := second.Execute(); !errors.Is(err, project.ErrProjectAlreadyInitialized) {
		t.Fatalf("second Execute() error = %v, want ErrProjectAlreadyInitialized", err)
	}
}
