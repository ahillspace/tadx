package app

import (
	"context"
	"net/http"
	"testing"

	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/jobmonitor"
)

type targetIdentitySession struct{}

func (targetIdentitySession) String() string                           { return "fixture session" }
func (targetIdentitySession) Authorize(*http.Request)                  {}
func (targetIdentitySession) SiteLUID() string                         { return "site-luid" }
func (targetIdentitySession) UserLUID() string                         { return "user-luid" }
func (targetIdentitySession) Renew(context.Context, http.Header) error { return nil }

func TestPublicationReceiptConnectionChecksEveryReceiptTarget(t *testing.T) {
	runtime := &runtimeDependencies{}
	connection := authenticatedTableau{environment: config.Environment{URL: "https://TABLEAU.example.test:443/Tableau", SiteContentURL: "site"}, session: targetIdentitySession{}}
	connections := map[string]authenticatedTableau{"dev\x00site\x00https://tableau.example.test/Tableau": connection}
	receipt := jobmonitor.Receipt{Environment: "dev", Server: "https://tableau.example.test/Tableau/", Site: "site", SiteID: "site-luid"}
	if _, err := runtime.receiptConnection(t.Context(), receipt, connections); err != nil {
		t.Fatalf("equivalent cached target rejected: %v", err)
	}
	receipt.SiteID = "another-site-luid"
	if _, err := runtime.receiptConnection(t.Context(), receipt, connections); err == nil {
		t.Fatal("cached connection bypassed the next receipt's exact site identity")
	}
}
