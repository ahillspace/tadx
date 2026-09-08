package list

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/paging"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/readsource"
)

type Input struct {
	All                                     bool
	Environment, Site, Cursor, Name, Domain string
	Limit                                   int
	Catalog                                 bool
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
	if in.All {
		return a.collectAll(ctx, in)
	}
	if a == nil || a.reader == nil {
		return Output{}, errors.New("admin group list reader is not configured")
	}
	fp, err := cursorFingerprint(struct {
		Environment, Site, Name, Domain string
		Catalog                         bool
	}{in.Environment, in.Site, in.Name, in.Domain, in.Catalog})
	if err != nil {
		return Output{}, err
	}
	n, s, snapshotCursor, err := selectPage(in.Cursor, in.Limit, fp)
	if err != nil {
		return Output{}, err
	}
	p, err := a.reader.ListGroups(ctx, PageRequest{PageNumber: n, PageSize: s, Name: in.Name, Domain: in.Domain, SnapshotCursor: snapshotCursor})
	if err != nil {
		return Output{}, err
	}
	if p.Number != n || p.Size <= 0 || len(p.Groups) > p.Size {
		return Output{}, errors.New("admin group list reader returned inconsistent pagination")
	}
	next := ""
	if !p.SuppressContinuation && (p.SnapshotCursor != "" || p.Number*p.Size < p.Total) {
		next, err = encodeCursor(p.Number+1, p.Size, fp, p.SnapshotCursor)
		if err != nil {
			return Output{}, err
		}
	}
	return Output{Status: "listed", Environment: in.Environment, Site: in.Site, Page: OutputPage{Returned: len(p.Groups), Total: p.Total, Limit: p.Size, NextCursor: next, MoreAvailable: next != "" || (p.SuppressContinuation && len(p.Groups) < p.Total)}, Groups: p.Groups, RequestID: p.RequestID, Help: []string{"tadx admin group inspect --id <group-luid>"}}, nil
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
			return 0, 0, "", errs.New(errs.KindUsage, "admin group list limit must be between 1 and 100")
		}
		return 1, requested, "", nil
	}
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	var v cursorValue
	if len(encoded) > 2048 || err != nil || json.Unmarshal(data, &v) != nil || v.Version != 1 || v.Page < 2 || v.Size < 1 || v.Size > 100 || v.Filter != filter || len(v.Snapshot) > 1024 {
		return 0, 0, "", errs.New(errs.KindUsage, "invalid admin group list continuation cursor")
	}
	if requested != 0 && requested != v.Size {
		return 0, 0, "", errs.New(errs.KindUsage, "admin group list limit must match the continuation cursor")
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
	items, err := paging.Collect(ctx, func(ctx context.Context, state paging.State) (paging.Page[Group], error) {
		page, err := a.reader.ListGroups(ctx, PageRequest{PageNumber: state.Number, PageSize: state.Size, SnapshotCursor: state.Token, Name: input.Name, Domain: input.Domain})
		requestID = page.RequestID
		return paging.Page[Group]{Number: page.Number, Size: page.Size, Total: page.Total, Items: page.Groups, Token: page.SnapshotCursor}, err
	}, func(item Group) string { return item.LUID })
	if err != nil {
		return Output{}, err
	}
	return Output{Status: "listed", Environment: input.Environment, Site: input.Site, Groups: items, Page: OutputPage{Returned: len(items), Total: len(items), Limit: 10000}, RequestID: requestID, Help: []string{"tadx admin group inspect --id <group-luid>"}}, nil
}
