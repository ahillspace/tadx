package list

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/paging"

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
	SiteRole string `json:"site_role"`
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
	if err := ValidateInput(input); err != nil {
		return Output{}, err
	}
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
	page, err := a.readPage(ctx, PageRequest{PageNumber: number, PageSize: size, Name: input.Name, SiteRole: input.SiteRole, SnapshotCursor: snapshotCursor})
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
	return Output{Status: "listed", Environment: input.Environment, Site: input.Site, Page: OutputPage{Returned: len(page.Users), Total: page.Total, Limit: page.Size, NextCursor: next, MoreAvailable: next != "" || (page.SuppressContinuation && len(page.Users) < page.Total)}, Users: page.Users, RequestID: page.RequestID, Help: listHelp(input.Environment, page.Users)}, nil
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
		if requested < 1 || requested > 10000 {
			return 0, 0, "", errs.New(errs.KindUsage, "admin user list limit must be between 1 and 10000")
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
	if a == nil || a.reader == nil {
		return Output{}, errors.New("inventory reader is not configured")
	}
	requestID := ""
	items, err := paging.Collect(ctx, func(ctx context.Context, state paging.State) (paging.Page[User], error) {
		page, err := a.reader.ListUsers(ctx, PageRequest{PageNumber: state.Number, PageSize: state.Size, SnapshotCursor: state.Token, Name: input.Name, SiteRole: input.SiteRole})
		requestID = page.RequestID
		return paging.Page[User]{Number: page.Number, Size: page.Size, Total: page.Total, Items: page.Users, Token: page.SnapshotCursor}, err
	}, func(item User) string { return item.LUID })
	if err != nil {
		return Output{}, err
	}
	return Output{Status: "listed", Environment: input.Environment, Site: input.Site, Users: items, Page: OutputPage{Returned: len(items), Total: len(items), Limit: 10000}, RequestID: requestID, Help: listHelp(input.Environment, items)}, nil
}

func listHelp(environment string, items []User) []string {
	if len(items) == 0 {
		return nil
	}
	return []string{commandhint.Environment(environment, "admin", "user", "inspect", "--id", items[0].LUID)}
}
