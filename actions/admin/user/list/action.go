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
	Environment, Site, Cursor, Name, SiteRole string
	Limit                                     int
}
type PageRequest struct {
	PageNumber, PageSize int
	Name, SiteRole       string
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
	Number, Size, Total int
	Users               []User
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
	Users                     []User
	RequestID                 string
	Help                      []string
}
type CompactUser struct {
	LUID     string `json:"luid"`
	Name     string `json:"name"`
	SiteRole string `json:"site_role,omitempty"`
}
type CompactResult struct {
	Status      string        `json:"status"`
	Environment string        `json:"environment,omitempty"`
	Site        string        `json:"site,omitempty"`
	Page        OutputPage    `json:"page"`
	Users       []CompactUser `json:"users"`
	Details     string        `json:"details"`
	Help        []string      `json:"help"`
}
type FullResult struct {
	Status      string     `json:"status"`
	Environment string     `json:"environment,omitempty"`
	Site        string     `json:"site,omitempty"`
	Page        OutputPage `json:"page"`
	Users       []User     `json:"users"`
	RequestID   string     `json:"tableau_request_id,omitempty"`
	Help        []string   `json:"help"`
}

func (o Output) CompactOutput() any {
	items := make([]CompactUser, len(o.Users))
	for i, v := range o.Users {
		items[i] = CompactUser{LUID: v.LUID, Name: v.Name, SiteRole: v.SiteRole}
	}
	return CompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Page: o.Page, Users: items, Details: "--full", Help: o.Help}
}
func (o Output) FullOutput() any {
	return FullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Page: o.Page, Users: append([]User(nil), o.Users...), RequestID: o.RequestID, Help: o.Help}
}

type Reader interface {
	ListUsers(context.Context, PageRequest) (Page, error)
}
type Action struct{ reader Reader }

func New(reader Reader) *Action { return &Action{reader: reader} }
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.reader == nil {
		return Output{}, errors.New("admin user list reader is not configured")
	}
	fingerprint, err := cursorFingerprint(struct{ Environment, Site, Name, SiteRole string }{input.Environment, input.Site, input.Name, input.SiteRole})
	if err != nil {
		return Output{}, err
	}
	number, size, err := selectPage(input.Cursor, input.Limit, fingerprint)
	if err != nil {
		return Output{}, err
	}
	page, err := a.reader.ListUsers(ctx, PageRequest{PageNumber: number, PageSize: size, Name: input.Name, SiteRole: input.SiteRole})
	if err != nil {
		return Output{}, err
	}
	if page.Number != number || page.Size <= 0 || len(page.Users) > page.Size {
		return Output{}, errors.New("admin user list reader returned inconsistent pagination")
	}
	next := ""
	if page.Number*page.Size < page.Total {
		next, err = encodeCursor(page.Number+1, page.Size, fingerprint)
		if err != nil {
			return Output{}, err
		}
	}
	return Output{Status: "listed", Environment: input.Environment, Site: input.Site, Page: OutputPage{Returned: len(page.Users), Total: page.Total, Limit: page.Size, NextCursor: next}, Users: page.Users, RequestID: page.RequestID, Help: []string{"tadx admin user get --id <user-luid>"}}, nil
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
			return 0, 0, fmt.Errorf("admin user list limit must be between 1 and 100")
		}
		return 1, requested, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	var v cursorValue
	if len(encoded) > 256 || err != nil || json.Unmarshal(data, &v) != nil || v.Version != 1 || v.Page < 2 || v.Size < 1 || v.Size > 100 || v.Filter != filter {
		return 0, 0, errors.New("invalid admin user list continuation cursor")
	}
	if requested != 0 && requested != v.Size {
		return 0, 0, errors.New("admin user list limit must match the continuation cursor")
	}
	return v.Page, v.Size, nil
}
func encodeCursor(page, size int, filter string) (string, error) {
	data, err := json.Marshal(cursorValue{1, page, size, filter})
	return base64.RawURLEncoding.EncodeToString(data), err
}
