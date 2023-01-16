package remotewrite

import (
	"context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

type oauth2Client struct {
	token  *oauth2.Token
	config *clientcredentials.Config
}

func NewOAuth2(c Config) *oauth2Client {
	return &oauth2Client{
		config: &clientcredentials.Config{
			ClientID:     c.ClientID,
			ClientSecret: c.ClientSecret,
			TokenURL:     c.AuthUrl,
			Scopes:       []string{c.TenantName},
		},
	}
}

func (a *oauth2Client) GetAuthToken() (string, error) {
	ctx := context.Background()

	if a.token == nil || !a.token.Valid() {
		authToken, err := a.config.Token(ctx)
		if err != nil {
			return "", err
		}
		a.token = authToken
		return a.token.AccessToken, nil
	}
	return a.token.AccessToken, nil
}
