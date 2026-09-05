package tabxml

import (
	"fmt"
	"strings"
	"testing"
)

func TestDecodeListStreamsAttributesAndDirectChildren(t *testing.T) {
	body := []byte(`<tsResponse xmlns="http://tableau.com/api"><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><users><user id="u1" name="Alice" siteRole="Explorer"><email>alice@example.com</email><owner id="ignored"/></user></users></tsResponse>`)
	var got Element
	page, count, err := DecodeList(body, "users", "user", func(element Element) error {
		got = element
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.Number != 1 || page.Size != 1000 || page.Total != 1 || count != 1 {
		t.Fatalf("page = %#v; count = %d", page, count)
	}
	if got.Attr("id") != "u1" || got.ChildText("email") != "alice@example.com" || got.ChildAttr("owner", "id") != "ignored" {
		t.Fatalf("element = %#v", got)
	}
}

func TestDecodeListPreservesGrandchildAttributes(t *testing.T) {
	body := []byte(`<tsResponse><pagination pageNumber="1" pageSize="1" totalAvailable="1"/><workbooks><workbook id="w1" name="Sales"><tags><tag label="daily"/><tag label="certified"/></tags></workbook></workbooks></tsResponse>`)
	var got Element
	_, _, err := DecodeList(body, "workbooks", "workbook", func(element Element) error { got = element; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if values := got.GrandchildAttrValues("tags", "tag", "label"); fmt.Sprint(values) != "[daily certified]" {
		t.Fatalf("tag labels = %v", values)
	}
}

func TestDecodeListRejectsMalformedEnvelopes(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "wrong root", body: `<response><pagination pageNumber="1" pageSize="1" totalAvailable="0"/><users/></response>`, want: "tsResponse"},
		{name: "missing pagination", body: `<tsResponse><users/></tsResponse>`, want: "pagination"},
		{name: "duplicate pagination", body: `<tsResponse><pagination pageNumber="1" pageSize="1" totalAvailable="0"/><pagination pageNumber="1" pageSize="1" totalAvailable="0"/><users/></tsResponse>`, want: "pagination"},
		{name: "missing container", body: `<tsResponse><pagination pageNumber="1" pageSize="1" totalAvailable="0"/></tsResponse>`, want: "users"},
		{name: "duplicate container", body: `<tsResponse><pagination pageNumber="1" pageSize="1" totalAvailable="0"/><users/><users/></tsResponse>`, want: "users"},
		{name: "missing page size", body: `<tsResponse><pagination pageNumber="1" totalAvailable="0"/><users/></tsResponse>`, want: "pageSize"},
		{name: "invalid total", body: `<tsResponse><pagination pageNumber="1" pageSize="1" totalAvailable="many"/><users/></tsResponse>`, want: "totalAvailable"},
		{name: "malformed XML", body: `<tsResponse><pagination`, want: "XML"},
		{name: "custom entity", body: `<!DOCTYPE x [<!ENTITY secret "value">]><tsResponse><pagination pageNumber="1" pageSize="1" totalAvailable="1"/><users><user id="&secret;"/></users></tsResponse>`, want: "entity"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := DecodeList([]byte(test.body), "users", "user", func(Element) error { return nil })
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(test.want)) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestDecodePermissionsPreservesGranteeCapabilityPairs(t *testing.T) {
	body := []byte(`<tsResponse><permissions><workbook id="w1"/><granteeCapabilities><user id="u1"/><capabilities><capability name="Read" mode="Allow"/><capability name="Write" mode="Deny"/></capabilities></granteeCapabilities><granteeCapabilities><group id="g1"/><capabilities><capability name="Read" mode="Allow"/></capabilities></granteeCapabilities></permissions></tsResponse>`)
	grantees, err := DecodePermissions(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(grantees) != 2 || grantees[0].Type != "user" || grantees[0].ID != "u1" || len(grantees[0].Capabilities) != 2 || grantees[1].Type != "group" {
		t.Fatalf("grantees = %#v", grantees)
	}
}

func TestDecodePermissionsRejectsIncompleteAndUnexpectedShapes(t *testing.T) {
	tests := []struct {
		body string
		want string
	}{
		{body: `<response><permissions/></response>`, want: "tsResponse"},
		{body: `<tsResponse/>`, want: "permissions"},
		{body: `<tsResponse><permissions><workbook id="w1"/></permissions><permissions><workbook id="w1"/></permissions></tsResponse>`, want: "permissions"},
		{body: `<tsResponse><permissions><workbook id="w1"/><granteeCapabilities><user/><capabilities><capability name="Read" mode="Allow"/></capabilities></granteeCapabilities></permissions></tsResponse>`, want: "grantee"},
		{body: `<tsResponse><permissions><workbook id="w1"/><granteeCapabilities><user id="u1"/><capabilities><capability mode="Allow"/></capabilities></granteeCapabilities></permissions></tsResponse>`, want: "capability"},
	}
	for _, test := range tests {
		_, err := DecodePermissions([]byte(test.body))
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(test.want)) {
			t.Fatalf("error = %v", err)
		}
	}
}
