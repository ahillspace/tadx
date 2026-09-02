package list

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

type Input struct {
	Environment, Site, Cursor, Name, Domain string
	Limit                                   int
}
type PageRequest struct {
	PageNumber, PageSize int
	Name, Domain         string
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
	Number, Size, Total int
	Groups              []Group
	RequestID           string
}
type OutputPage struct {
	Returned   int    `json:"returned"`
	Total      int    `json:"total"`
	Limit      int    `json:"limit"`
	NextCursor string `json:"next_cursor,omitempty"`
}
type Output struct {
	Status, Environment, Site string
	Page                      OutputPage
	Groups                    []Group
	RequestID                 string
	Help                      []string
}
type CompactGroup struct {
	LUID   string `json:"luid"`
	Name   string `json:"name"`
	Domain string `json:"domain,omitempty"`
}
type CompactResult struct {
	Status      string         `json:"status"`
	Environment string         `json:"environment,omitempty"`
	Site        string         `json:"site,omitempty"`
	Page        OutputPage     `json:"page"`
	Groups      []CompactGroup `json:"groups"`
	Details     string         `json:"details"`
	Help        []string       `json:"help"`
}
type FullResult struct {
	Status      string     `json:"status"`
	Environment string     `json:"environment,omitempty"`
	Site        string     `json:"site,omitempty"`
	Page        OutputPage `json:"page"`
	Groups      []Group    `json:"groups"`
	RequestID   string     `json:"tableau_request_id,omitempty"`
	Help        []string   `json:"help"`
}

func (o Output) CompactOutput() any {
	v := make([]CompactGroup, len(o.Groups))
	for i, x := range o.Groups {
		v[i] = CompactGroup{LUID: x.LUID, Name: x.Name, Domain: x.Domain}
	}
	return CompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Page: o.Page, Groups: v, Details: "--full", Help: o.Help}
}
func (o Output) FullOutput() any {
	return FullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Page: o.Page, Groups: append([]Group(nil), o.Groups...), RequestID: o.RequestID, Help: o.Help}
}

type Reader interface {
	ListGroups(context.Context, PageRequest) (Page, error)
}
type Action struct{ reader Reader }

func New(r Reader) *Action { return &Action{reader: r} }
func (a *Action) Execute(ctx context.Context, in Input) (Output, error) {
	if a == nil || a.reader == nil {
		return Output{}, errors.New("admin group list reader is not configured")
	}
	fp, err := cursorFingerprint(struct{ Environment, Site, Name, Domain string }{in.Environment, in.Site, in.Name, in.Domain})
	if err != nil {
		return Output{}, err
	}
	n, s, err := selectPage(in.Cursor, in.Limit, fp)
	if err != nil {
		return Output{}, err
	}
	p, err := a.reader.ListGroups(ctx, PageRequest{PageNumber: n, PageSize: s, Name: in.Name, Domain: in.Domain})
	if err != nil {
		return Output{}, err
	}
	if p.Number != n || p.Size <= 0 || len(p.Groups) > p.Size {
		return Output{}, errors.New("admin group list reader returned inconsistent pagination")
	}
	next := ""
	if p.Number*p.Size < p.Total {
		next, err = encodeCursor(p.Number+1, p.Size, fp)
		if err != nil {
			return Output{}, err
		}
	}
	return Output{Status: "listed", Environment: in.Environment, Site: in.Site, Page: OutputPage{Returned: len(p.Groups), Total: p.Total, Limit: p.Size, NextCursor: next}, Groups: p.Groups, RequestID: p.RequestID, Help: []string{"tadx admin group get --id <group-luid>"}}, nil
}

type cursorValue struct {
	Version, Page, Size int
	Filter              string
}

func cursorFingerprint(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return base64.RawURLEncoding.EncodeToString(sum[:]), nil
}
func selectPage(encoded string, requested int, filter string) (int, int, error) {
	if encoded == "" {
		if requested == 0 {
			requested = 25
		}
		if requested < 1 || requested > 100 {
			return 0, 0, fmt.Errorf("admin group list limit must be between 1 and 100")
		}
		return 1, requested, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	var v cursorValue
	if len(encoded) > 256 || err != nil || json.Unmarshal(data, &v) != nil || v.Version != 1 || v.Page < 2 || v.Size < 1 || v.Size > 100 || v.Filter != filter {
		return 0, 0, errors.New("invalid admin group list continuation cursor")
	}
	if requested != 0 && requested != v.Size {
		return 0, 0, errors.New("admin group list limit must match the continuation cursor")
	}
	return v.Page, v.Size, nil
}
func encodeCursor(page, size int, filter string) (string, error) {
	data, err := json.Marshal(cursorValue{1, page, size, filter})
	return base64.RawURLEncoding.EncodeToString(data), err
}
