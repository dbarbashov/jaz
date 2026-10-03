package drive

import "github.com/wins/jaz/backend/pkg/integrations"

const (
	ProviderID   = "google_drive"
	RemoteMCPURL = "https://drivemcp.googleapis.com/mcp/v1"
)

var OAuthScopes = []string{
	"https://www.googleapis.com/auth/drive.readonly",
	"https://www.googleapis.com/auth/drive.file",
}

func Plugin() integrations.Plugin {
	return integrations.Plugin{
		ID:             ProviderID,
		Name:           "Google Drive",
		Description:    "Search, read, and create files with Google's official Drive MCP server. Requires Google Workspace Developer Preview access.",
		Examples:       []string{"Find files in my shared folder", "Summarize a document in Google Drive"},
		Provider:       integrations.Provider{ID: ProviderID, Name: "Google Drive"},
		Category:       "productivity",
		Icon:           integrations.PluginIcon{Kind: integrations.PluginIconKindAsset, Value: ProviderID},
		Auth:           []integrations.AuthOption{{Kind: integrations.AuthKindOAuth, Scopes: OAuthScopes}},
		Capabilities:   []integrations.Capability{integrations.CapabilityAct, integrations.CapabilityMCP},
		MultiAccount:   true,
		RemoteMCP:      &integrations.RemoteMCP{URL: RemoteMCPURL, Status: "available", TokenAuth: true},
		Implementation: integrations.Implementation{Status: "available", Owner: "google"},
	}
}
