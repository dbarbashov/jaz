package mcpconfig

const OAuthCallbackPath = "/v1/mcp/oauth/callback"

func OAuthConnectionID(serverID string) string {
	return "mcp:" + serverID
}

func (s Server) TokenID() string {
	if s.TokenConnectionID != "" {
		return s.TokenConnectionID
	}
	return OAuthConnectionID(s.ID)
}
