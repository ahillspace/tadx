package list

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/paging"
	"github.com/ahillspace/tadx/internal/readsource"
)

type Input struct {
	All                                     bool
	Environment, Site, Cursor, Name, Domain string
	Limit                                   int
	Cache                                   bool
}
type PageRequest struct {
	PageNumber, PageSize int
	Name, Domain         string
	SnapshotCursor       string
}
type Group struct {
	LUID                string `json:"luid"`
	Name                string `json:"name"`
	Domain              string `json:"domain,omitempty"`
	MinimumSiteRole     string `json:"minimum_site_role,omitempty"`
	GrantLicenseMode    string `json:"grant_license_mode,omitempty"`
	ExternalUserEnabled *bool  `json:"external_user_enabled,omitempty"`
}
type Page struct {
	Number, Size, Total  int
	Groups               []Group
	RequestID            string
	SnapshotCursor       string
	SuppressContinuation bool
}
type OutputPage = output.Page
type Output struct {
	Status, Environment, Site string
	Page                      OutputPage
	Groups                    []Group
	RequestID                 string
	Help                      []string
	Source                    *readsource.Metadata
}
type CompactGroup struct {
	LUID   string `json:"luid"`
	Name   string `json:"name"`
	Domain string `json:"domain"`
}
type CompactResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Page        OutputPage           `json:"page"`
	Groups      []CompactGroup       `json:"groups"`
	Details     string               `json:"details"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}
type FullResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Page        OutputPage           `json:"page"`
	Groups      []Group              `json:"groups"`
	RequestID   string               `json:"tableau_request_id,omitempty"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

func (o Output) CompactOutput() any {
	v := make([]CompactGroup, len(o.Groups))
	for i, x := range o.Groups {
		v[i] = CompactGroup{LUID: x.LUID, Name: x.Name, Domain: x.Domain}
	}
	return CompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Page: o.Page, Groups: v, Details: "--full", Help: o.Help, Source: o.Source}
}
func (o Output) FullOutput() any {
	return FullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Page: o.Page, Groups: append([]Group(nil), o.Groups...), RequestID: o.RequestID, Help: o.Help, Source: o.Source}
}

type Reader interface {
	ListGroups(context.Context, PageRequest) (Page, error)
}
type Action struct{ reader Reader }

func New(r Reader) *Action { return &Action{reader: r} }
func (a *Action) Execute(ctx context.Context, in Input) (Output, error) {
	cursor, err := validateInput(in)
	if err != nil {
		return Output{}, err
	}
	if in.All {
		return a.collectAll(ctx, in)
	}
	if a == nil || a.reader == nil {
		return Output{}, errors.New("admin group list reader is not configured")
	}
	fp, err := paging.AdminCursorFingerprint(struct {
		Environment, Site, Name, Domain string
		Cache                           bool
	}{in.Environment, in.Site, in.Name, in.Domain, in.Cache})
	if err != nil {
		return Output{}, err
	}
	n, s, snapshotCursor, err := selectPage(in.Cursor, in.Limit, fp, cursor)
	if err != nil {
		return Output{}, err
	}
	p, err := a.readPage(ctx, PageRequest{PageNumber: n, PageSize: s, Name: in.Name, Domain: in.Domain, SnapshotCursor: snapshotCursor})
	if err != nil {
		return Output{}, err
	}
	if p.Number != n || p.Size <= 0 || len(p.Groups) > p.Size {
		return Output{}, errors.New("admin group list reader returned inconsistent pagination")
	}
	next := ""
	if !p.SuppressContinuation && (p.SnapshotCursor != "" || p.Number*p.Size < p.Total) {
		next, err = paging.EncodeAdminCursor(p.Number+1, p.Size, fp, p.SnapshotCursor)
		if err != nil {
			return Output{}, err
		}
	}
	return Output{Status: "listed", Environment: in.Environment, Site: in.Site, Page: OutputPage{Returned: len(p.Groups), Total: p.Total, Limit: p.Size, NextCursor: next, MoreAvailable: next != "" || (p.SuppressContinuation && len(p.Groups) < p.Total)}, Groups: p.Groups, RequestID: p.RequestID, Help: listHelp(in.Environment, p.Groups)}, nil
}

func selectPage(encoded string, requested int, filter string, v paging.AdminCursor) (int, int, string, error) {
	if encoded == "" {
		if requested == 0 {
			requested = 25
		}
		if requested < 1 || requested > 10000 {
			return 0, 0, "", errs.New(errs.KindUsage, "admin group list limit must be between 1 and 10000")
		}
		return 1, requested, "", nil
	}
	if v.Filter != filter {
		return 0, 0, "", errs.New(errs.KindUsage, "invalid admin group list continuation cursor")
	}
	if requested != 0 && requested != v.Size {
		return 0, 0, "", errs.New(errs.KindUsage, "admin group list limit must match the continuation cursor")
	}
	return v.Page, v.Size, v.Snapshot, nil
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
	items, err := paging.Collect(ctx, func(ctx context.Context, state paging.State) (paging.Page[Group], error) {
		page, err := a.reader.ListGroups(ctx, PageRequest{PageNumber: state.Number, PageSize: state.Size, SnapshotCursor: state.Token, Name: input.Name, Domain: input.Domain})
		requestID = page.RequestID
		return paging.Page[Group]{Number: page.Number, Size: page.Size, Total: page.Total, Items: page.Groups, Token: page.SnapshotCursor}, err
	}, func(item Group) string { return item.LUID })
	if err != nil {
		return Output{}, err
	}
	return Output{Status: "listed", Environment: input.Environment, Site: input.Site, Groups: items, Page: OutputPage{Returned: len(items), Total: len(items), Limit: 10000}, RequestID: requestID, Help: listHelp(input.Environment, items)}, nil
}

func listHelp(environment string, items []Group) []string {
	if len(items) == 0 {
		return nil
	}
	return []string{commandhint.Environment(environment, "admin", "group", "inspect", "--id", items[0].LUID)}
}

// ValidateInput checks pagination shape; resolved-target cursor binding remains in Execute.
func ValidateInput(input Input) error { _, err := validateInput(input); return err }

func validateInput(input Input) (paging.AdminCursor, error) {
	var cursor paging.AdminCursor
	if input.All {
		if input.Limit != 0 || input.Cursor != "" {
			return cursor, errs.New(errs.KindUsage, "--all cannot be combined with --limit or --cursor")
		}
		return cursor, nil
	}
	if input.Limit < 0 || input.Limit > 10000 {
		return cursor, errs.New(errs.KindUsage, "admin group list limit must be between 1 and 10000")
	}
	if input.Cursor != "" {
		var valid bool
		cursor, valid = paging.DecodeAdminCursor(input.Cursor)
		if !valid || input.Limit != 0 && input.Limit != cursor.Size {
			return cursor, errs.New(errs.KindUsage, "invalid admin group list continuation cursor")
		}
	}
	return cursor, nil
}

// ValidateContinuation binds a private cursor after local environment resolution, before authentication.
func ValidateContinuation(input Input) error {
	fingerprint, err := paging.AdminCursorFingerprint(struct {
		Environment, Site, Name, Domain string
		Cache                           bool
	}{input.Environment, input.Site, input.Name, input.Domain, input.Cache})

	if err != nil {
		return err
	}
	cursor, valid := paging.DecodeAdminCursor(input.Cursor)
	if input.Cursor != "" && !valid {
		return errs.New(errs.KindUsage, "invalid admin group list continuation cursor")
	}
	_, _, _, err = selectPage(input.Cursor, input.Limit, fingerprint, cursor)
	return err
}

func (a *Action) readPage(ctx context.Context, input PageRequest) (Page, error) {
	if input.PageSize <= 100 {
		return a.reader.ListGroups(ctx, input)
	}
	requestID := ""
	window, err := paging.Window(ctx, paging.State{Number: input.PageNumber, Size: input.PageSize, Token: input.SnapshotCursor}, 100, func(ctx context.Context, state paging.State) (paging.Page[Group], error) {
		page, err := a.reader.ListGroups(ctx, PageRequest{PageNumber: state.Number, PageSize: state.Size, SnapshotCursor: state.Token, Name: input.Name, Domain: input.Domain})
		requestID = page.RequestID
		return paging.Page[Group]{Number: page.Number, Size: page.Size, Total: page.Total, Items: page.Groups, Token: page.SnapshotCursor}, err
	}, func(item Group) string { return item.LUID })
	return Page{Number: window.Number, Size: window.Size, Total: window.Total, Groups: window.Items, RequestID: requestID, SuppressContinuation: true}, err
}
