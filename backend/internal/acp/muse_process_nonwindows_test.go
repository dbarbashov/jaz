//go:build !windows

package acp_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestMuseConfigViewSurvivesGracefulProcessShutdown(t *testing.T) {
	root := t.TempDir()
	store, err := jsonstore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	capture := filepath.Join(root, "shutdown-rules")
	manager := acp.NewManager(store, acp.Config{
		Root:         root,
		Workspace:    root,
		SystemPrompt: staticPrompt("Jaz shutdown rules"),
		Agents: map[string]acp.AgentConfig{
			acp.AgentMuse: {
				Command: os.Args[0],
				Args:    []string{"-test.run=TestFakeACPAgentProcess"},
				Env: map[string]string{
					"MUSE_CLI":                           os.Args[0],
					"XDG_CONFIG_HOME":                    filepath.Join(root, "config"),
					"JAZ_FAKE_ACP_AGENT":                 "1",
					"JAZ_FAKE_ACP_MUSE_SHUTDOWN_CAPTURE": capture,
				},
			},
		},
	}, log.New(io.Discard))
	t.Cleanup(manager.Close)
	if _, err := manager.Spawn(t.Context(), acp.SpawnRequest{ACPAgent: acp.AgentMuse, Slug: "muse-shutdown"}); err != nil {
		t.Fatal(err)
	}
	manager.Close()
	rules, err := os.ReadFile(capture)
	if err != nil || string(rules) != "Jaz shutdown rules" {
		t.Fatalf("Muse lost its configuration before native shutdown finished: %q, %v", rules, err)
	}
}
