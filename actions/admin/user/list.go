package user

import (
	"context"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/paging"
	"github.com/ahillspace/tadx/internal/readsource"
)

type ListInput struct {
	All                                       bool
	Environment, Site, Cursor, Name, SiteRole string
	Limit                                     int
	Cache                                     bool
	cursor                                    paging.AdminCursor
	fingerprint                               string
}
type ListPageRequest struct {
	PageNumber, PageSize int
	Name, SiteRole       string
	SnapshotCursor       string
}
type ListUser struct {
	LUID        string `json:"luid"`
	Name        string `json:"name"`
	FullName    string `json:"full_name,omitempty"`
	Email       string `json:"email,omitempty"`
	SiteRole    string `json:"site_role,omitempty"`
	LastLogin   string `json:"last_login,omitempty"`
	AuthSetting string `json:"auth_setting,omitempty"`
	Domain      string `json:"domain,omitempty"`
}
type ListPage struct {
	Number, Size, Total  int
	Users                []Record
	RequestID            string
	SnapshotCursor       string
	SuppressContinuation bool
}
type ListOutputPage = output.Page
type ListOutput struct {
	Status, Environment, Site string
	Page                      ListOutputPage
	Users                     []ListUser
	RequestID                 string
	Help                      []string
	Source                    *readsource.Metadata
}
type ListCompactUser struct {
	LUID     string `json:"luid"`
	Name     string `json:"name"`
	SiteRole string `json:"site_role"`
}
type ListCompactResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Page        ListOutputPage       `json:"page"`
	Users       []ListCompactUser    `json:"users"`
	Details     string               `json:"details"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}
type ListFullResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Page        ListOutputPage       `json:"page"`
	Users       []ListUser           `json:"users"`
	RequestID   string               `json:"tableau_request_id,omitempty"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

func (o ListOutput) CompactOutput() any {
	items := make([]ListCompactUser, len(o.Users))
	for i, v := range o.Users {
		items[i] = ListCompactUser{LUID: v.LUID, Name: v.Name, SiteRole: v.SiteRole}
	}
	return ListCompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Page: o.Page, Users: items, Details: "--full", Help: o.Help, Source: o.Source}
}
func (o ListOutput) FullOutput() any {
	return ListFullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Page: o.Page, Users: append([]ListUser(nil), o.Users...), RequestID: o.RequestID, Help: o.Help, Source: o.Source}
}

type ListReader interface {
	ListUsers(context.Context, ListPageRequest) (ListPage, error)
}

// List consumes options prepared before connecting to the selected target.
func List(ctx context.Context, reader ListReader, input ListInput) (ListOutput, error) {
	if input.All {
		return collectAll(ctx, reader, input)
	}
	fingerprint := input.fingerprint
	number, size, snapshot := input.cursor.Page, input.cursor.Size, input.cursor.Snapshot
	if input.Cursor == "" {
		var err error
		fingerprint, err = listFingerprint(input)
		if err != nil {
			return ListOutput{}, err
		}
		number, size = 1, input.Limit
		if size == 0 {
			size = 25
		}
	}
	page, err := readListPage(ctx, reader, ListPageRequest{PageNumber: number, PageSize: size, Name: input.Name, SiteRole: input.SiteRole, SnapshotCursor: snapshot})
	if err != nil {
		return ListOutput{}, err
	}
	next := ""
	if !page.SuppressContinuation && (page.SnapshotCursor != "" || page.Number*page.Size < page.Total) {
		next, err = paging.EncodeAdminCursor(page.Number+1, page.Size, fingerprint, page.SnapshotCursor)
		if err != nil {
			return ListOutput{}, err
		}
	}
	items := listProjection(page.Users)
	return ListOutput{Status: "listed", Environment: input.Environment, Site: input.Site, Page: ListOutputPage{Returned: len(items), Total: page.Total, Limit: page.Size, NextCursor: next, MoreAvailable: next != "" || (page.SuppressContinuation && len(items) < page.Total)}, Users: items, RequestID: page.RequestID, Help: listHelp(input.Environment, items)}, nil
}

// collectAll follows private bounded pages and fails closed on incomplete inventories.
func collectAll(ctx context.Context, reader ListReader, input ListInput) (ListOutput, error) {
	requestID := ""
	records, err := paging.Collect(ctx, func(ctx context.Context, state paging.State) (paging.Page[Record], error) {
		page, err := reader.ListUsers(ctx, ListPageRequest{PageNumber: state.Number, PageSize: state.Size, SnapshotCursor: state.Token, Name: input.Name, SiteRole: input.SiteRole})
		requestID = page.RequestID
		return paging.Page[Record]{Number: page.Number, Size: page.Size, Total: page.Total, Items: page.Users, Token: page.SnapshotCursor}, err
	}, func(item Record) string { return item.LUID })
	if err != nil {
		return ListOutput{}, err
	}
	items := listProjection(records)
	return ListOutput{Status: "listed", Environment: input.Environment, Site: input.Site, Users: items, Page: ListOutputPage{Returned: len(items), Total: len(items), Limit: 10000}, RequestID: requestID, Help: listHelp(input.Environment, items)}, nil
}
func listProjection(records []Record) []ListUser {
	items := make([]ListUser, len(records))
	for i, record := range records {
		items[i] = listUser(record)
	}
	return items
}
func listHelp(environment string, items []ListUser) []string {
	if len(items) == 0 {
		return nil
	}
	return []string{commandhint.Environment(environment, "admin", "user", "inspect", "--id", items[0].LUID)}
}

// ValidateListInput parses local pagination once, before environment resolution or authentication.
func ValidateListInput(input *ListInput) error {
	input.cursor = paging.AdminCursor{}
	input.fingerprint = ""
	if input.All {
		if input.Limit != 0 || input.Cursor != "" {
			return errs.New(errs.KindUsage, "--all cannot be combined with --limit or --cursor")
		}
		return nil
	}
	if input.Limit < 0 || input.Limit > 10000 {
		return errs.New(errs.KindUsage, "admin user list limit must be between 1 and 10000")
	}
	if input.Cursor != "" {
		var valid bool
		input.cursor, valid = paging.DecodeAdminCursor(input.Cursor)
		if !valid || input.Limit != 0 && input.Limit != input.cursor.Size {
			return errs.New(errs.KindUsage, "invalid admin user list continuation cursor")
		}
	}
	return nil
}

// ValidateListContinuation binds the already parsed cursor to the locally resolved target before authentication.
func ValidateListContinuation(input *ListInput) error {
	fingerprint, err := listFingerprint(*input)
	if err != nil {
		return err
	}
	if input.Cursor != "" && input.cursor.Filter != fingerprint {
		return errs.New(errs.KindUsage, "invalid admin user list continuation cursor")
	}
	input.fingerprint = fingerprint
	return nil
}
func listFingerprint(input ListInput) (string, error) {
	return paging.AdminCursorFingerprint(struct {
		Environment, Site, Name, SiteRole string
		Cache                             bool
	}{input.Environment, input.Site, input.Name, input.SiteRole, input.Cache})
}
func readListPage(ctx context.Context, reader ListReader, input ListPageRequest) (ListPage, error) {
	if input.PageSize <= 100 {
		return reader.ListUsers(ctx, input)
	}
	requestID := ""
	window, err := paging.Window(ctx, paging.State{Number: input.PageNumber, Size: input.PageSize, Token: input.SnapshotCursor}, 100, func(ctx context.Context, state paging.State) (paging.Page[Record], error) {
		page, err := reader.ListUsers(ctx, ListPageRequest{PageNumber: state.Number, PageSize: state.Size, SnapshotCursor: state.Token, Name: input.Name, SiteRole: input.SiteRole})
		requestID = page.RequestID
		return paging.Page[Record]{Number: page.Number, Size: page.Size, Total: page.Total, Items: page.Users, Token: page.SnapshotCursor}, err
	}, func(item Record) string { return item.LUID })
	return ListPage{Number: window.Number, Size: window.Size, Total: window.Total, Users: window.Items, RequestID: requestID, SuppressContinuation: true}, err
}

func listUser(v Record) ListUser {
	return ListUser{LUID: v.LUID, Name: v.Name, FullName: v.FullName, Email: v.Email, SiteRole: v.SiteRole, LastLogin: v.LastLogin, AuthSetting: v.AuthSetting, Domain: v.Domain}
}
