package main

import (
	"bytes"
	"context"
	"io"
	"testing"
)

func TestMCPCommandForwardsConfigurationAndStreams(t *testing.T) {
	t.Parallel()

	var received mcpConfig
	input := bytes.NewBufferString("request")
	output := &bytes.Buffer{}
	errors := &bytes.Buffer{}
	command := newMCPCommandWithRunner(func(
		_ context.Context,
		configuration mcpConfig,
		stdin io.Reader,
		stdout io.Writer,
		stderr io.Writer,
	) error {
		received = configuration
		if stdin != input || stdout != output || stderr != errors {
			t.Fatal("MCP streams were not forwarded")
		}
		return nil
	})
	command.SetIn(input)
	command.SetOut(output)
	command.SetErr(errors)
	command.SetArgs([]string{
		"--root", "/project",
		"--analysis-command", "/opt/analysis",
	})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if received.ProjectRoot != "/project" ||
		received.AnalysisCommand != "/opt/analysis" {
		t.Fatalf("received config = %#v", received)
	}
}

func TestMCPCommandUsesDefaultConfiguration(t *testing.T) {
	t.Parallel()

	var received mcpConfig
	command := newMCPCommandWithRunner(func(
		_ context.Context,
		configuration mcpConfig,
		_ io.Reader,
		_ io.Writer,
		_ io.Writer,
	) error {
		received = configuration
		return nil
	})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if received.ProjectRoot != "." || received.AnalysisCommand != defaultAnalysisCommand {
		t.Fatalf("default config = %#v", received)
	}
}
