package admin

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/ahillspace/tadx/internal/tableau"
)

type PermissionMutationRequest struct {
	PermissionRequest
	Rule PermissionRule
}

// PermissionMutationClient keeps permission writes separate from inventory clients.
type PermissionMutationClient interface {
	CreatePermission(context.Context, PermissionMutationRequest) (MutationResult, error)
	DeletePermission(context.Context, PermissionMutationRequest) (MutationResult, error)
}

func ValidatePermissionMutation(in PermissionMutationRequest) error {
	if _, _, err := permissionPath(in.PermissionRequest); err != nil {
		return err
	}
	if in.Rule.PrincipalType != "user" && in.Rule.PrincipalType != "group" {
		return errors.New("permission principal type must be user or group")
	}
	if strings.TrimSpace(in.Rule.PrincipalLUID) == "" {
		return errors.New("permission principal LUID is required")
	}
	if in.Rule.Mode != "Allow" && in.Rule.Mode != "Deny" {
		return errors.New("permission mode must be Allow or Deny")
	}
	kind := in.ResourceKind
	if in.DefaultFor != "" {
		kind = strings.TrimSuffix(in.DefaultFor, "s")
	}
	if !slices.Contains(PermissionCapabilities(kind), in.Rule.Capability) {
		return &permissionCapabilityError{kind: kind, capability: in.Rule.Capability}
	}
	return nil
}

// PermissionCapabilities returns the supported capability names for one resource kind.
func PermissionCapabilities(kind string) []string {
	return map[string][]string{
		"workbook":   {"AddComment", "ChangeHierarchy", "ChangePermissions", "CreateRefreshMetrics", "Delete", "ExportData", "ExportImage", "ExportXml", "ExtractRefresh", "Filter", "Read", "RunExplainData", "ShareView", "ViewComments", "ViewUnderlyingData", "WebAuthoring", "Write"},
		"datasource": {"PulseMetricDefine", "ChangePermissions", "Connect", "Delete", "ExportXml", "ExtractRefresh", "Read", "Write", "SaveAs"},
		"flow":       {"ChangeHierarchy", "ChangePermissions", "Delete", "Execute", "ExportXml", "Read", "WebAuthoringForFlows", "Write"},
		"project":    {"ProjectLeader", "Read", "Write"},
	}[kind]
}

type permissionCapabilityError struct{ kind, capability string }

func (e *permissionCapabilityError) Error() string {
	return fmt.Sprintf("unsupported %s permission capability %q", e.kind, e.capability)
}

func (e *permissionCapabilityError) Retryable() bool { return false }

func (e *permissionCapabilityError) CorrectiveAction() string {
	return "Supported " + e.kind + " capabilities: " + strings.Join(PermissionCapabilities(e.kind), ", ") + ". Select an exact --capability and review a new --preview."
}

func (c *Client) CreatePermission(ctx context.Context, in PermissionMutationRequest) (MutationResult, error) {
	const operation = "admin.permission.create"
	if err := ValidatePermissionMutation(in); err != nil {
		return MutationResult{}, err
	}
	parts, _, _ := permissionPath(in.PermissionRequest)
	payload := permissionWriteXML{}
	if in.ResourceKind == "flow" && in.DefaultFor == "" {
		payload.Flow = &idXML{ID: in.ResourceLUID}
	}
	grantee := permissionGranteeWriteXML{}
	if in.Rule.PrincipalType == "user" {
		grantee.User = &idXML{ID: in.Rule.PrincipalLUID}
	} else {
		grantee.Group = &idXML{ID: in.Rule.PrincipalLUID}
	}
	grantee.Capabilities.Items = []capabilityXML{{Name: in.Rule.Capability, Mode: in.Rule.Mode}}
	payload.Grantees = []permissionGranteeWriteXML{grantee}
	response, err := c.write(ctx, http.MethodPut, operation, parts, payload)
	if err != nil {
		return permissionWriteFailure(in, err)
	}
	unknown := MutationResult{Status: "unknown", ResourceLUID: in.ResourceLUID, RequestID: response.TableauRequestID}
	if err := exactStatus(operation, response, http.StatusOK); err != nil {
		return unknown, err
	}
	rules, err := decodePermissionRules(operation, response, in.PermissionRequest, false)
	if err != nil {
		return unknown, err
	}
	found := false
	for _, rule := range rules {
		if rule.PrincipalType == in.Rule.PrincipalType && rule.PrincipalLUID == in.Rule.PrincipalLUID && rule.Capability == in.Rule.Capability {
			if found || rule.Mode != in.Rule.Mode {
				return unknown, mutationProtocol(operation, response, errors.New("permission mutation returned a conflicting capability"))
			}
			found = true
		}
	}
	if !found {
		return unknown, mutationProtocol(operation, response, errors.New("permission mutation response omitted the requested rule"))
	}
	return MutationResult{Status: "created", ResourceLUID: in.ResourceLUID, RequestID: response.TableauRequestID}, nil
}

func (c *Client) DeletePermission(ctx context.Context, in PermissionMutationRequest) (MutationResult, error) {
	if err := ValidatePermissionMutation(in); err != nil {
		return MutationResult{}, err
	}
	parts, _, _ := permissionPath(in.PermissionRequest)
	parts = append(parts, in.Rule.PrincipalType+"s", in.Rule.PrincipalLUID, in.Rule.Capability, in.Rule.Mode)
	result, err := c.delete(ctx, "admin.permission.delete", parts, in.ResourceLUID)
	if err != nil && result.Status == "" {
		return permissionWriteFailure(in, err)
	}
	return result, err
}

func permissionWriteFailure(in PermissionMutationRequest, err error) (MutationResult, error) {
	var upstream *tableau.UpstreamError
	if errors.As(err, &upstream) {
		return MutationResult{}, err
	}
	return MutationResult{Status: "unknown", ResourceLUID: in.ResourceLUID, RequestID: tableau.RequestID(err)}, err
}

type permissionWriteXML struct {
	XMLName  xml.Name                    `xml:"permissions"`
	Flow     *idXML                      `xml:"flow,omitempty"`
	Grantees []permissionGranteeWriteXML `xml:"granteeCapabilities"`
}
type permissionGranteeWriteXML struct {
	User         *idXML `xml:"user,omitempty"`
	Group        *idXML `xml:"group,omitempty"`
	Capabilities struct {
		Items []capabilityXML `xml:"capability"`
	} `xml:"capabilities"`
}

// decodePermissionRules validates identity when present; default permission responses
// do not consistently contain a resource identity in the documented contract.
func decodePermissionRules(operation string, response tableau.Response, in PermissionRequest, read bool) ([]PermissionRule, error) {
	fail := func(err error) ([]PermissionRule, error) {
		return nil, tableau.NewProtocolError(operation, response, err, read)
	}
	var envelope struct {
		XMLName     xml.Name `xml:"tsResponse"`
		Permissions []struct {
			Workbook   idXML        `xml:"workbook"`
			Datasource idXML        `xml:"datasource"`
			Flow       idXML        `xml:"flow"`
			Project    idXML        `xml:"project"`
			Grantees   []granteeXML `xml:"granteeCapabilities"`
		} `xml:"permissions"`
	}
	if err := xml.Unmarshal(response.Body, &envelope); err != nil {
		return fail(err)
	}
	if len(envelope.Permissions) != 1 {
		return fail(errors.New("permission response must contain exactly one permissions element"))
	}
	p := envelope.Permissions[0]
	if in.DefaultFor == "" {
		identities := map[string]string{"workbook": p.Workbook.ID, "datasource": p.Datasource.ID, "flow": p.Flow.ID, "project": p.Project.ID}
		for kind, luid := range identities {
			if luid != "" && (kind != in.ResourceKind || luid != in.ResourceLUID) {
				return fail(errors.New("permission response contains a different resource identity"))
			}
		}
	}
	rules := make([]PermissionRule, 0)
	seen := map[string]bool{}
	for _, g := range p.Grantees {
		principal, luid := "user", g.User.ID
		if g.Group.ID != "" {
			if luid != "" {
				return fail(errors.New("permission response contains two principals in one grantee"))
			}
			principal, luid = "group", g.Group.ID
		}
		if luid == "" {
			return fail(errors.New("permission response omitted principal identity"))
		}
		for _, c := range g.Capabilities.Items {
			if c.Name == "" || (c.Mode != "Allow" && c.Mode != "Deny") {
				return fail(errors.New("permission response contains an invalid capability or mode"))
			}
			key := principal + "\x00" + luid + "\x00" + c.Name
			if seen[key] {
				return fail(errors.New("permission response contains duplicate capability identities"))
			}
			seen[key] = true
			rules = append(rules, PermissionRule{PrincipalType: principal, PrincipalLUID: luid, Capability: c.Name, Mode: c.Mode})
		}
	}
	return rules, nil
}
