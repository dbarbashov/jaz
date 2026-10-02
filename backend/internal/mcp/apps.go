package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/sessionevents"
)

// AppMIMEType marks an MCP Apps UI resource (the io.modelcontextprotocol/ui
// extension): a self-contained HTML document the host renders in a sandbox.
const AppMIMEType = "text/html;profile=mcp-app"

var (
	ErrAppNotFound   = errors.New("mcp app not found")
	ErrAppToolDenied = errors.New("tool is not available to apps")
)

// Entrypoint is an MCP App a person opens directly rather than through the
// model, as OpenAI's MCP extensions declare in _meta["openai/ui"]: a
// fullscreen app in the sidebar ("global"), a tab in a thread's side panel
// ("thread"), or a viewer for files with the given extensions ("file").
type Entrypoint struct {
	ServerID   string   `json:"server_id"`
	Tool       string   `json:"tool"`
	Type       string   `json:"type"`
	Title      string   `json:"title"`
	Icon       string   `json:"icon,omitempty"`
	Extensions []string `json:"extensions,omitempty"`
}

// toolMeta is what a tool's _meta declares for MCP Apps: the UI resource it
// opens and who may call it, and where OpenAI's MCP extensions open it.
type toolMeta struct {
	UI struct {
		ResourceURI string   `json:"resourceUri"`
		Visibility  []string `json:"visibility"`
	} `json:"ui"`
	OpenAI struct {
		Entrypoints []struct {
			Type       string   `json:"type"`
			Extensions []string `json:"extensions"`
		} `json:"entrypoints"`
	} `json:"openai/ui"`
}

// readToolMeta decodes a tool's _meta; a malformed field reads as unset.
func readToolMeta(tool *mcpsdk.Tool) toolMeta {
	var meta toolMeta
	if data, err := json.Marshal(tool.Meta); err == nil {
		_ = json.Unmarshal(data, &meta)
	}
	return meta
}

// visibleTo reports whether "model" or "app" may call the tool; both may when
// its visibility is unset.
func (m toolMeta) visibleTo(audience string) bool {
	return m.UI.Visibility == nil || slices.Contains(m.UI.Visibility, audience)
}

// serverApps is what a server's tools declare for MCP Apps: the UI resource
// each linked tool opens, its entrypoints, and the tools apps may call.
type serverApps struct {
	serverID    string
	icon        string
	uris        map[string]string
	entrypoints []Entrypoint
	callable    map[string]bool
}

func newServerApps(serverID string, init *mcpsdk.InitializeResult) *serverApps {
	apps := &serverApps{serverID: serverID, uris: map[string]string{}, callable: map[string]bool{}}
	if init != nil && init.ServerInfo != nil && len(init.ServerInfo.Icons) > 0 {
		apps.icon = init.ServerInfo.Icons[0].Source
	}
	return apps
}

// add records what one tool declares. An entrypoint's tool is callable by the
// host whatever its visibility, as opening the entrypoint calls it.
func (a *serverApps) add(tool *mcpsdk.Tool, meta toolMeta) {
	if meta.visibleTo("app") {
		a.callable[tool.Name] = true
	}
	if meta.UI.ResourceURI == "" {
		return
	}
	a.uris[tool.Name] = meta.UI.ResourceURI
	for _, declared := range meta.OpenAI.Entrypoints {
		point := Entrypoint{ServerID: a.serverID, Tool: tool.Name, Type: declared.Type, Title: toolTitle(tool), Icon: a.icon}
		switch declared.Type {
		case "global", "thread":
		case "file":
			for _, ext := range declared.Extensions {
				if strings.HasPrefix(ext, ".") {
					point.Extensions = append(point.Extensions, strings.ToLower(ext))
				}
			}
			if len(point.Extensions) == 0 {
				continue
			}
		default:
			continue
		}
		a.entrypoints = append(a.entrypoints, point)
		a.callable[tool.Name] = true
	}
}

func toolTitle(tool *mcpsdk.Tool) string {
	if tool.Title != "" {
		return tool.Title
	}
	if tool.Annotations != nil && tool.Annotations.Title != "" {
		return tool.Annotations.Title
	}
	return tool.Name
}

// Entrypoints lists the MCP Apps people can open from connected servers. It
// waits for the first full refresh, so a client asking as Jaz starts gets
// them instead of an empty list.
func (m *Manager) Entrypoints(ctx context.Context) ([]Entrypoint, error) {
	select {
	case <-m.refreshed:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	servers, err := m.store.ListMCPServers()
	if err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []Entrypoint{}
	for _, server := range servers {
		if session := m.sessions[server.ID]; session != nil {
			out = append(out, session.apps.entrypoints...)
		}
	}
	return out, nil
}

// ReadApp returns the HTML document of the MCP App a server's tool opens.
func (m *Manager) ReadApp(ctx context.Context, serverID, tool string) (string, error) {
	session, uri, err := m.appSession(serverID, tool)
	if err != nil {
		return "", err
	}
	result, err := session.readResource(ctx, uri)
	if err != nil {
		return "", err
	}
	for _, content := range result.Contents {
		if content.MIMEType != AppMIMEType {
			continue
		}
		if content.Text != "" {
			return content.Text, nil
		}
		if len(content.Blob) > 0 {
			return string(content.Blob), nil
		}
	}
	return "", fmt.Errorf("mcp app %s has no %s content", uri, AppMIMEType)
}

// CallAppTool relays a tool call from an app, or from the host opening an
// entrypoint, to the app's own server. Only tools visible to apps and
// entrypoint tools may be called.
func (m *Manager) CallAppTool(ctx context.Context, serverID, name string, arguments json.RawMessage, meta mcpsdk.Meta) (*mcpsdk.CallToolResult, error) {
	session := m.session(serverID)
	if session == nil || len(session.apps.uris) == 0 {
		return nil, ErrAppNotFound
	}
	// A tool the catalog has never seen may have come with a new deploy of
	// the server's app, so the catalogs reload once before it is refused.
	if !session.apps.callable[name] && !slices.ContainsFunc(session.tools, func(tool remoteTool) bool { return tool.remoteName == name }) {
		m.Refresh(context.WithoutCancel(ctx))
		session = m.session(serverID)
	}
	if session == nil || !session.apps.callable[name] {
		return nil, fmt.Errorf("%w: %s", ErrAppToolDenied, name)
	}
	return session.callTool(ctx, &mcpsdk.CallToolParams{Name: name, Arguments: arguments, Meta: meta})
}

type sessionEventAppender interface {
	AppendSessionEvents(id string, events ...sessionevents.Event) error
}

type sessionEventPublisher interface {
	Publish(event sessionevents.Event)
}

// WithSessionEvents shows explicitly opened apps in the calling agent's thread.
func WithSessionEvents(store sessionEventAppender, bus sessionEventPublisher) Option {
	return func(m *Manager) {
		m.eventStore = store
		m.eventBus = bus
	}
}

// proxyCall preserves the remote result and opens entrypoint apps. Linked
// resources on ordinary tools remain part of the tool's result, without
// opening an app for every lookup in a batch.
func (m *Manager) proxyCall(tool remoteTool) mcpsdk.ToolHandler {
	return func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		result, err := tool.callRaw(ctx, req)
		if err == nil && !result.IsError {
			m.showApp(mcpsession.SessionID(req), tool, req.Params.Arguments, result)
		}
		return result, err
	}
}

func (m *Manager) showApp(sessionID string, tool remoteTool, arguments json.RawMessage, result *mcpsdk.CallToolResult) {
	session := m.session(tool.serverID)
	if m.eventStore == nil || sessionID == "" || session == nil || !slices.ContainsFunc(session.apps.entrypoints, func(point Entrypoint) bool {
		return point.Tool == tool.remoteName
	}) {
		return
	}
	data, err := json.Marshal(result)
	if err != nil {
		return
	}
	events := []sessionevents.Event{{
		SessionID: sessionID,
		Type:      sessionevents.TypeMCPApp,
		MCPApp:    &sessionevents.MCPAppEvent{ServerID: tool.serverID, Tool: tool.remoteName, Arguments: arguments, Result: data},
		At:        time.Now().UTC(),
	}}
	// AppendSessionEvents assigns Seq in place; publish the stored event.
	if m.eventStore.AppendSessionEvents(sessionID, events...) == nil {
		m.eventBus.Publish(events[0])
	}
}

func (m *Manager) appSession(serverID, tool string) (*serverSession, string, error) {
	session := m.session(serverID)
	if session == nil || session.apps.uris[tool] == "" {
		return nil, "", ErrAppNotFound
	}
	return session, session.apps.uris[tool], nil
}

func (m *Manager) session(serverID string) *serverSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[serverID]
}
