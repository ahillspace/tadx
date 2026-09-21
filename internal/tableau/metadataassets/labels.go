package metadataassets

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ahillspace/tadx/internal/tableau"
	"github.com/ahillspace/tadx/internal/value"
)

type acknowledgedError struct{ cause error }

func (e *acknowledgedError) Error() string {
	return "Tableau acknowledged the write, but response verification failed: " + e.cause.Error()
}
func (e *acknowledgedError) Unwrap() error         { return e.cause }
func (*acknowledgedError) WriteAcknowledged() bool { return true }

// acknowledged marks only response-verification failures after the expected HTTP acknowledgement.
// It does not claim that requested values or a new attachment identity were verified.
func acknowledged(op string, r tableau.Response, cause error) error {
	var protocolError *tableau.ProtocolError
	if !errors.As(cause, &protocolError) {
		cause = protocol(op, r, cause)
	}
	return &acknowledgedError{cause: cause}
}

type labelTargetXML struct {
	Type string `xml:"contentType,attr"`
	ID   string `xml:"id,attr"`
}
type labelPatchXML struct {
	Value    string `xml:"value,attr"`
	Message  string `xml:"message,attr"`
	Active   bool   `xml:"active,attr"`
	Elevated bool   `xml:"elevated,attr"`
}
type labelEnvelope struct {
	XMLName xml.Name         `xml:"tsRequest"`
	Targets *labelTargetsXML `xml:"contentList,omitempty"`
	Label   *labelPatchXML   `xml:"label,omitempty"`
}
type labelTargetsXML struct {
	Items []labelTargetXML `xml:"content"`
}

func labelBody(t *LabelTarget, u *LabelUpdate) labelEnvelope {
	body := labelEnvelope{}
	if t != nil {
		body.Targets = &labelTargetsXML{[]labelTargetXML{{t.Type, t.LUID}}}
	}
	if u != nil {
		body.Label = &labelPatchXML{u.Value, u.Message, u.Active, u.Elevated}
	}
	return body
}
func label(n nodeXML) (value.ContentLabel, error) {
	active, e := strconv.ParseBool(n.attr("active"))
	if e != nil {
		return value.ContentLabel{}, errors.New("label response omitted valid active state")
	}
	elevated, e := strconv.ParseBool(n.attr("elevated"))
	if e != nil {
		return value.ContentLabel{}, errors.New("label response omitted valid elevated state")
	}
	v := value.ContentLabel{LUID: n.attr("id"), TargetLUID: n.attr("contentId"), Type: value.CanonicalContentType(n.attr("contentType")), Value: n.attr("value"), Category: n.attr("category"), Message: n.attr("message"), Active: active, Elevated: elevated, CreatedAt: n.attr("createdAt"), UpdatedAt: n.attr("updatedAt")}
	owner, _ := n.child("owner")
	v.OwnerLUID = owner.attr("id")
	if v.LUID == "" || v.TargetLUID == "" || v.Type == "" || v.Value == "" || v.Category == "" {
		return v, errors.New("label response has incomplete identity")
	}
	return v, nil
}

func (c *Client) labelRows(r tableau.Response, op string, t *LabelTarget) ([]value.ContentLabel, error) {
	nodes, e := many(r, op, "labelList", "label")
	if e != nil {
		return nil, e
	}
	out := make([]value.ContentLabel, 0, len(nodes))
	for _, n := range nodes {
		v, e := label(n)
		if e != nil {
			return nil, protocol(op, r, e)
		}
		if t != nil && (v.TargetLUID != t.LUID || v.Type != t.Type) {
			return nil, protocol(op, r, fmt.Errorf("label target mismatch: requested type=%q target=%q, returned type=%q target=%q", t.Type, t.LUID, v.Type, v.TargetLUID))
		}
		out = append(out, v)
	}
	out, e = unique(out, func(v value.ContentLabel) string { return v.LUID })
	if e != nil {
		return nil, protocol(op, r, e)
	}
	return out, nil
}
func (c *Client) GetLabels(ctx context.Context, t LabelTarget, categories []string) ([]value.ContentLabel, error) {
	if e := target(t, true); e != nil {
		return nil, e
	}
	q := url.Values{}
	if len(categories) > 100 {
		return nil, errors.New("too many label categories")
	}
	for _, s := range categories {
		if strings.TrimSpace(s) == "" || strings.Contains(s, ",") {
			return nil, errors.New("invalid label category")
		}
	}
	if len(categories) > 0 {
		q.Set("categories", strings.Join(categories, ","))
	}
	op := "content.label.list"
	r, e := c.rest(ctx, http.MethodPost, []string{"labels"}, q, labelBody(&t, nil), op)
	if e != nil {
		return nil, e
	}
	return c.labelRows(r, op, &t)
}
func (c *Client) GetLabel(ctx context.Context, id string) (value.ContentLabel, error) {
	if e := exact(id); e != nil {
		return value.ContentLabel{}, e
	}
	op := "content.label.inspect"
	r, e := c.rest(ctx, http.MethodGet, []string{"labels", id}, nil, nil, op)
	if e != nil {
		return value.ContentLabel{}, e
	}
	n, e := one(r, op, "label")
	if e != nil {
		return value.ContentLabel{}, e
	}
	v, e := label(n)
	if e == nil && v.LUID != id {
		e = errors.New("label identity mismatch")
	}
	if e != nil {
		return v, protocol(op, r, e)
	}
	return v, nil
}
func validateLabel(u LabelUpdate) error {
	if e := exact(u.Value); e != nil {
		return e
	}
	if len(u.Message) > 65536 {
		return errors.New("label message exceeds 65536 bytes")
	}
	if u.Value == "extract_refresh_failure" || u.Value == "flow_run_failure" {
		return &ConstraintError{"monitoring labels require separate trigger operations"}
	}
	return nil
}
func labelMatches(v value.ContentLabel, u LabelUpdate) bool {
	return v.Value == u.Value && v.Message == u.Message && v.Active == u.Active && v.Elevated == u.Elevated
}
func (c *Client) SetLabel(ctx context.Context, t LabelTarget, u LabelUpdate) (value.ContentLabel, error) {
	if e := target(t, true); e != nil {
		return value.ContentLabel{}, e
	}
	if e := validateLabel(u); e != nil {
		return value.ContentLabel{}, e
	}
	op := "content.label.update"
	r, e := c.rest(ctx, http.MethodPut, []string{"labels"}, nil, labelBody(&t, &u), op)
	if e != nil {
		return value.ContentLabel{}, e
	}
	rows, e := c.labelRows(r, op, &t)
	if e != nil {
		return value.ContentLabel{TargetLUID: t.LUID, Type: t.Type}, acknowledged(op, r, e)
	}
	if len(rows) != 1 {
		return value.ContentLabel{TargetLUID: t.LUID, Type: t.Type}, acknowledged(op, r, errors.New("label write did not return one attachment"))
	}
	v := rows[0]
	if !labelMatches(v, u) {
		return v, acknowledged(op, r, errors.New("label saved configuration differs from request"))
	}
	return v, nil
}
func (c *Client) UpdateLabel(ctx context.Context, id string, u LabelUpdate) (value.ContentLabel, error) {
	if e := exact(id); e != nil {
		return value.ContentLabel{}, e
	}
	if e := validateLabel(u); e != nil {
		return value.ContentLabel{}, e
	}
	op := "content.label.update"
	r, e := c.rest(ctx, http.MethodPut, []string{"labels", id}, nil, labelBody(nil, &u), op)
	if e != nil {
		return value.ContentLabel{}, e
	}
	n, e := one(r, op, "label")
	if e != nil {
		return value.ContentLabel{LUID: id}, acknowledged(op, r, e)
	}
	v, e := label(n)
	if e == nil && (v.LUID != id || !labelMatches(v, u)) {
		e = errors.New("label identity or saved configuration mismatch")
	}
	if e != nil {
		if v.LUID != id {
			v = value.ContentLabel{LUID: id}
		}
		return v, acknowledged(op, r, e)
	}
	return v, nil
}
func (c *Client) DeleteLabel(ctx context.Context, id string) error {
	if e := exact(id); e != nil {
		return e
	}
	_, e := c.rest(ctx, http.MethodDelete, []string{"labels", id}, nil, nil, "content.label.delete")
	return e
}

type vocabXML struct {
	XMLName     xml.Name
	Name        string `xml:"name,attr"`
	Category    string `xml:"category,attr,omitempty"`
	Description string `xml:"description,attr"`
}
type vocabEnvelope struct {
	XMLName xml.Name `xml:"tsRequest"`
	Value   vocabXML
}

func decodeValue(n nodeXML) (value.LabelValue, error) {
	v := value.LabelValue{Name: n.attr("name"), Category: n.attr("category"), Description: n.attr("description")}
	if v.Name == "" || v.Category == "" {
		return v, errors.New("label value identity incomplete")
	}
	var e error
	for name, dest := range map[string]*bool{"internal": &v.Internal, "elevatedDefault": &v.ElevatedDefault, "builtIn": &v.BuiltIn} {
		*dest, e = strconv.ParseBool(n.attr(name))
		if e != nil {
			return v, errors.New("label value omitted valid " + name + " state")
		}
	}
	return v, nil
}
func decodeCategory(n nodeXML) (value.LabelCategory, error) {
	v := value.LabelCategory{Name: n.attr("name"), Description: n.attr("description")}
	if v.Name == "" {
		return v, errors.New("label category identity incomplete")
	}
	return v, nil
}
func validateVocabulary(name, description string) error {
	if e := exact(name); e != nil {
		return e
	}
	if utf8.RuneCountInString(name) > 128 {
		return errors.New("name exceeds 128 characters")
	}
	if n := utf8.RuneCountInString(description); n == 0 || n > 500 {
		return errors.New("description must contain 1 to 500 characters")
	}
	return nil
}
func (c *Client) ListLabelValues(ctx context.Context) ([]value.LabelValue, error) {
	if err := c.authorize("admin.label.value.list"); err != nil {
		return nil, err
	}
	op := "admin.label.value.list"
	r, e := c.rest(ctx, http.MethodGet, []string{"labelValues"}, nil, nil, op)
	if e != nil {
		return nil, e
	}
	nodes, e := many(r, op, "labelValueList", "labelValue")
	if e != nil {
		return nil, e
	}
	out := make([]value.LabelValue, 0, len(nodes))
	for _, n := range nodes {
		v, e := decodeValue(n)
		if e != nil {
			return nil, protocol(op, r, e)
		}
		out = append(out, v)
	}
	out, e = unique(out, func(v value.LabelValue) string { return v.Name })
	if e != nil {
		return nil, protocol(op, r, e)
	}
	return out, nil
}
func (c *Client) GetLabelValue(ctx context.Context, name string) (value.LabelValue, error) {
	if err := c.authorize("admin.label.value.inspect"); err != nil {
		return value.LabelValue{}, err
	}
	if e := exact(name); e != nil {
		return value.LabelValue{}, e
	}
	op := "admin.label.value.inspect"
	r, e := c.rest(ctx, http.MethodGet, []string{"labelValues", name}, nil, nil, op)
	if e != nil {
		return value.LabelValue{}, e
	}
	n, e := one(r, op, "labelValue")
	if e != nil {
		return value.LabelValue{}, e
	}
	v, e := decodeValue(n)
	if e == nil && v.Name != name {
		e = errors.New("label value name mismatch")
	}
	if e != nil {
		return v, protocol(op, r, e)
	}
	return v, nil
}
func (c *Client) SetLabelValue(ctx context.Context, oldName string, v value.LabelValue) (value.LabelValue, error) {
	if err := c.authorize("admin.label.value.update"); err != nil {
		return value.LabelValue{}, err
	}
	if e := validateVocabulary(v.Name, v.Description); e != nil {
		return value.LabelValue{}, e
	}
	if e := exact(v.Category); e != nil {
		return value.LabelValue{}, e
	}
	parts := []string{"labelValues"}
	if oldName != "" {
		if e := exact(oldName); e != nil {
			return value.LabelValue{}, e
		}
		parts = append(parts, oldName)
	}
	op := "admin.label.value.update"
	body := vocabEnvelope{Value: vocabXML{XMLName: xml.Name{Local: "labelValue"}, Name: v.Name, Category: v.Category, Description: v.Description}}
	r, e := c.rest(ctx, http.MethodPut, parts, nil, body, op)
	if e != nil {
		return value.LabelValue{}, e
	}
	n, e := one(r, op, "labelValue")
	if e != nil {
		return value.LabelValue{Name: v.Name}, acknowledged(op, r, e)
	}
	got, e := decodeValue(n)
	if e == nil && (got.Name != v.Name || got.Category != v.Category || got.Description != v.Description) {
		e = errors.New("label value saved configuration mismatch")
	}
	if e != nil {
		if got.Name != v.Name {
			got = value.LabelValue{Name: v.Name}
		}
		return got, acknowledged(op, r, e)
	}
	return got, nil
}
func (c *Client) DeleteLabelValue(ctx context.Context, name string) error {
	if err := c.authorize("admin.label.value.delete"); err != nil {
		return err
	}
	if e := exact(name); e != nil {
		return e
	}
	_, e := c.rest(ctx, http.MethodDelete, []string{"labelValues", name}, nil, nil, "admin.label.value.delete")
	return e
}
func (c *Client) ListLabelCategories(ctx context.Context) ([]value.LabelCategory, error) {
	if err := c.authorize("admin.label.category.list"); err != nil {
		return nil, err
	}
	return c.listLabelCategories(ctx)
}

func (c *Client) listLabelCategories(ctx context.Context) ([]value.LabelCategory, error) {
	op := "admin.label.category.list"
	r, e := c.rest(ctx, http.MethodGet, []string{"labelCategories"}, nil, nil, op)
	if e != nil {
		return nil, e
	}
	nodes, e := many(r, op, "labelCategoryList", "labelCategory")
	if e != nil {
		return nil, e
	}
	out := make([]value.LabelCategory, 0, len(nodes))
	for _, n := range nodes {
		v, e := decodeCategory(n)
		if e != nil {
			return nil, protocol(op, r, e)
		}
		out = append(out, v)
	}
	out, e = unique(out, func(v value.LabelCategory) string { return v.Name })
	if e != nil {
		return nil, protocol(op, r, e)
	}
	return out, nil
}
func (c *Client) GetLabelCategory(ctx context.Context, name string) (value.LabelCategory, error) {
	if err := c.authorize("admin.label.category.inspect"); err != nil {
		return value.LabelCategory{}, err
	}
	if e := exact(name); e != nil {
		return value.LabelCategory{}, e
	}
	rows, e := c.listLabelCategories(ctx)
	if e != nil {
		return value.LabelCategory{}, e
	}
	for _, v := range rows {
		if v.Name == name {
			return v, nil
		}
	}
	return value.LabelCategory{}, errors.New("no label category matches exact name")
}
func (c *Client) categoryWrite(ctx context.Context, old string, v value.LabelCategory) (value.LabelCategory, error) {
	if e := validateVocabulary(v.Name, v.Description); e != nil {
		return value.LabelCategory{}, e
	}
	method := http.MethodPost
	parts := []string{"labelCategories"}
	if old != "" {
		if e := exact(old); e != nil {
			return value.LabelCategory{}, e
		}
		method = http.MethodPut
		parts = append(parts, old)
	}
	op := "admin.label.category.update"
	body := vocabEnvelope{Value: vocabXML{XMLName: xml.Name{Local: "labelCategory"}, Name: v.Name, Description: v.Description}}
	r, e := c.rest(ctx, method, parts, nil, body, op)
	if e != nil {
		return value.LabelCategory{}, e
	}
	n, e := one(r, op, "labelCategory")
	if e != nil {
		return value.LabelCategory{Name: v.Name}, acknowledged(op, r, e)
	}
	got, e := decodeCategory(n)
	if e == nil && (got.Name != v.Name || got.Description != v.Description) {
		e = errors.New("label category saved configuration mismatch")
	}
	if e != nil {
		if got.Name != v.Name {
			got = value.LabelCategory{Name: v.Name}
		}
		return got, acknowledged(op, r, e)
	}
	return got, nil
}
func (c *Client) CreateLabelCategory(ctx context.Context, v value.LabelCategory) (value.LabelCategory, error) {
	if err := c.authorize("admin.label.category.create"); err != nil {
		return value.LabelCategory{}, err
	}
	return c.categoryWrite(ctx, "", v)
}
func (c *Client) UpdateLabelCategory(ctx context.Context, old string, v value.LabelCategory) (value.LabelCategory, error) {
	if err := c.authorize("admin.label.category.update"); err != nil {
		return value.LabelCategory{}, err
	}
	if e := exact(old); e != nil {
		return value.LabelCategory{}, e
	}
	return c.categoryWrite(ctx, old, v)
}
func (c *Client) DeleteLabelCategory(ctx context.Context, name string) error {
	if err := c.authorize("admin.label.category.delete"); err != nil {
		return err
	}
	if e := exact(name); e != nil {
		return e
	}
	_, e := c.rest(ctx, http.MethodDelete, []string{"labelCategories", name}, nil, nil, "admin.label.category.delete")
	return e
}
