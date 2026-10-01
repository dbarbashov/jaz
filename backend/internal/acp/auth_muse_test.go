package acp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMuseNativeAccountLoginAndDisconnect(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	t.Chdir(root)
	relativeExecutable, err := filepath.Rel(root, executable)
	if err != nil {
		t.Fatal(err)
	}
	accountFile := filepath.Join(root, "account.json")
	cfg := AgentConfig{Command: executable, Env: map[string]string{
		"MUSE_CLI":          relativeExecutable,
		"MUSE_CHANNEL":      "test-channel",
		"MUSE_AUTH_PATH":    filepath.Join(root, "native-auth.json"),
		"JAZ_TEST_MUSE_CLI": root,
		"XDG_CONFIG_HOME":   root,
		"META_API_KEY":      "",
	}}
	t.Setenv("META_API_KEY", "")
	t.Setenv("JAZ_ACP_MUSE_API_KEY", "")
	for _, state := range []struct {
		account       string
		authenticated bool
		kind          string
	}{
		{"{\"state\":\"loggedOut\",\"credentialRequired\":true}", false, ""},
		{"{\"state\":\"accountLogin\",\"credentialRequired\":true}", true, AuthKindOAuth},
		{"{\"state\":\"apiKey\",\"credentialRequired\":true}", true, AuthKindAPIKey},
		{"{\"state\":\"loggedOut\",\"credentialRequired\":false}", true, AuthKindNone},
	} {
		if err := os.WriteFile(accountFile, []byte(state.account), 0o600); err != nil {
			t.Fatal(err)
		}
		status := ProbeAgentAuth(AgentMuse, cfg, root, nil)
		if status.Authenticated != state.authenticated || status.AuthKind != state.kind || !status.LoginCommandAvailable {
			t.Fatalf("native account %s => %#v", state.account, status)
		}
	}
	invocation := AgentLoginInvocationFor(AgentMuse, root, AgentAuthConfig{}, "", cfg.Env)
	if invocation.Executable != executable || invocation.Env["XDG_CONFIG_HOME"] != root {
		t.Fatalf("login selected a different native profile: %#v", invocation)
	}
	if err := PrepareAgentLoginInvocation(AgentMuse, AgentAuthConfig{}, root, invocation); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"test-channel", "native-auth.json"} {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("native environment value created a profile directory: %s, %v", name, err)
		}
	}
	if err := DisconnectAgentAuth(t.Context(), AgentMuse, cfg, root, ""); err != nil {
		t.Fatal(err)
	}
	if status := ProbeAgentAuth(AgentMuse, cfg, root, nil); status.Authenticated {
		t.Fatalf("Muse remained signed in: %#v", status)
	}
}

func runFakeMuseCLI(root string) int {
	if len(os.Args) != 2 || os.Getenv("XDG_CONFIG_HOME") != root {
		return 2
	}
	accountFile := filepath.Join(root, "account.json")
	if os.Args[1] == "logout" {
		if err := os.WriteFile(accountFile, []byte("{\"state\":\"loggedOut\",\"credentialRequired\":true}"), 0o600); err != nil {
			return 3
		}
		return 0
	}
	if os.Args[1] != "serve" {
		return 4
	}
	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for _, method := range []string{"initialize", "initialized", "account/read"} {
		var request struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := decoder.Decode(&request); err != nil || request.Method != method {
			return 5
		}
		if method == "initialized" {
			continue
		}
		result := json.RawMessage("{}")
		if method == "account/read" {
			data, err := os.ReadFile(accountFile)
			if err != nil {
				return 6
			}
			result = data
		}
		if err := encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result}); err != nil {
			return 7
		}
	}
	var extra json.RawMessage
	if decoder.Decode(&extra) == nil {
		return 8
	}
	return 0
}
