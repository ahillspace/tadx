package list

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/ahillspace/tadx/internal/output"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/readsource"
)

type Input struct {
	All                                       bool
	Environment, Site, Cursor, Name, SiteRole string
	Limit                                     int
	Catalog                                   bool
}
type PageRequest struct {
	PageNumber, PageSize int
	Name, SiteRole       string
	SnapshotCursor       string
}
type User struct {
	LUID        string `json:"luid"`
	Name        string `json:"name"`
	FullName    string `json:"full_name,omitempty"`
	Email       string `json:"email,omitempty"`
	SiteRole    string `json:"site_role,omitempty"`
	LastLogin   string `json:"last_login,omitempty"`
	AuthSetting string `json:"auth_setting,omitempty"`
	Domain      string `json:"domain,omitempty"`
}
type Page struct {
	Number, Size, Total  int
	Users                []User
	RequestID            string
	SnapshotCursor       string
	SuppressContinuation bool
}
type OutputPage = output.Page
type Output struct {
	Status, Environment, Site string
	Page                      OutputPage
	Users                     []User
	RequestID                 string
	Help                      []string
	Source                    *readsource.Metadata
}
type CompactUser struct {
	LUID     string `json:"luid"`
	Name     string `json:"name"`
	SiteRole string `json:"site_role,omitempty"`
}
type CompactResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Page        OutputPage           `json:"page"`
	Users       []CompactUser        `json:"users"`
	Details     string               `json:"details"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}
type FullResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Page        OutputPage           `json:"page"`
	Users       []User               `json:"users"`
	RequestID   string               `json:"tableau_request_id,omitempty"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

func (o Output) CompactOutput() any {
	items := make([]CompactUser, len(o.Users))
	for i, v := range o.Users {
		items[i] = CompactUser{LUID: v.LUID, Name: v.Name, SiteRole: v.SiteRole}
	}
	return CompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Page: o.Page, Users: items, Details: "--full", Help: o.Help, Source: o.Source}
}
func (o Output) FullOutput() any {
	return FullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Page: o.Page, Users: append([]User(nil), o.Users...), RequestID: o.RequestID, Help: o.Help, Source: o.Source}
}

type Reader interface {
	ListUsers(context.Context, PageRequest) (Page, error)
}
type Action struct{ reader Reader }

func New(reader Reader) *Action { return &Action{reader: reader} }
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if input.All {
		return a.collectAll(ctx, input)
	}
	if a == nil || a.reader == nil {
		return Output{}, errors.New("admin user list reader is not configured")
	}
	fingerprint, err := cursorFingerprint(struct {
		Environment, Site, Name, SiteRole string
		Catalog                           bool
	}{input.Environment, input.Site, input.Name, input.SiteRole, input.Catalog})
	if err != nil {
		return Output{}, err
	}
	number, size, snapshotCursor, err := selectPage(input.Cursor, input.Limit, fingerprint)
	if err != nil {
		return Output{}, err
	}
	page, err := a.reader.ListUsers(ctx, PageRequest{PageNumber: number, PageSize: size, Name: input.Name, SiteRole: input.SiteRole, SnapshotCursor: snapshotCursor})
	if err != nil {
		return Output{}, err
	}
	if page.Number != number || page.Size <= 0 || len(page.Users) > page.Size {
		return Output{}, errors.New("admin user list reader returned inconsistent pagination")
	}
	next := ""
	if !page.SuppressContinuation && (page.SnapshotCursor != "" || page.Number*page.Size < page.Total) {
		next, err = encodeCursor(page.Number+1, page.Size, fingerprint, page.SnapshotCursor)
		if err != nil {
			return Output{}, err
		}
	}
	return Output{Status: "listed", Environment: input.Environment, Site: input.Site, Page: OutputPage{Returned: len(page.Users), Total: page.Total, Limit: page.Size, NextCursor: next, MoreAvailable: next != "" || (page.SuppressContinuation && len(page.Users) < page.Total)}, Users: page.Users, RequestID: page.RequestID, Help: []string{"tadx admin user inspect --id <user-luid>"}}, nil
}

type cursorValue struct {
	Version, Page, Size int
	Filter              string
	Snapshot            string
}

func cursorFingerprint(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return base64.RawURLEncoding.EncodeToString(sum[:]), nil
}
func selectPage(encoded string, requested int, filter string) (int, int, string, error) {
	if encoded == "" {
		if requested == 0 {
			requested = 25
		}
		if requested < 1 || requested > 100 {
			return 0, 0, "", errs.New(errs.KindUsage, "admin user list limit must be between 1 and 100")
		}
		return 1, requested, "", nil
	}
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	var v cursorValue
	if len(encoded) > 2048 || err != nil || json.Unmarshal(data, &v) != nil || v.Version != 1 || v.Page < 2 || v.Size < 1 || v.Size > 100 || v.Filter != filter || len(v.Snapshot) > 1024 {
		return 0, 0, "", errs.New(errs.KindUsage, "invalid admin user list continuation cursor")
	}
	if requested != 0 && requested != v.Size {
		return 0, 0, "", errs.New(errs.KindUsage, "admin user list limit must match the continuation cursor")
	}
	return v.Page, v.Size, v.Snapshot, nil
}
func encodeCursor(page, size int, filter, snapshot string) (string, error) {
	data, err := json.Marshal(cursorValue{Version: 1, Page: page, Size: size, Filter: filter, Snapshot: snapshot})
	return base64.RawURLEncoding.EncodeToString(data), err
}

// collectAll follows private bounded pages and fails closed on incomplete inventories.
func (a *Action) collectAll(ctx context.Context, input Input) (Output, error) {
	if input.Limit != 0 || input.Cursor != "" {
		return Output{}, errs.New(errs.KindUsage, "--all cannot be combined with --limit or --cursor")
	}
	input.All = false
	input.Limit = 100
	var result Output
	seen := map[string]bool{}
	cursors := map[string]bool{}
	for pageNumber := 0; pageNumber < 100; pageNumber++ {
		page, err := a.Execute(ctx, input)
		if err != nil {
			return Output{}, err
		}
		if pageNumber == 0 {
			result = page
			result.Users = nil
		} else if page.Page.Total != result.Page.Total {
			return Output{}, errors.New("admin/user inventory changed during pagination; retry")
		}
		for _, item := range page.Users {
			if item.LUID == "" || seen[item.LUID] {
				return Output{}, errors.New("admin/user inventory returned missing or repeated identities")
			}
			seen[item.LUID] = true
			result.Users = append(result.Users, item)
		}
		result.RequestID = page.RequestID
		if !page.Page.MoreAvailable && page.Page.NextCursor == "" {
			if len(result.Users) != result.Page.Total {
				return Output{}, errors.New("admin/user inventory completeness could not be established")
			}
			result.Page = OutputPage{Returned: len(result.Users), Total: result.Page.Total, Limit: 10000}
			return result, nil
		}
		if len(page.Users) == 0 || page.Page.NextCursor == "" || cursors[page.Page.NextCursor] {
			return Output{}, errors.New("admin/user inventory pagination did not advance; completeness could not be established")
		}
		cursors[page.Page.NextCursor] = true
		input.Cursor = page.Page.NextCursor
	}
	return Output{}, errors.New("admin/user --all exceeds the 10000-record bound; use narrower filters")
}
