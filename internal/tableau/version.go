package tableau

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// MaximumAPIVersion is the highest released REST contract implemented by this build.
const MaximumAPIVersion = "3.29"

// NegotiateAPIVersion discovers the server's REST version before credentials are sent.
// Command construction calls it once per canonical server before constructing clients.
func (t *Transport) NegotiateAPIVersion(ctx context.Context, serverURL string) error {
	if t == nil {
		return errors.New("Tableau transport is not configured")
	}
	t.apiVersion = ""
	const operation = "server.info"
	response, err := t.Do(ctx, nil, Request{
		Method: http.MethodGet, ServerURL: serverURL, Path: "/api/2.4/serverinfo",
		Accept: "application/xml", Operation: operation, MaxResponseBytes: 64 * 1024,
	})
	if err != nil {
		return err
	}
	invalid := func(message string) error {
		failure := NewProtocolError(operation, response, errors.New(message), true)
		failure.correctiveAction = "Retry after Tableau returns valid Server Info version evidence."
		return failure
	}
	if response.StatusCode != http.StatusOK {
		return invalid("Server Info did not return HTTP 200")
	}
	var document struct {
		XMLName xml.Name `xml:"tsResponse"`
		Info    []struct {
			Versions []string `xml:"restApiVersion"`
		} `xml:"serverInfo"`
	}
	decoder := xml.NewDecoder(bytes.NewReader(response.Body))
	if err := decoder.Decode(&document); err != nil {
		// XML errors can contain rejected element names or other response text.
		return invalid("Server Info contains malformed XML")
	}
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return invalid("Server Info contains malformed XML")
		}
		if text, ok := token.(xml.CharData); ok && strings.TrimSpace(string(text)) == "" {
			continue
		}
		if _, ok := token.(xml.Comment); ok {
			continue
		}
		return invalid("Server Info contains content after its response document")
	}
	if len(document.Info) != 1 || len(document.Info[0].Versions) != 1 {
		return invalid("Server Info must contain exactly one supported REST API version")
	}
	reported, valid := parseRESTVersion(document.Info[0].Versions[0])
	if !valid {
		return invalid("Server Info REST API version must be numeric major.minor")
	}
	if reported.before(restVersion{3, 6}) {
		failure := NewProtocolError(operation, response, fmt.Errorf("server supports REST API %s; TADX requires REST API 3.6 or later for PAT authentication", reported), false)
		failure.correctiveAction = "Upgrade Tableau Server to REST API 3.6 or later to use PAT authentication with TADX."
		return failure
	}
	maximum, _ := parseRESTVersion(MaximumAPIVersion)
	selected := maximum
	if reported.before(maximum) {
		selected = reported
	}
	t.apiVersion = selected.String()
	return nil
}

type restVersion struct{ major, minor int }

func parseRESTVersion(value string) (restVersion, bool) {
	major, minor, found := strings.Cut(value, ".")
	if !found || major == "" || minor == "" {
		return restVersion{}, false
	}
	for _, part := range []string{major, minor} {
		for _, digit := range part {
			if digit < '0' || digit > '9' {
				return restVersion{}, false
			}
		}
	}
	majorNumber, majorErr := strconv.Atoi(major)
	minorNumber, minorErr := strconv.Atoi(minor)
	return restVersion{majorNumber, minorNumber}, majorErr == nil && minorErr == nil
}

func (v restVersion) before(other restVersion) bool {
	return v.major < other.major || v.major == other.major && v.minor < other.minor
}

func (v restVersion) String() string { return fmt.Sprintf("%d.%d", v.major, v.minor) }
