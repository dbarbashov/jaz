package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/tools"
)

type appQueryInput struct {
	Query string `json:"query"`
}

func TestManagerServesMCPAppEntrypoints(t *testing.T) {
	remote := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "tasks",
		Version: "1.0.0",
		Icons:   []mcpsdk.Icon{{Source: "data:image/svg+xml;base64,PHN2Zy8+", MIMEType: "image/svg+xml"}},
	}, nil)
	for _, uri := range []string{"ui://tasks/app", "ui://tasks/tray", "ui://tasks/viewer"} {
		remote.AddResource(&mcpsdk.Resource{URI: uri, Name: uri, MIMEType: AppMIMEType}, func(_ context.Context, req *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
			return &mcpsdk.ReadResourceResult{Contents: []*mcpsdk.ResourceContents{{URI: req.Params.URI, MIMEType: AppMIMEType, Text: "<div id=" + req.Params.URI + "></div>"}}}, nil
		})
	}
	answer := func(ctx context.Context, req *mcpsdk.CallToolRequest, input appQueryInput) (*mcpsdk.CallToolResult, any, error) {
		return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: req.Params.Name + ":" + input.Query}}}, nil, nil
	}
	entry := func(uri string, entrypoints ...map[string]any) mcpsdk.Meta {
		list := make([]any, len(entrypoints))
		for i, e := range entrypoints {
			list[i] = e
		}
		return mcpsdk.Meta{"ui": map[string]any{"resourceUri": uri}, "openai/ui": map[string]any{"entrypoints": list}}
	}
	mcpsdk.AddTool(remote, &mcpsdk.Tool{Name: "library", Title: "Library", Meta: entry("ui://tasks/app", map[string]any{"type": "global"})}, answer)
	tray := entry("ui://tasks/tray", map[string]any{"type": "thread"})
	tray["ui"] = map[string]any{"resourceUri": "ui://tasks/tray", "visibility": []string{"model"}}
	mcpsdk.AddTool(remote, &mcpsdk.Tool{Name: "tray", Annotations: &mcpsdk.ToolAnnotations{Title: "Tray"}, Meta: tray}, answer)
	mcpsdk.AddTool(remote, &mcpsdk.Tool{Name: "viewer", Meta: entry("ui://tasks/viewer", map[string]any{"type": "file", "extensions": []string{".STL", "step"}}, map[string]any{"type": "sidebar"})}, answer)
	mcpsdk.AddTool(remote, &mcpsdk.Tool{Name: "search", Meta: mcpsdk.Meta{"ui": map[string]any{"resourceUri": "ui://tasks/app"}}}, answer)
	mcpsdk.AddTool(remote, &mcpsdk.Tool{Name: "graphql", Meta: mcpsdk.Meta{"ui": map[string]any{"visibility": []string{"app"}}}}, answer)
	mcpsdk.AddTool(remote, &mcpsdk.Tool{Name: "admin", Meta: mcpsdk.Meta{"ui": map[string]any{"visibility": []string{"model"}}}}, answer)
	httpServer := httptest.NewServer(mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server {
		return remote
	}, &mcpsdk.StreamableHTTPOptions{JSONResponse: true}))
	defer httpServer.Close()

	store := &testStore{servers: []mcpconfig.Server{{
		ID:        "srv1",
		Name:      "Tasks",
		Transport: mcpconfig.TransportStreamableHTTP,
		URL:       httpServer.URL,
		Enabled:   true,
	}}}
	registry := tools.NewRegistry()
	manager := NewManager(store, nil, registry, log.New(io.Discard))
	defer manager.Close()
	early, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if points, err := manager.Entrypoints(early); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Entrypoints before the first refresh = %#v, %v; want it to wait", points, err)
	}
	manager.Refresh(context.Background())

	var agentTools []string
	for _, def := range registry.Definitions() {
		agentTools = append(agentTools, tools.DefinitionName(def))
	}
	if got := strings.Join(agentTools, ","); strings.Contains(got, "graphql") || !strings.Contains(got, "search") || !strings.Contains(got, "admin") {
		t.Fatalf("agent tools = %s, want search and admin without the app-only graphql", got)
	}

	icon := "data:image/svg+xml;base64,PHN2Zy8+"
	want := []Entrypoint{
		{ServerID: "srv1", Tool: "library", Type: "global", Title: "Library", Icon: icon},
		{ServerID: "srv1", Tool: "tray", Type: "thread", Title: "Tray", Icon: icon},
		{ServerID: "srv1", Tool: "viewer", Type: "file", Title: "viewer", Icon: icon, Extensions: []string{".stl"}},
	}
	points, err := manager.Entrypoints(context.Background())
	if err != nil || !reflect.DeepEqual(points, want) {
		t.Fatalf("entrypoints = %#v, %v\nwant %#v", points, err, want)
	}
	for tool, doc := range map[string]string{"library": "<div id=ui://tasks/app></div>", "tray": "<div id=ui://tasks/tray></div>", "search": "<div id=ui://tasks/app></div>"} {
		if html, err := manager.ReadApp(context.Background(), "srv1", tool); err != nil || html != doc {
			t.Fatalf("ReadApp(%s) = %q, %v", tool, html, err)
		}
	}
	if _, err := manager.ReadApp(context.Background(), "srv1", "admin"); err != ErrAppNotFound {
		t.Fatalf("ReadApp(admin) err = %v", err)
	}

	for _, tool := range []string{"graphql", "tray", "library"} {
		result, err := manager.CallAppTool(context.Background(), "srv1", tool, json.RawMessage(`{"query":"q"}`), nil)
		if err != nil || result.Content[0].(*mcpsdk.TextContent).Text != tool+":q" {
			t.Fatalf("app calling %s: %+v %v", tool, result, err)
		}
	}
	if _, err := manager.CallAppTool(context.Background(), "srv1", "admin", nil, nil); !errors.Is(err, ErrAppToolDenied) {
		t.Fatalf("app calling a model-only tool: err = %v", err)
	}

	// A tool the server adds after the catalog loaded, as a new deploy of its
	// app does, is found when the app first calls it.
	mcpsdk.AddTool(remote, &mcpsdk.Tool{Name: "rename"}, answer)
	if result, err := manager.CallAppTool(context.Background(), "srv1", "rename", json.RawMessage(`{"query":"q"}`), nil); err != nil || result.Content[0].(*mcpsdk.TextContent).Text != "rename:q" {
		t.Fatalf("app calling a tool added since the catalog loaded: %+v %v", result, err)
	}

	if _, err := manager.ReadApp(context.Background(), "missing", "library"); err != ErrAppNotFound {
		t.Fatalf("ReadApp(missing) err = %v", err)
	}
}

type recordedEvents struct {
	mu                  sync.Mutex
	appended, published []sessionevents.Event
	err                 error
}

func (r *recordedEvents) AppendSessionEvents(_ string, events ...sessionevents.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	for i := range events {
		events[i].Seq = int64(len(r.appended) + i + 1)
	}
	r.appended = append(r.appended, events...)
	return nil
}

func (r *recordedEvents) Publish(event sessionevents.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.published = append(r.published, event)
}

func TestProxyShowsOnlyExplicitMCPAppsInCallersThread(t *testing.T) {
	remote := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "tasks", Version: "1.0.0"}, nil)
	card := mcpsdk.Meta{"ui": map[string]any{"resourceUri": "ui://tasks/issue"}}
	create := func(_ context.Context, req *mcpsdk.CallToolRequest, input appQueryInput) (*mcpsdk.CallToolResult, map[string]string, error) {
		return &mcpsdk.CallToolResult{Meta: mcpsdk.Meta{"color": "#f2c94c"}, IsError: input.Query == "fail"}, map[string]string{"identifier": "AUG-" + input.Query}, nil
	}
	mcpsdk.AddTool(remote, &mcpsdk.Tool{Name: "create_issue", Meta: card}, create)
	mcpsdk.AddTool(remote, &mcpsdk.Tool{Name: "show_selection", Title: "Selected tasks", Meta: card, Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true}},
		func(ctx context.Context, req *mcpsdk.CallToolRequest, input appQueryInput) (*mcpsdk.CallToolResult, map[string]string, error) {
			result, out, err := create(ctx, req, input)
			uri := "ui://tasks/issue"
			if input.Query == "wrong-resource" {
				uri = "ui://other/app"
			}
			result.Content = []mcpsdk.Content{&mcpsdk.ResourceLink{URI: uri, Name: "Tasks", MIMEType: AppMIMEType}}
			return result, out, err
		})
	mcpsdk.AddTool(remote, &mcpsdk.Tool{Name: "show_issue", Meta: mcpsdk.Meta{
		"ui":        map[string]any{"resourceUri": "ui://tasks/issue"},
		"openai/ui": map[string]any{"entrypoints": []map[string]any{{"type": "thread"}}},
	}}, create)
	mcpsdk.AddTool(remote, &mcpsdk.Tool{Name: "list_issues"}, create)
	httpServer := httptest.NewServer(mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server {
		return remote
	}, &mcpsdk.StreamableHTTPOptions{JSONResponse: true}))
	defer httpServer.Close()
	events := &recordedEvents{}
	manager := NewManager(&testStore{servers: []mcpconfig.Server{{
		ID: "srv1", Name: "Tasks", Transport: mcpconfig.TransportStreamableHTTP, URL: httpServer.URL, Enabled: true,
	}}}, nil, tools.NewRegistry(), log.New(io.Discard), WithSessionEvents(events, events))
	defer manager.Close()
	manager.Refresh(context.Background())
	proxy := httptest.NewServer(manager.Handler())
	defer proxy.Close()

	connect := func(headers ...mcpconfig.Header) *mcpsdk.ClientSession {
		t.Helper()
		session, err := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "agent", Version: "1.0.0"}, nil).Connect(context.Background(), &mcpsdk.StreamableClientTransport{
			Endpoint: proxy.URL, HTTPClient: &http.Client{Transport: headerTransport{headers: headers}}, MaxRetries: -1, DisableStandaloneSSE: true,
		}, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = session.Close() })
		return session
	}
	call := func(session *mcpsdk.ClientSession, tool, query string) *mcpsdk.CallToolResult {
		t.Helper()
		result, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: "mcp_Tasks_" + tool, Arguments: map[string]any{"query": query}})
		if err != nil {
			t.Fatal(err)
		}
		if result.IsError != (query == "fail") || result.Meta["color"] != "#f2c94c" || fmt.Sprint(result.StructuredContent) != "map[identifier:AUG-"+query+"]" {
			t.Fatalf("proxy changed %s result: %+v", tool, result)
		}
		return result
	}
	agent := connect(mcpconfig.Header{Name: mcpsession.HeaderName, Value: "thread-1"})
	catalog, err := agent.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range catalog.Tools {
		if spec.Name == "mcp_Tasks_show_selection" && (readToolMeta(spec).UI.ResourceURI != "ui://tasks/issue" || spec.Title != "Selected tasks" || spec.Annotations == nil || !spec.Annotations.ReadOnlyHint || spec.OutputSchema == nil) {
			t.Fatalf("proxy lost presentation metadata: %+v", spec)
		}
	}
	call(agent, "create_issue", "12")
	if result := call(agent, "show_issue", "12"); len(result.Content) != 1 {
		t.Fatal("an app launcher without an explicit resource claimed inline delivery")
	}
	call(agent, "list_issues", "13")
	call(agent, "create_issue", "fail")
	call(agent, "show_issue", "fail")
	call(connect(), "create_issue", "14")
	call(connect(), "show_issue", "14")
	shown := call(agent, "show_selection", "15")
	if len(shown.Content) != 2 || !strings.Contains(shown.Content[1].(*mcpsdk.TextContent).Text, "Jaz has presented") {
		t.Fatalf("agent did not receive presentation feedback: %+v", shown)
	}
	call(agent, "show_selection", "fail")
	if result := call(agent, "show_selection", "wrong-resource"); len(result.Content) != 1 {
		t.Fatal("an unrelated resource was treated as the tool's UI")
	}
	manager.AppVisible = func(string) bool { return false }
	private := call(agent, "show_selection", "16")
	if !strings.Contains(private.Content[1].(*mcpsdk.TextContent).Text, "has not displayed") {
		t.Fatalf("private turn claimed public delivery: %+v", private)
	}
	manager.AppVisible = nil
	events.err = errors.New("disk full")
	failed := call(agent, "show_selection", "17")
	if !strings.Contains(failed.Content[1].(*mcpsdk.TextContent).Text, "could not save") || failed.IsError {
		t.Fatalf("display failure lost the successful tool result: %+v", failed)
	}

	events.mu.Lock()
	defer events.mu.Unlock()
	if len(events.appended) != 2 || len(events.published) != 2 || events.published[1].Seq != events.appended[1].Seq {
		t.Fatalf("only successful presentations open apps: appended %d, published %d", len(events.appended), len(events.published))
	}
	event := events.appended[0]
	var result mcpsdk.CallToolResult
	if err := json.Unmarshal(event.MCPApp.Result, &result); err != nil {
		t.Fatal(err)
	}
	if event.SessionID != "thread-1" || event.Type != sessionevents.TypeMCPApp || event.MCPApp.ServerID != "srv1" || event.MCPApp.Tool != "show_issue" || event.MCPApp.Presented || !events.appended[1].MCPApp.Presented ||
		string(event.MCPApp.Arguments) != `{"query":"12"}` || result.Meta["color"] != "#f2c94c" || fmt.Sprint(result.StructuredContent) != "map[identifier:AUG-12]" {
		t.Fatalf("event = %+v, result = %+v", event, result)
	}
	if err := json.Unmarshal(events.appended[1].MCPApp.Result, &result); err != nil || len(result.Content) != 1 {
		t.Fatalf("host feedback leaked into the app's payload: %+v, %v", result, err)
	}
}
