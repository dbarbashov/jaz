package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/gluonfield/acp-transport/stdio"
	"github.com/wins/jaz/backend/internal/processenv"
)

func museExecutable(env map[string]string) (string, error) {
	if command := strings.TrimSpace(env["MUSE_CLI"]); command != "" {
		resolved, err := ResolveExecutable(command)
		if err != nil {
			return "", err
		}
		return filepath.Abs(resolved)
	}
	if command, err := executableInPath("muse", env["PATH"]); err == nil {
		return filepath.Abs(command)
	}
	dir := env["MUSE_INSTALL_DIR"]
	if dir == "" {
		dir = filepath.Join(env["HOME"], ".local", "bin")
		if runtime.GOOS == "windows" {
			dir = filepath.Join(env["LOCALAPPDATA"], "Programs", "muse")
		}
	}
	return resolveLoginExecutable(dir, "muse")
}

func museLoginInvocation(env map[string]string) AgentLoginInvocation {
	invocation := AgentLoginInvocation{
		Args:        []string{"login"},
		Display:     "muse login",
		InheritHome: true,
	}
	command, err := museExecutable(env)
	invocation.Available = err == nil
	if err != nil {
		invocation.Reason = "Muse Code executable (muse) not found"
		return invocation
	}
	invocation.Executable = command
	invocation.Display = shellCommand(command, "login")
	invocation.Reason = ""
	invocation.Env = map[string]string{}
	for key, value := range env {
		if strings.HasPrefix(key, "MUSE_") || key == "META_API_KEY" || key == "XDG_CONFIG_HOME" || key == "XDG_DATA_HOME" {
			invocation.Env[key] = value
		}
	}
	return invocation
}

func resolveMuseAuth(root string, env map[string]string) resolvedAgentAuth {
	status := resolvedAgentAuth{
		Config: AgentAuthConfig{Mode: AuthModeExistingCLI},
		Source: AuthModeExistingCLI,
	}
	if status.resolveAPIKey(AgentMuse, root, env) {
		status.markAuthenticated("api_key_env", AuthKindAPIKey)
		return status
	}
	if value := strings.TrimSpace(env["META_API_KEY"]); value != "" {
		status.APIKeyValue = value
		status.APIKeySet = true
		status.markAuthenticated("api_key_env", AuthKindAPIKey)
		return status
	}
	command, err := museExecutable(env)
	if err != nil {
		status.Reason = "Muse Code executable (muse) not found; install it from https://dev.meta.ai/docs/muse-code"
		return status
	}
	state, err := museAccountState(command, env)
	if err != nil {
		status.Reason = "Read Muse account status: " + err.Error()
		return status
	}
	switch state.State {
	case "accountLogin":
		status.markAuthenticated("muse_account", AuthKindOAuth)
	case "apiKey", "envKey":
		status.markAuthenticated("muse_account", AuthKindAPIKey)
	default:
		if state.CredentialRequired != nil && !*state.CredentialRequired {
			status.markAuthenticated("no_credential_required", AuthKindNone)
		} else {
			status.Reason = "Sign in with muse login or set JAZ_ACP_MUSE_API_KEY"
		}
	}
	return status
}

type museAccount struct {
	State              string `json:"state"`
	CredentialRequired *bool  `json:"credentialRequired"`
}

func museAccountState(command string, env map[string]string) (museAccount, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, command, "serve")
	cmd.Env = processenv.List(env)
	cmd.WaitDelay = time.Second
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return museAccount{}, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return museAccount{}, err
	}
	if err := cmd.Start(); err != nil {
		return museAccount{}, err
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	peer := jsonrpc.NewPeer(stdio.New(stdout, stdin), nil)
	defer peer.Close()
	go func() {
		_ = peer.Serve(ctx)
	}()
	if _, err := peer.Call(ctx, "initialize", map[string]any{
		"clientInfo":   map[string]string{"name": "jaz_auth", "version": "0.1.0"},
		"capabilities": map[string]bool{"experimentalApi": true},
	}); err != nil {
		return museAccount{}, err
	}
	if err := peer.Notify(ctx, "initialized", map[string]any{}); err != nil {
		return museAccount{}, err
	}
	raw, err := peer.Call(ctx, "account/read", map[string]any{})
	if err != nil {
		return museAccount{}, err
	}
	var state museAccount
	if err := json.Unmarshal(raw, &state); err != nil {
		return museAccount{}, fmt.Errorf("decode Muse account: %w", err)
	}
	return state, nil
}
