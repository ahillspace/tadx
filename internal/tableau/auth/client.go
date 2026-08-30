// Package auth implements Tableau PAT sign-in against the released REST API.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/tableau"
)

// Client signs in with PAT credentials through the shared transport.
type Client struct{ transport *tableau.Transport }

// NewClient creates a PAT sign-in client.
func NewClient(transport *tableau.Transport) *Client { return &Client{transport: transport} }

// SignIn implements auth.Signer.
func (c *Client) SignIn(ctx context.Context, input coreauth.SignInRequest) (coreauth.SignInResponse, error) {
	if c == nil || c.transport == nil {
		return coreauth.SignInResponse{}, errors.New("Tableau sign-in client is not configured")
	}
	payload := struct {
		Credentials struct {
			PATName   string `json:"personalAccessTokenName"`
			PATSecret string `json:"personalAccessTokenSecret"`
			Site      struct {
				ContentURL string `json:"contentUrl"`
			} `json:"site"`
		} `json:"credentials"`
	}{}
	payload.Credentials.PATName = input.PATName
	payload.Credentials.PATSecret = input.PATSecret
	payload.Credentials.Site.ContentURL = input.SiteContentURL
	body, err := json.Marshal(payload)
	if err != nil {
		return coreauth.SignInResponse{}, fmt.Errorf("encode PAT sign-in request: %w", err)
	}
	response, err := c.transport.Do(ctx, nil, tableau.Request{
		Method: http.MethodPost, ServerURL: input.ServerURL,
		Path: "/api/" + c.transport.APIVersion() + "/auth/signin", Operation: "auth.check",
		Body: body, ContentType: "application/json", Accept: "application/json", Secrets: []string{input.PATName, input.PATSecret},
	})
	if err != nil {
		return coreauth.SignInResponse{}, err
	}
	var envelope struct {
		Credentials struct {
			Token string `json:"token"`
			Site  struct {
				ID string `json:"id"`
			} `json:"site"`
			User struct {
				ID string `json:"id"`
			} `json:"user"`
		} `json:"credentials"`
	}
	if err := json.Unmarshal(response.Body, &envelope); err != nil {
		return coreauth.SignInResponse{}, fmt.Errorf("decode PAT sign-in response: %w", err)
	}
	if envelope.Credentials.Token == "" || envelope.Credentials.Site.ID == "" {
		return coreauth.SignInResponse{}, errors.New("PAT sign-in response omitted token or site LUID")
	}
	return coreauth.SignInResponse{Token: envelope.Credentials.Token, SiteLUID: envelope.Credentials.Site.ID, UserLUID: envelope.Credentials.User.ID}, nil
}
