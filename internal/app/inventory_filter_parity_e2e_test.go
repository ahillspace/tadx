package app_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestInventoryProviderFilterParityThroughCLI(t *testing.T) {
	for _, test := range []struct {
		name, kind, want string
		flags            []string
	}{
		{"workbook", "workbook", "name:eq:Selected,ownerName:eq:Owner,projectName:eq:Project,tags:eq:Tag", []string{"--name", "Selected", "--owner", "Owner", "--project-name", "Project", "--tag", "Tag"}},
		{"datasource", "datasource", "name:eq:Selected,ownerName:eq:Owner,projectName:eq:Project,type:eq:hyper,tags:eq:Tag,updatedAt:gte:2026-09-01T00:00:00Z,updatedAt:lte:2026-09-02T00:00:00Z", []string{"--name", "Selected", "--owner", "Owner", "--project-name", "Project", "--type", "hyper", "--tag", "Tag", "--updated-after", "2026-09-01T00:00:00Z", "--updated-before", "2026-09-02T00:00:00Z"}},
		{"flow", "flow", "name:eq:Selected,ownerName:eq:Owner,projectId:eq:p1,projectName:eq:Project", []string{"--name", "Selected", "--owner", "Owner", "--project-id", "p1", "--project-name", "Project"}},
		{"project false", "project", "name:eq:Selected,parentProjectId:eq:p1,ownerName:eq:Owner,topLevelProject:eq:false", []string{"--name", "Selected", "--parent-id", "p1", "--owner", "Owner", "--top-level=false"}},
		{"project true", "project", "topLevelProject:eq:true", []string{"--top-level=true"}},
		{"user", "user", "name:eq:Selected,siteRole:eq:Viewer", []string{"--name", "Selected", "--site-role", "Viewer"}},
		{"group", "group", "name:eq:Selected,domainName:eq:local", []string{"--name", "Selected", "--domain", "local"}},
		{"after only", "datasource", "updatedAt:gte:2026-09-01T00:00:00Z", []string{"--updated-after", "2026-09-01T00:00:00Z"}},
		{"before only", "datasource", "updatedAt:lte:2026-09-02T00:00:00Z", []string{"--updated-before", "2026-09-02T00:00:00Z"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, all := range []bool{false, true} {
				t.Run(fmt.Sprint(all), func(t *testing.T) {
					filters, exit, output := runFilterCLI(t, test.kind, test.flags, all)
					if exit != 0 {
						t.Fatalf("exit=%d output=%s", exit, output)
					}
					matched := 0
					for _, got := range filters {
						if test.kind == "project" && got == "" {
							continue
						}
						if got != test.want {
							t.Errorf("filter=%q want=%q", got, test.want)
						}
						matched++
					}
					if matched == 0 {
						t.Fatalf("missing selected provider request: filters=%v output=%s", filters, output)
					}
				})
			}
		})
	}
}

func TestInventoryEmptyAndInvalidFiltersThroughCLI(t *testing.T) {
	for _, kind := range []string{"workbook", "datasource", "flow", "project", "user", "group"} {
		for _, value := range []string{"", "One,Two", "One&Two"} {
			for _, all := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%q/%t", kind, value, all), func(t *testing.T) {
					filters, exit, output := runFilterCLI(t, kind, []string{"--name", value}, all)
					if value != "" {
						if exit == 0 || len(filters) != 0 {
							t.Fatalf("invalid filter made resource requests: filters=%v exit=%d output=%s", filters, exit, output)
						}
					} else {
						if exit != 0 || len(filters) == 0 {
							t.Fatalf("empty filter failed: exit=%d output=%s", exit, output)
						}
						for _, filter := range filters {
							if filter != "" {
								t.Fatalf("empty filter=%q", filter)
							}
						}
					}
				})
			}
		}
	}
	for _, flags := range [][]string{
		{"--updated-after", "not-a-timestamp"},
		{"--updated-before", "2026-13-01T00:00:00Z"},
		{"--updated-after", "2026-09-02T00:00:00Z", "--updated-before", "2026-09-01T00:00:00Z"},
	} {
		for _, all := range []bool{false, true} {
			filters, exit, output := runFilterCLI(t, "datasource", flags, all)
			if exit == 0 || len(filters) != 0 {
				t.Fatalf("invalid time bounds made resource requests: flags=%v all=%t filters=%v exit=%d output=%s", flags, all, filters, exit, output)
			}
		}
	}
}

func runFilterCLI(t *testing.T, kind string, flags []string, all bool) ([]string, int, string) {
	t.Helper()
	var mu sync.Mutex
	var filters []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/auth/signin") {
			_, _ = io.WriteString(w, `{"credentials":{"token":"test-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
			return
		}
		resource := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		if resource == kind+"s" {
			mu.Lock()
			filters = append(filters, r.URL.Query().Get("filter"))
			mu.Unlock()
		}
		_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="0"/><%s/></tsResponse>`, r.URL.Query().Get("pageNumber"), r.URL.Query().Get("pageSize"), resource)
	}))
	defer server.Close()
	options := cacheResilienceOptions(t, server)
	root := "content"
	if kind == "user" || kind == "group" {
		root = "admin"
	}
	args := append([]string{root, kind, "list", "--environment", "production"}, flags...)
	if all {
		args = append(args, "--all")
	} else {
		args = append(args, "--limit", "3")
	}
	var output strings.Builder
	exit := app.Run(context.Background(), args, &output, options)
	mu.Lock()
	defer mu.Unlock()
	return append([]string(nil), filters...), exit, output.String()
}
