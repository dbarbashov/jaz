package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/provider"
	agentsettings "github.com/wins/jaz/backend/internal/settings"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/testexec"
)

func TestAgentSettingsFastModeFollowsNativeModelCatalogAndAuthProfile(t *testing.T) {
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	first := t.TempDir()
	second := t.TempDir()
	for directory, models := range map[string]string{
		first:  `{"models":[{"slug":"native-fast","additional_speed_tiers":["fast"]},{"slug":"native-standard","additional_speed_tiers":[]},{"slug":"native-unknown"}]}`,
		second: `{"models":[{"slug":"other-account-model","additional_speed_tiers":["fast"]}]}`,
	} {
		if err := os.WriteFile(filepath.Join(directory, "models_cache.json"), []byte(models), 0600); err != nil {
			t.Fatal(err)
		}
	}
	server := &Server{Root: root, Store: store, ModelCatalog: modelcatalog.NewService(nil), AgentCatalog: acp.AgentCatalog{}}
	handler := server.Handler()
	for _, tc := range []struct {
		profile  string
		provider string
		model    string
	}{
		{first, provider.ProviderOpenAI, "native-fast"},
		{second, provider.ProviderOpenAI, "other-account-model"},
		{t.TempDir(), provider.ProviderOpenAI, ""},
		{first, provider.ProviderOpenRouter, ""},
	} {
		server.AgentCatalog[acp.AgentCodex] = acp.AgentConfig{
			Local: true, ModelProvider: tc.provider,
			Auth: acp.AgentAuthConfig{Mode: acp.AuthModeExistingCLI, Path: first},
		}
		_, err := agentsettings.SaveAgentDefaults(store, agentsettings.AgentDefaults{ACP: map[string]agentsettings.ACPAgentDefaults{
			acp.AgentCodex: {ModelProvider: tc.provider, Auth: acp.AgentAuthConfig{Mode: acp.AuthModeExistingCLI, Path: tc.profile}},
		}})
		if err != nil {
			t.Fatal(err)
		}
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/settings/agents", nil))
		if res.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
		}
		var response struct {
			Options map[string]acp.AgentOptions `json:"acp_options"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		option := response.Options[acp.AgentCodex]
		if tc.model == "" {
			if option.FastModeConfigID != "" || len(option.FastModeModels) != 0 {
				t.Fatalf("invented Fast Mode capability: %#v", option)
			}
		} else if option.FastModeConfigID != "fast-mode" || len(option.FastModeModels) != 1 || option.FastModeModels[0] != tc.model {
			t.Fatalf("profile %s: Fast Mode = %#v", tc.profile, option)
		}
	}
}

func TestAgentSettingsEnablesCodexOpenAIAPIKeyWithOpenAIProviderKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir()+"/codex-home")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_APIKEY", "")
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	exe := testexec.Write(t, filepath.Join(root, "codex-acp"), "", "")
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil),
		Store: store,
		Root:  root,
		AgentCatalog: acp.AgentCatalog{
			acp.AgentCodex: {
				Command:                 exe,
				ProviderMode:            acp.AgentProviderModeAgentDefaults,
				ModelProviderCapability: provider.CapabilityResponses,
				ModelProvider:           provider.ProviderOpenAI,
				Model:                   "gpt-5.4-mini",
			},
		},
	}).Handler()

	body := func(providerKeys map[string]any) *strings.Reader {
		return jsonReader(t, map[string]any{
			"acp": map[string]any{
				"codex": map[string]any{
					"enabled":        true,
					"command":        exe,
					"model_provider": acp.CodexProviderOpenAIAPIKey,
					"model":          "gpt-5.4-mini",
				},
			},
			"provider_keys": providerKeys,
		})
	}

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/v1/settings/agents", body(nil))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:1234"
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "cannot be enabled without authentication") {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}

	res = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/v1/settings/agents", body(map[string]any{"openai": "openai-key"}))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:1234"
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestAgentSettingsCodexOpenAIKeyOptionUsesOpenAIConfig(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil),
		Store:        store,
		AgentCatalog: testACPAgentCatalog(nil),
		Providers: provider.StaticSource(map[string]provider.ModelProviderConfig{
			provider.ProviderOpenAI: {APIKey: "configured"},
		}),
	}).Handler()

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/settings/agents", nil)
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var got struct {
		ACPOptions map[string]struct {
			ModelProviders []struct {
				ID         string `json:"id"`
				Configured bool   `json:"configured"`
			} `json:"model_providers"`
		} `json:"acp_options"`
		Providers []struct {
			ID         string `json:"id"`
			Configured bool   `json:"configured"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if codexProviderConfigured(got.Providers, acp.CodexProviderOpenAIAPIKey) {
		t.Fatalf("openai api-key option must not leak into global providers: %#v", got.Providers)
	}
	if !codexProviderConfigured(got.ACPOptions[acp.AgentCodex].ModelProviders, acp.CodexProviderOpenAIAPIKey) {
		t.Fatalf("openai api-key option should inherit openai config: %#v", got.ACPOptions[acp.AgentCodex].ModelProviders)
	}
}

func codexProviderConfigured(providers []struct {
	ID         string `json:"id"`
	Configured bool   `json:"configured"`
}, id string) bool {
	for _, provider := range providers {
		if provider.ID == id {
			return provider.Configured
		}
	}
	return false
}
