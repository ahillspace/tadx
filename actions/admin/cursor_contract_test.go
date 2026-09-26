package admin_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	grouplist "github.com/ahillspace/tadx/actions/admin/group/list"
	userlist "github.com/ahillspace/tadx/actions/admin/user/list"
)

func TestAdminCursorCompatibility(t *testing.T) {
	for _, kind := range []string{"user", "group"} {
		t.Run(kind, func(t *testing.T) {
			filterJSON := `{"Environment":"dev","Site":"site","Name":"","SiteRole":"","Cache":false}`
			if kind == "group" {
				filterJSON = strings.ReplaceAll(filterJSON, "SiteRole", "Domain")
			}
			sum := sha256.Sum256([]byte(filterJSON))
			fingerprint := base64.RawURLEncoding.EncodeToString(sum[:])
			cursor := func(size int, filter, snapshot string) string {
				raw, _ := json.Marshal(struct {
					Version, Page, Size int
					Filter, Snapshot    string
				}{1, 2, size, filter, snapshot})
				return base64.RawURLEncoding.EncodeToString(raw)
			}
			check := func(encoded string, limit int, all bool, site string) (error, error) {
				if kind == "user" {
					in := userlist.Input{Environment: "dev", Site: site, Cursor: encoded, Limit: limit, All: all}
					return userlist.ValidateInput(in), userlist.ValidateContinuation(in)
				}
				in := grouplist.Input{Environment: "dev", Site: site, Cursor: encoded, Limit: limit, All: all}
				return grouplist.ValidateInput(in), grouplist.ValidateContinuation(in)
			}
			for _, tc := range []struct {
				name, token, site string
				limit             int
				all               bool
				shape, bound      string
			}{
				{name: "legacy snapshot", token: cursor(25, fingerprint, "opaque-snapshot"), site: "site"},
				{name: "default limit", site: "site"},
				{name: "maximum initial limit", limit: 10000, site: "site"},
				{name: "changed target", token: cursor(25, fingerprint, ""), site: "other", bound: "invalid admin " + kind + " list continuation cursor"},
				{name: "changed limit", token: cursor(25, fingerprint, ""), limit: 10, site: "site", shape: "invalid admin " + kind + " list continuation cursor", bound: "admin " + kind + " list limit must match the continuation cursor"},
				{name: "oversized continuation", token: cursor(101, fingerprint, ""), site: "site", shape: "invalid admin " + kind + " list continuation cursor", bound: "invalid admin " + kind + " list continuation cursor"},
				{name: "invalid encoding", token: "!", site: "site", shape: "invalid admin " + kind + " list continuation cursor", bound: "invalid admin " + kind + " list continuation cursor"},
				{name: "all conflict", limit: 1, all: true, site: "site", shape: "--all cannot be combined with --limit or --cursor"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					shape, bound := check(tc.token, tc.limit, tc.all, tc.site)
					for _, pair := range []struct {
						err  error
						want string
					}{{shape, tc.shape}, {bound, tc.bound}} {
						if pair.want == "" {
							if pair.err != nil {
								t.Fatal(pair.err)
							}
						} else if pair.err == nil || pair.err.Error() != pair.want {
							t.Fatalf("error=%v want=%q", pair.err, pair.want)
						}
					}
				})
			}
		})
	}
}
