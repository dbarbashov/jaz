package acp_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/provider"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestMuseSessionPreservesNativeConfigAndJazInstructionsOnReload(t *testing.T) {
	for _, rulesName := range []string{"AGENTS.md", "CLAUDE.md"} {
		t.Run(rulesName, func(t *testing.T) {
			root := t.TempDir()
			config := filepath.Join(root, "config", "muse")
			if err := os.MkdirAll(config, 0o700); err != nil {
				t.Fatal(err)
			}
			nativeSettings := "{\"model\":\"native-default\"}"
			for name, content := range map[string]string{rulesName: "native rules", "settings.json": nativeSettings} {
				if err := os.WriteFile(filepath.Join(config, name), []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			capture := filepath.Join(root, "config-path")
			requestLog := filepath.Join(root, "requests")
			store, err := jsonstore.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			factory := func() *acp.Manager {
				manager := acp.NewManager(store, acp.Config{
					Root:         root,
					Workspace:    root,
					SystemPrompt: staticPrompt("Jaz context"),
					Agents: map[string]acp.AgentConfig{
						acp.AgentMuse: {
							Command:         os.Args[0],
							Args:            []string{"-test.run=TestFakeACPAgentProcess"},
							Model:           "fake-large",
							ReasoningEffort: "none",
							Env: map[string]string{
								"MUSE_CLI":                         os.Args[0],
								"XDG_CONFIG_HOME":                  filepath.Dir(config),
								"JAZ_FAKE_ACP_AGENT":               "1",
								"JAZ_FAKE_ACP_MUSE_CONFIG_CAPTURE": capture,
								"JAZ_FAKE_ACP_MUSE_WRITE_CONFIG":   "1",
								"JAZ_FAKE_ACP_REQUEST_LOG":         requestLog,
								"JAZ_FAKE_ACP_RESUME":              "1",
								"JAZ_FAKE_ACP_MODELS":              "fake-large",
								"JAZ_FAKE_ACP_EXPECT_MODEL_CONFIG": "fake-large",
								"JAZ_FAKE_ACP_SET_CONFIG":          "1",
								"JAZ_FAKE_ACP_EXPECT_EFFORT":       "none",
							},
						},
					},
				}, log.New(io.Discard))
				t.Cleanup(manager.Close)
				return manager
			}
			manager := factory()
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: acp.AgentMuse, Slug: "muse-context"})
			if err != nil {
				t.Fatal(err)
			}
			for index := 0; index < 2; index++ {
				if _, err := manager.Send(ctx, acp.SendRequest{Session: spawned.SessionID, Message: "hello", Completion: acp.CompletionInline}); err != nil {
					t.Fatal(err)
				}
				job, err := manager.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: 5 * time.Second})
				if err != nil || job.State != acp.StateIdle || job.Assistant != "hello from fake agent" {
					t.Fatalf("Muse turn = %#v, %v", job, err)
				}
				view, err := os.ReadFile(capture)
				if err != nil {
					t.Fatal(err)
				}
				rules, err := os.ReadFile(filepath.Join(string(view), "muse", "AGENTS.md"))
				if err != nil || string(rules) != "native rules\n\nJaz context" {
					t.Fatalf("Muse instructions = %q, %v", rules, err)
				}
				settings, err := os.ReadFile(filepath.Join(string(view), "muse", "settings.json"))
				if err != nil || string(settings) != nativeSettings {
					t.Fatalf("native settings = %q, %v", settings, err)
				}
				manager.Close()
				if _, err := os.Stat(string(view)); !os.IsNotExist(err) {
					t.Fatalf("Muse configuration view survived Close: %v", err)
				}
				if index == 0 {
					manager = factory()
				}
			}
			rules, err := os.ReadFile(filepath.Join(config, rulesName))
			if err != nil || string(rules) != "native rules" {
				t.Fatalf("native rules changed: %q, %v", rules, err)
			}
			for _, name := range []string{"auth.json", "trust.json", ".auth.json.lock", ".trust.json.lock", ".settings.json.lock"} {
				data, err := os.ReadFile(filepath.Join(config, name))
				if err != nil || string(data) != "native state" {
					t.Fatalf("native config save did not reach its original profile (%s): %q, %v", name, data, err)
				}
			}
			requests, err := os.ReadFile(requestLog)
			if err != nil {
				t.Fatal(err)
			}
			resumed := false
			for _, line := range strings.Split(strings.TrimSpace(string(requests)), "\n") {
				var request map[string]json.RawMessage
				if err := json.Unmarshal([]byte(line), &request); err != nil {
					t.Fatal(err)
				}
				resumed = resumed || string(request["method"]) == "\"session/resume\""
				if strings.Contains(string(request["params"]), "Jaz context") {
					t.Fatal("Jaz instructions leaked into ACP prompt or metadata")
				}
			}
			if !resumed {
				t.Fatal("Muse session was not resumed after restart")
			}
			messages, err := store.LoadMessages(spawned.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			for _, message := range messages {
				if strings.Contains(provider.MessageContent(message), "Jaz context") {
					t.Fatal("Jaz instructions leaked into transcript")
				}
			}
			unsupported := "{\"permissions\":{\"default_profile\":\":auto-review\"}}"
			if err := os.WriteFile(filepath.Join(config, "settings.json"), []byte(unsupported), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := factory().Spawn(ctx, acp.SpawnRequest{ACPAgent: acp.AgentMuse, Slug: "muse-permissions"}); err == nil || !strings.Contains(err.Error(), ":auto-review") {
				t.Fatalf("unsupported native profile was substituted: %v", err)
			}
			settings, err := os.ReadFile(filepath.Join(config, "settings.json"))
			if err != nil || string(settings) != unsupported {
				t.Fatalf("native permission settings changed: %q, %v", settings, err)
			}
		})
	}
}
