package cache

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/tableau"
)

// AuthenticatedExecutor binds a fixed Tableau site and session to cache requests.
type AuthenticatedExecutor struct {
	Transport *tableau.Transport
	Session   coreauth.Session
	ServerURL string
	SiteLUID  string
}

func (e AuthenticatedExecutor) Do(ctx context.Context, input Request) (Response, error) {
	if e.Transport == nil || e.Session == nil || strings.TrimSpace(e.ServerURL) == "" || strings.TrimSpace(e.SiteLUID) == "" {
		return Response{}, errors.New("authenticated cache transport is not configured")
	}
	response, err := e.Transport.Do(ctx, e.Session, tableau.Request{
		Method: http.MethodGet, ServerURL: e.ServerURL,
		Path:  fmt.Sprintf("/api/%s/sites/%s%s", e.Transport.APIVersion(), url.PathEscape(e.SiteLUID), input.Path),
		Query: input.Query, Accept: "application/xml", Operation: input.Operation, MaxResponseBytes: input.MaxResponseBytes,
	})
	if err != nil {
		return Response{}, err
	}
	return Response{StatusCode: response.StatusCode, Body: response.Body, TableauRequestID: response.TableauRequestID}, nil
}
