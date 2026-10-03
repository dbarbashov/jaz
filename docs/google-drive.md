# Google Drive connection

Google Drive in Settings → Connections signs in with Google OAuth (PKCE and offline access), then exposes Google's official Streamable HTTP MCP at `https://drivemcp.googleapis.com/mcp/v1`. Each account has its own alias and refreshable grant. Tool definitions come from Google.

Google currently requires Workspace Developer Preview membership and the Drive API and Drive MCP API enabled in the OAuth client's Cloud project. Shared items remain subject to the signed-in account's permissions and Google's MCP file eligibility policies. The official server supports shared-item and folder queries; access to a particular Workspace shared drive must be verified with that account.

The default uses Jaz's bundled Google desktop client. To use a preview-enabled project, set both `JAZ_GOOGLE_DRIVE_OAUTH_CLIENT_ID` and `JAZ_GOOGLE_DRIVE_OAUTH_CLIENT_SECRET` on the Jaz server. Desktop OAuth clients accept loopback callbacks. For hosted Jaz, use a web OAuth client and register the server's `/v1/connections/oauth/callback` URL, with the public base URL configured as described in [remote-backend.md](remote-backend.md).

Scopes are `drive.readonly` for existing accessible files and `drive.file` for files created or explicitly authorized through the app, following Google's MCP setup guide. Disconnect removes the account and its stored credentials from Jaz.

## Sources

- [Google's MCP setup](https://developers.google.com/workspace/drive/api/guides/configure-mcp-server)
- [File eligibility](https://developers.google.com/workspace/drive/api/guides/drive-mcp-server-file-eligibility)
- [Search and folder queries](https://developers.google.com/workspace/drive/api/reference/mcp/tools_list/search_files)
- [Official logo and usage](https://developers.google.com/workspace/drive/api/guides/branding)

## Request ledger

- [x] Investigate Workspace/shared-folder support and HTTP OAuth MCP availability.
- [x] Add Google Drive to Connections using the official HTTP MCP and account OAuth.
- [x] Use Google's official product logo.
- [x] Verify the OAuth callback, refresh and disconnect through the production handlers, SQLite store and MCP transport against a controlled provider; verify the rendered Connections UI and Google's actual sign-in URL.
- [x] Run full Go tests and vet, frontend tests/typecheck/lint/dictation, desktop bundle build, a race check and strict code review.
- [ ] Verify access to the user's shared folder after their Google authorization.

The UI check rendered the production Connections screen and detail modal with the official logo and exercised the Connect button. Reverting to the former fixed-token transport makes the connection regression fail. Google account authorization and real shared-folder access remain unverified.
