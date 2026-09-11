package metadataassets

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/tableau"
	"github.com/ahillspace/tadx/internal/value"
)

const maxResponseBytes = 8 << 20
const maxItems = 10000
const maxPageSize = 100

type Client struct {
	transport *tableau.Transport
	session   auth.Session
	serverURL string
}

func NewClient(t *tableau.Transport, s auth.Session, server string) *Client {
	return &Client{t, s, server}
}

func (c *Client) configured() error {
	if c == nil || c.transport == nil || c.session == nil || strings.TrimSpace(c.serverURL) == "" || strings.TrimSpace(c.session.SiteLUID()) == "" {
		return errors.New("metadata client is not configured")
	}
	return nil
}
func (c *Client) path(parts ...string) string {
	all := append([]string{"api", c.transport.APIVersion(), "sites", c.session.SiteLUID()}, parts...)
	for i := range all {
		all[i] = url.PathEscape(all[i])
	}
	return "/" + strings.Join(all, "/")
}
func exact(s string) error {
	if strings.TrimSpace(s) == "" || s != strings.TrimSpace(s) || strings.ContainsAny(s, "\x00\r\n") {
		return errors.New("an exact nonempty identity is required")
	}
	return nil
}
func (c *Client) rest(ctx context.Context, method string, parts []string, query url.Values, body any, op string) (tableau.Response, error) {
	if err := c.configured(); err != nil {
		return tableau.Response{}, err
	}
	minimum := 5
	if len(parts) > 0 {
		switch parts[0] {
		case "labels":
			minimum = 17
		case "labelValues", "labelCategories":
			minimum = 21
		}
	}
	version := strings.Split(c.transport.APIVersion(), ".")
	if len(version) != 2 {
		return tableau.Response{}, &ConstraintError{"cannot establish metadata REST version support"}
	}
	major, majorErr := strconv.Atoi(version[0])
	minor, minorErr := strconv.Atoi(version[1])
	if majorErr != nil || minorErr != nil || major < 3 || (major == 3 && minor < minimum) {
		return tableau.Response{}, &ConstraintError{fmt.Sprintf("this metadata operation requires REST API 3.%d or later under the shared Cloud/Server contract", minimum)}
	}
	var data []byte
	var err error
	if body != nil {
		data, err = xml.Marshal(body)
		if err != nil {
			return tableau.Response{}, err
		}
	}
	r, err := c.transport.Do(ctx, c.session, tableau.Request{Method: method, ServerURL: c.serverURL, Path: c.path(parts...), Query: query, Body: data, Accept: "application/xml", ContentType: "application/xml", Operation: op, MaxResponseBytes: maxResponseBytes})
	if err != nil {
		return r, err
	}
	expected := http.StatusOK
	if method == http.MethodDelete && len(parts) > 0 && (parts[0] == "labels" || (len(parts) > 2 && parts[2] == "tags")) {
		expected = http.StatusNoContent
	}
	if r.StatusCode != expected {
		return r, protocol(op, r, fmt.Errorf("metadata operation returned HTTP %d, expected %d", r.StatusCode, expected))
	}
	return r, nil
}

type nodeXML struct {
	XMLName  xml.Name
	Attrs    []xml.Attr `xml:",any,attr"`
	Children []nodeXML  `xml:",any"`
}

func (n nodeXML) attr(name string) string {
	for _, a := range n.Attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}
func (n nodeXML) ptr(name string) *string {
	for _, a := range n.Attrs {
		if a.Name.Local == name {
			v := a.Value
			return &v
		}
	}
	return nil
}
func (n nodeXML) child(name string) (nodeXML, bool) {
	for _, v := range n.Children {
		if v.XMLName.Local == name {
			return v, true
		}
	}
	return nodeXML{}, false
}
func (n nodeXML) rows(name string) []nodeXML {
	var out []nodeXML
	for _, v := range n.Children {
		if v.XMLName.Local == name {
			out = append(out, v)
		}
	}
	return out
}
func protocol(op string, r tableau.Response, err error) error {
	return tableau.NewProtocolError(op, r, err, false)
}
func decode(r tableau.Response, op string) (nodeXML, error) {
	var n nodeXML
	if err := xml.Unmarshal(r.Body, &n); err != nil {
		return n, protocol(op, r, fmt.Errorf("decode metadata response: %w", err))
	}
	if n.XMLName.Local != "tsResponse" {
		return n, protocol(op, r, errors.New("metadata response has unexpected root"))
	}
	if err := validateXMLNodes(n, 0, new(int)); err != nil {
		return n, protocol(op, r, err)
	}
	return n, nil
}
func validateXMLNodes(n nodeXML, depth int, count *int) error {
	*count++
	if depth > 32 || *count > maxItems*10 {
		return errors.New("metadata XML exceeds structural bound")
	}
	seen := map[xml.Name]bool{}
	for _, a := range n.Attrs {
		if seen[a.Name] {
			return errors.New("duplicate metadata XML attribute")
		}
		seen[a.Name] = true
	}
	for _, child := range n.Children {
		if err := validateXMLNodes(child, depth+1, count); err != nil {
			return err
		}
	}
	return nil
}
func one(r tableau.Response, op, kind string) (nodeXML, error) {
	n, err := decode(r, op)
	if err != nil {
		return nodeXML{}, err
	}
	rows := n.rows(kind)
	if len(rows) != 1 {
		return nodeXML{}, protocol(op, r, fmt.Errorf("expected one %s response", kind))
	}
	return rows[0], nil
}
func many(r tableau.Response, op, container, item string) ([]nodeXML, error) {
	n, err := one(r, op, container)
	if err != nil {
		return nil, err
	}
	rows := n.rows(item)
	if len(rows) > maxItems {
		return nil, protocol(op, r, errors.New("metadata collection exceeds item bound"))
	}
	return rows, nil
}
func (c *Client) validateNode(n nodeXML, kind, id string) error {
	if n.attr("id") == "" || n.attr("name") == "" || (id != "" && n.attr("id") != id) {
		return fmt.Errorf("%s response has incomplete or mismatched authoritative identity", kind)
	}
	if sites := n.rows("site"); len(sites) > 1 {
		return errors.New("multiple response site identities")
	} else if len(sites) == 1 && sites[0].attr("id") != c.session.SiteLUID() {
		return errors.New("response site identity mismatch")
	}
	return nil
}
func tags(n nodeXML) ([]string, bool, error) {
	sets := n.rows("tags")
	if len(sets) == 0 {
		return nil, false, nil
	}
	if len(sets) != 1 {
		return nil, false, errors.New("duplicate tag collections")
	}
	out := make([]string, 0)
	seen := map[string]bool{}
	for _, v := range sets[0].rows("tag") {
		s := v.attr("label")
		if strings.TrimSpace(s) == "" {
			return nil, false, errors.New("empty tag identity")
		}
		if !seen[s] {
			out = append(out, s)
			seen[s] = true
		}
	}
	sort.Strings(out)
	return out, true, nil
}
func database(n nodeXML) (value.MetadataDatabase, error) {
	ts, seen, err := tags(n)
	contact, _ := n.child("contact")
	return value.MetadataDatabase{MetadataIdentity: value.MetadataIdentity{LUID: n.attr("id"), Name: n.attr("name"), Type: n.attr("type")}, Description: n.ptr("description"), ContactLUID: contact.attr("id"), ConnectionType: n.attr("connectionType"), FilePath: n.attr("filePath"), Embedded: n.attr("isEmbedded") == "true", Tags: ts, TagsObserved: seen}, err
}
func table(n nodeXML) (value.MetadataTable, error) {
	ts, seen, err := tags(n)
	contact, _ := n.child("contact")
	db, _ := n.child("database")
	return value.MetadataTable{MetadataIdentity: value.MetadataIdentity{LUID: n.attr("id"), Name: n.attr("name"), Type: "table"}, Description: n.ptr("description"), ContactLUID: contact.attr("id"), Database: value.MetadataIdentity{LUID: db.attr("id"), Name: db.attr("name"), Type: "database"}, FullName: n.attr("fullName"), Schema: n.attr("schema"), Tags: ts, TagsObserved: seen}, err
}
func column(n nodeXML) (value.MetadataColumn, error) {
	ts, seen, err := tags(n)
	return value.MetadataColumn{MetadataIdentity: value.MetadataIdentity{LUID: n.attr("id"), Name: n.attr("name"), Type: "column"}, Description: n.ptr("description"), Table: value.MetadataIdentity{LUID: n.attr("parentTableId"), Type: "table"}, RemoteType: n.attr("remoteType"), Tags: ts, TagsObserved: seen}, err
}
func (c *Client) get(ctx context.Context, kind, id string, parts []string) (nodeXML, error) {
	if err := exact(id); err != nil {
		return nodeXML{}, err
	}
	op := "catalog." + kind + ".inspect"
	r, err := c.rest(ctx, http.MethodGet, parts, nil, nil, op)
	if err != nil {
		return nodeXML{}, err
	}
	n, err := one(r, op, kind)
	if err == nil {
		err = c.validateNode(n, kind, id)
		if err != nil {
			err = protocol(op, r, err)
		}
	}
	return n, err
}
func (c *Client) GetDatabase(ctx context.Context, id string) (value.MetadataDatabase, error) {
	n, e := c.get(ctx, "database", id, []string{"databases", id})
	if e != nil {
		return value.MetadataDatabase{}, e
	}
	return database(n)
}
func (c *Client) GetTable(ctx context.Context, id string) (value.MetadataTable, error) {
	n, e := c.get(ctx, "table", id, []string{"tables", id})
	if e != nil {
		return value.MetadataTable{}, e
	}
	return table(n)
}
func (c *Client) GetColumn(ctx context.Context, parent, id string) (value.MetadataColumn, error) {
	if err := exact(parent); err != nil {
		return value.MetadataColumn{}, err
	}
	n, e := c.get(ctx, "column", id, []string{"tables", parent, "columns", id})
	if e != nil {
		return value.MetadataColumn{}, e
	}
	v, e := column(n)
	if e == nil && v.Table.LUID != parent {
		return value.MetadataColumn{}, errors.New("column response parent identity mismatch")
	}
	return v, e
}

type patchXML struct {
	XMLName     xml.Name
	Description *string     `xml:"description,attr,omitempty"`
	Contact     *contactXML `xml:"contact,omitempty"`
}
type contactXML struct {
	ID string `xml:"id,attr"`
}
type patchEnvelope struct {
	XMLName xml.Name `xml:"tsRequest"`
	Patch   patchXML
}

func validateUpdate(u Update, kind string) error {
	if u.Description == nil && u.ContactLUID == nil {
		return errors.New("metadata update requires a property")
	}
	if u.Description != nil {
		if *u.Description == "" {
			return &ConstraintError{"clearing descriptions is not yet verified"}
		}
		if len(*u.Description) > 65536 {
			return errors.New("description exceeds 65536 bytes")
		}
	}
	if u.ContactLUID != nil {
		if kind == "column" {
			return &ConstraintError{"columns do not support contacts"}
		}
		if *u.ContactLUID == "" {
			return &ConstraintError{"clearing contacts is not yet verified"}
		}
		if err := exact(*u.ContactLUID); err != nil {
			return err
		}
	}
	return nil
}
func (c *Client) update(ctx context.Context, kind, id string, parts []string, u Update) (nodeXML, error) {
	if err := exact(id); err != nil {
		return nodeXML{}, err
	}
	if err := validateUpdate(u, kind); err != nil {
		return nodeXML{}, err
	}
	p := patchXML{XMLName: xml.Name{Local: kind}, Description: u.Description}
	if u.ContactLUID != nil {
		p.Contact = &contactXML{*u.ContactLUID}
	}
	op := "catalog." + kind + ".update"
	r, err := c.rest(ctx, http.MethodPut, parts, nil, patchEnvelope{Patch: p}, op)
	if err != nil {
		return nodeXML{}, err
	}
	n, err := one(r, op, kind)
	if err != nil {
		return nodeXML{Attrs: []xml.Attr{{Name: xml.Name{Local: "id"}, Value: id}}}, err
	}
	if err = c.validateNode(n, kind, id); err != nil {
		return n, protocol(op, r, err)
	}
	if u.Description != nil && (n.ptr("description") == nil || n.attr("description") != *u.Description) {
		return n, protocol(op, r, errors.New("metadata update succeeded but description read-back differs"))
	}
	if u.ContactLUID != nil {
		v, ok := n.child("contact")
		if !ok || v.attr("id") != *u.ContactLUID {
			return n, protocol(op, r, errors.New("metadata update succeeded but contact read-back differs"))
		}
	}
	return n, nil
}
func (c *Client) UpdateDatabase(ctx context.Context, id string, u Update) (value.MetadataDatabase, error) {
	n, e := c.update(ctx, "database", id, []string{"databases", id}, u)
	v, parse := database(n)
	if e != nil {
		return v, e
	}
	return v, parse
}
func (c *Client) UpdateTable(ctx context.Context, id string, u Update) (value.MetadataTable, error) {
	n, e := c.update(ctx, "table", id, []string{"tables", id}, u)
	v, parse := table(n)
	if e != nil {
		return v, e
	}
	return v, parse
}
func (c *Client) UpdateColumn(ctx context.Context, parent, id string, u Update) (value.MetadataColumn, error) {
	if err := exact(parent); err != nil {
		return value.MetadataColumn{}, err
	}
	n, e := c.update(ctx, "column", id, []string{"tables", parent, "columns", id}, u)
	v, parse := column(n)
	if e != nil {
		return v, e
	}
	if v.Table.LUID != parent {
		return v, errors.New("updated column parent identity mismatch")
	}
	return v, parse
}

func target(t LabelTarget, labels bool) error {
	if err := exact(t.LUID); err != nil {
		return err
	}
	switch t.Type {
	case "database", "table", "column":
		return nil
	case "datasource", "flow":
		if labels {
			return nil
		}
	}
	return &ConstraintError{"unsupported metadata target type: " + t.Type}
}

type tagXML struct {
	Label string `xml:"label,attr"`
}
type tagsEnvelope struct {
	XMLName xml.Name `xml:"tsRequest"`
	Tags    []tagXML `xml:"tags>tag"`
}

func (c *Client) AddTags(ctx context.Context, t LabelTarget, values []string) ([]string, error) {
	if err := target(t, false); err != nil {
		return nil, err
	}
	if len(values) == 0 || len(values) > 100 {
		return nil, errors.New("provide 1 to 100 tags")
	}
	body := tagsEnvelope{}
	for _, s := range values {
		if strings.TrimSpace(s) == "" || utf8.RuneCountInString(s) > 128 {
			return nil, errors.New("tag must contain 1 to 128 characters")
		}
		body.Tags = append(body.Tags, tagXML{s})
	}
	op := "catalog.tags.add"
	r, e := c.rest(ctx, http.MethodPut, []string{t.Type + "s", t.LUID, "tags"}, nil, body, op)
	if e != nil {
		return nil, e
	}
	n, e := decode(r, op)
	if e != nil {
		return nil, e
	}
	out, seen, e := tags(n)
	if e != nil || !seen {
		return nil, protocol(op, r, errors.New("tag response omitted verified tags"))
	}
	return out, nil
}
func (c *Client) DeleteTag(ctx context.Context, t LabelTarget, tag string) error {
	if e := target(t, false); e != nil {
		return e
	}
	if e := exact(tag); e != nil {
		return e
	}
	_, e := c.rest(ctx, http.MethodDelete, []string{t.Type + "s", t.LUID, "tags", tag}, nil, nil, "catalog.tags.delete")
	return e
}

func unique[T any](rows []T, key func(T) string) ([]T, error) {
	out := make([]T, 0, len(rows))
	seen := map[string]T{}
	for _, v := range rows {
		k := key(v)
		if k == "" {
			return nil, errors.New("missing record identity")
		}
		if old, ok := seen[k]; ok {
			if !reflect.DeepEqual(old, v) {
				return nil, errors.New("conflicting duplicate identity")
			}
			continue
		}
		seen[k] = v
		out = append(out, v)
	}
	return out, nil
}
