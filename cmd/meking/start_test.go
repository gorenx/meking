package main

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/memoria-space/meking/cmd/meking/internal/httpservice"
)

func TestNewStartCommandWithRunnerForwardsProjectServerConfig(t *testing.T) {
	t.Parallel()

	var received httpservice.Config
	runner := func(_ context.Context, config httpservice.Config, _ io.Writer, _ io.Writer) error {
		received = config
		return nil
	}
	command := newStartCommandWithRunner(runner)
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{
		"--root", "/project",
		"--analysis-command", "/opt/analysis",
		"--port", "7001",
		"--address", "127.0.0.2",
	})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if received.ProjectRoot != "/project" || received.AnalysisCommand != "/opt/analysis" ||
		received.Address != "127.0.0.2" || received.Port != 7001 {
		t.Fatalf("received config = %#v", received)
	}
}

func TestStartCommandUsesSafeDefaults(t *testing.T) {
	t.Parallel()

	var received httpservice.Config
	command := newStartCommandWithRunner(func(
		_ context.Context, config httpservice.Config, _ io.Writer, _ io.Writer,
	) error {
		received = config
		return nil
	})
	command.SetArgs(nil)
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if received.ProjectRoot != "." || received.AnalysisCommand != defaultAnalysisCommand ||
		received.Address != defaultStartAddress || received.Port != defaultStartPort {
		t.Fatalf("default config = %#v", received)
	}
}

func TestStartCommandRejectsInvalidListenerConfig(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"--root", ""},
		{"--address", ""},
		{"--port", "0"},
		{"--port", "65536"},
	} {
		command := newStartCommandWithRunner(func(
			context.Context, httpservice.Config, io.Writer, io.Writer,
		) error {
			t.Fatal("runner called with invalid configuration")
			return nil
		})
		command.SetArgs(args)
		if err := command.Execute(); err == nil {
			t.Fatalf("Execute(%v) error = nil", args)
		}
	}
}
