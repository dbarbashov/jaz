package connections

import (
	"context"
	"net/http"

	"github.com/wins/jaz/backend/internal/connectors/drive"
	googleconnector "github.com/wins/jaz/backend/internal/connectors/google"
)

func newDriveOAuthProvider(config googleconnector.OAuthClientConfig) oauthProvider {
	return googleOAuthProvider{
		providerID: drive.ProviderID,
		config:     config,
		scopes:     drive.OAuthScopes,
		verify: func(ctx context.Context, client *http.Client) (oauthIdentity, error) {
			account, err := drive.GetAccount(ctx, client)
			return oauthIdentity{accountID: account.Email, accountName: account.Name}, err
		},
	}
}
