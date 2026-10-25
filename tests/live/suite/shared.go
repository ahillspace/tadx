package suite

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ahillspace/tadx/tests/live/harness"
	"gopkg.in/yaml.v3"
	_ "modernc.org/sqlite"
)

// SharedCases qualifies the shared runtime against real target observations.
func SharedCases() map[string]harness.Scenario {
	return map[string]harness.Scenario{
		"live.auth.check":                   sharedAuthCheck,
		"live.auth.reject-invalid-pat":      sharedAuthReject,
		"live.auth.login-native-store":      sharedNativeLogin,
		"live.doctor.live-readiness":        sharedDoctor,
		"live.cache.content-generation":     sharedCache("content"),
		"live.cache.admin-generation":       sharedCache("admin"),
		"live.cache.permission-generation":  sharedCache("permissions"),
		"live.cache.replace-generation":     sharedCacheReplace,
		"live.search.content-discovery":     sharedSearch("content"),
		"live.search.admin-discovery":       sharedSearch("admin"),
		"live.search.pulse-discovery":       sharedSearch("pulse"),
		"live.contract.batch-partial":       sharedBatch,
		"live.contract.output-real-result":  sharedOutput,
		"live.contract.permission-refusal":  sharedPermissionRefusal,
		"live.contract.preview-no-mutation": sharedPreview,
	}
}

func sharedAuthCheck(ctx context.Context, c *harness.Case) error {
	out, err := c.Run(ctx, "auth", "check", "--environment", c.Environment)
	if err != nil {
		return err
	}
	if contentString(out, "status") != "authenticated" || contentString(out, "site_luid") != c.API.SiteID || contentString(out, "user_luid") != c.ActorUserID {
		return errors.New("auth check target or actor differs from independent sign-in")
	}
	return nil
}

func sharedConfig(c *harness.Case, name string, change func(map[string]any) error) (string, error) {
	data, err := os.ReadFile(c.ConfigPath)
	if err != nil {
		return "", err
	}
	var doc map[string]any
	if err = yaml.Unmarshal(data, &doc); err != nil {
		return "", err
	}
	if err = change(doc); err != nil {
		return "", err
	}
	data, err = yaml.Marshal(doc)
	if err != nil {
		return "", err
	}
	path := filepath.Join(c.RunDir, name+".yaml")
	if err = os.WriteFile(path, data, 0600); err != nil {
		return "", err
	}
	return path, nil
}

func sharedSameServer(a, b string) bool {
	left, le := url.Parse(strings.TrimRight(a, "/"))
	right, re := url.Parse(strings.TrimRight(b, "/"))
	if le != nil || re != nil {
		return false
	}
	return strings.EqualFold(left.Scheme, right.Scheme) && strings.EqualFold(left.Host, right.Host) && left.EscapedPath() == right.EscapedPath()
}

// SharedBindingRequirements declares portable setup inputs for shared contracts.
func SharedBindingRequirements() map[string][]string {
	requirements := map[string][]string{}
	for id := range SharedCases() {
		requirements[id] = []string{}
	}
	for _, id := range []string{"live.cache.content-generation", "live.search.content-discovery"} {
		requirements[id] = []string{"workbook-native", "datasource-native"}
	}
	for _, id := range []string{"live.cache.permission-generation", "live.cache.replace-generation", "live.contract.batch-partial", "live.contract.preview-no-mutation"} {
		requirements[id] = []string{"workbook-native"}
	}
	requirements["live.search.admin-discovery"] = []string{"controlled_user"}
	requirements["live.search.pulse-discovery"] = []string{"pulse_datasource"}
	requirements["live.auth.login-native-store"] = []string{"native_login"}
	requirements["live.contract.permission-refusal"] = []string{"restricted_actor"}
	return requirements
}

// SharedFixtureCapabilities declares independent fixture mutations, restoration,
// and recovery commands, without granting unrelated subject operations.
func SharedFixtureCapabilities(id string) []string {
	switch id {
	case "live.auth.check", "live.auth.reject-invalid-pat", "live.doctor.live-readiness":
		return nil
	case "live.auth.login-native-store":
		return []string{"auth.check", "auth.logout"}
	case "live.cache.admin-generation":
		return []string{"admin.group.create", "admin.group.delete", "cache.status"}
	case "live.cache.content-generation":
		return []string{"project.create", "project.delete", "workbook.publish", "workbook.delete", "datasource.publish", "datasource.delete", "cache.status", "search.run"}
	case "live.cache.permission-generation":
		return []string{"project.create", "project.delete", "workbook.publish", "workbook.delete", "admin.group.create", "admin.group.delete", "admin.permission.create", "cache.status"}
	case "live.cache.replace-generation":
		return []string{"project.create", "project.delete", "workbook.publish", "workbook.delete", "workbook.update", "search.run"}
	case "live.search.content-discovery":
		return []string{"project.create", "project.delete", "workbook.publish", "workbook.delete", "datasource.publish", "datasource.delete"}
	case "live.search.admin-discovery":
		return []string{"admin.group.create", "admin.group.delete", "admin.user.create", "admin.user.delete"}
	case "live.search.pulse-discovery":
		return []string{"pulse.definition.create", "pulse.definition.delete"}
	case "live.contract.batch-partial":
		return []string{"project.create", "project.delete", "workbook.publish", "workbook.delete"}
	case "live.contract.output-real-result":
		return nil
	case "live.contract.preview-no-mutation":
		return []string{"project.create", "project.delete", "workbook.publish", "workbook.delete", "mutation.status"}
	case "live.contract.permission-refusal":
		return []string{"auth.check", "workbook.inspect", "workbook.update"}
	default:
		return nil
	}
}

// ValidateSharedBindings rejects incomplete setup inputs before remote fixtures.
func ValidateSharedBindings(id string, p harness.Profile) error {
	for _, key := range SharedBindingRequirements()[id] {
		b, ok := p.Bindings[key]
		if !ok {
			return harness.Blocked("target profile requires binding " + key)
		}
		if strings.Contains(key, "-native") {
			if err := ValidateContentBinding(key, b); err != nil {
				return err
			}
			continue
		}
		switch key {
		case "native_login":
			if b.Params["operator_assisted"] != "true" || b.Params["explicit_persistence_consent"] != "true" {
				return harness.Blocked("native login requires an operator and explicit native PAT persistence consent")
			}
		case "controlled_user":
			if err := harness.ValidateControlledUserTemplate(b.Name); err != nil {
				return harness.Blocked("controlled_user must provide an owned disposable login template containing {prefix}")
			}
		case "pulse_datasource":
			if b.ID == "" || b.Params["measure_field"] == "" || b.Params["date_field"] == "" {
				return harness.Blocked("pulse_datasource requires datasource LUID and authored measure/date fields")
			}
		case "restricted_actor":
			if b.Path == "" || b.ID == "" || b.Params["allowed_id"] == "" || b.Params["environment"] == "" {
				return harness.Blocked("restricted_actor requires config path, forbidden workbook ID, allowed_id, and environment")
			}
			data, err := os.ReadFile(b.Path)
			if err != nil {
				return harness.Blocked("restricted_actor source configuration is unavailable")
			}
			var doc map[string]any
			if yaml.Unmarshal(data, &doc) != nil {
				return harness.Blocked("restricted_actor source configuration is invalid")
			}
			environment := contentMap(contentMap(doc["environments"])[b.Params["environment"]])
			if !sharedSameServer(contentString(environment, "url"), p.ServerURL) || contentString(environment, "site_content_url") != p.SiteContentURL {
				return harness.Blocked("restricted_actor configuration targets a different server or exact site")
			}
			consent := false
			settings, _ := doc["site_mutations"].([]any)
			for _, raw := range settings {
				setting := contentMap(raw)
				if sharedSameServer(contentString(setting, "server_url"), p.ServerURL) && contentString(setting, "site_content_url") == p.SiteContentURL && setting["enabled"] == true {
					consent = true
				}
			}
			if !consent {
				return harness.Blocked("restricted_actor requires preexisting saved consent for the exact target; setup never changes it")
			}
		}
	}
	if id == "live.search.content-discovery" && p.Features["flows"] == "supported" {
		b, ok := p.Bindings["flow-native"]
		if !ok {
			return harness.Blocked("flow-enabled discovery requires flow-native")
		}
		return ValidateContentBinding("flow-native", b)
	}
	return nil
}
func sharedInvalidConfig(c *harness.Case, name string) (string, error) {
	return sharedConfig(c, name, func(doc map[string]any) error {
		environments := contentMap(doc["environments"])
		environment := contentMap(environments[c.Environment])
		if len(environment) == 0 {
			return errors.New("isolated config omits selected environment")
		}
		environment["auth"] = map[string]any{"type": "pat", "pat_name_env": "TADX_LIVE_INVALID_NAME", "pat_secret_env": "TADX_LIVE_INVALID_SECRET"}
		return nil
	})
}
func sharedAuthReject(ctx context.Context, c *harness.Case) error {
	path, err := sharedInvalidConfig(c, "invalid-auth")
	if err != nil {
		return err
	}
	saved := c.ExtraEnv
	c.ExtraEnv = append(slices.Clone(saved), "TADX_LIVE_INVALID_NAME=deliberately-invalid-live-pat", "TADX_LIVE_INVALID_SECRET=deliberately-invalid-live-secret")
	defer func() { c.ExtraEnv = saved }()
	out, commandErr := c.Run(ctx, "auth", "check", "--environment", c.Environment, "--config", path)
	if commandErr == nil {
		return errors.New("invalid PAT unexpectedly authenticates")
	}
	failure := contentMap(out["error"])
	if contentString(failure, "id") != "auth.check.authenticate" {
		return fmt.Errorf("invalid PAT fails outside authentication: %s", contentString(failure, "id"))
	}
	if contentString(out, "status") == "authenticated" || contentString(out, "user_luid") != "" {
		return errors.New("invalid PAT result claims authenticated identity")
	}
	last := c.Commands[len(c.Commands)-1]
	for _, value := range []string{"deliberately-invalid-live-secret", "deliberately-invalid-live-pat"} {
		if strings.Contains(last.Stdout+last.Stderr, value) {
			return errors.New("invalid PAT material appears in captured output")
		}
	}
	return nil
}

func sharedNativeListing(ctx context.Context) (string, error) {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.CommandContext(ctx, "cmdkey.exe", "/list")
	case "darwin":
		return "", harness.Unsupported("native-store inventory qualification currently requires Windows Credential Manager")
	default:
		return "", harness.Unsupported("native-store inventory qualification currently requires Windows Credential Manager")
	}
	data, err := command.Output()
	if err != nil {
		return "", harness.Blocked("native credential inventory is unavailable")
	}
	return string(data), nil
}
func sharedNativeLogin(ctx context.Context, c *harness.Case) error {
	b, err := c.Require("native_login")
	if err != nil {
		return err
	}
	if b.Params["explicit_persistence_consent"] != "true" || b.Params["operator_assisted"] != "true" {
		return harness.Blocked("native login requires an operator and explicit approval to persist the supplied PAT")
	}
	before, err := sharedNativeListing(ctx)
	if err != nil {
		return err
	}
	path, err := sharedConfig(c, "native-login", func(doc map[string]any) error {
		environment := contentMap(contentMap(doc["environments"])[c.Environment])
		environment["auth"] = map[string]any{"type": "pat"}
		return nil
	})
	if err != nil {
		return err
	}
	result, commandErr := c.Interactive(ctx, "auth", "login", "--environment", c.Environment, "--config", path, "--json")
	data, err := os.ReadFile(path)
	if err != nil {
		return errors.Join(commandErr, err)
	}
	var cfg map[string]any
	if err = yaml.Unmarshal(data, &cfg); err != nil {
		return err
	}
	auth := contentMap(contentMap(contentMap(cfg["environments"])[c.Environment])["auth"])
	ref := contentString(auth, "credential_ref")
	account := "pat:v1:" + strings.TrimPrefix(ref, "cred_")
	ownedTarget := "io.github.ahillspace.tadx:" + account
	if ref != "" {
		c.DeferCleanup("remove owned native credential", func(cleanup context.Context) error {
			_, err := c.Run(cleanup, "auth", "logout", "--environment", c.Environment, "--config", path)
			if err != nil {
				return err
			}
			listing, err := sharedNativeListing(cleanup)
			if err != nil {
				return err
			}
			if strings.Contains(listing, ownedTarget) {
				return errors.New("native logout leaves owned credential record")
			}
			return nil
		})
	}
	if commandErr != nil {
		return commandErr
	}
	if result.ExitCode != 0 || !strings.HasPrefix(ref, "cred_") || len(ref) != 37 {
		return errors.New("native login does not persist one opaque credential reference")
	}
	if contentString(auth, "pat_name_env", "pat_secret_env") != "" || len(auth) != 2 {
		return errors.New("native-login configuration contains values beyond auth type and opaque reference")
	}
	after, err := sharedNativeListing(ctx)
	if err != nil {
		return err
	}
	if strings.Contains(before, ownedTarget) || strings.Count(after, ownedTarget) != 1 {
		return errors.New("native OS inventory does not contain exactly one new owned credential record")
	}
	saved := c.ExtraEnv
	c.ExtraEnv = slices.Clone(saved)
	for _, key := range []string{c.Profile.ActorPAT.NameEnv, c.Profile.ActorPAT.SecretEnv} {
		if key != "" {
			c.ExtraEnv = append(c.ExtraEnv, key+"=")
		}
	}
	defer func() { c.ExtraEnv = saved }()
	out, err := c.Run(ctx, "auth", "check", "--environment", c.Environment, "--config", path)
	if err != nil {
		return err
	}
	if contentString(out, "credential_source") != "os_keyring" || contentString(out, "site_luid") != c.API.SiteID || contentString(out, "user_luid") != c.ActorUserID {
		return errors.New("fresh native-store authentication changes source, site, or actor identity")
	}
	return nil
}

func sharedDoctor(ctx context.Context, c *harness.Case) error {
	out, err := c.Run(ctx, "doctor", "--environment", c.Environment, "--workspace", c.Workspace, "--full")
	if err != nil {
		return err
	}
	scope := contentMap(out["scope"])
	if contentString(scope, "environment") != c.Environment || contentString(scope, "workspace") != c.Workspace {
		return errors.New("doctor reports a different environment or workspace")
	}
	checks, _ := out["checks"].([]any)
	for _, raw := range checks {
		check := contentMap(raw)
		if contentString(check, "id") == "auth.tableau.connectivity" {
			return c.Assert(contentString(check, "status") == "pass", "doctor live connectivity does not pass")
		}
	}
	return errors.New("doctor omits live connectivity check")
}

func sharedContentFixtureName(prefix, token, suffix string) string {
	marker := token
	if marker == "" {
		marker = "discovery"
	}
	name := prefix + "-" + marker
	if suffix != "" {
		name += "-" + suffix
	}
	return name
}

func sharedContentFixtures(ctx context.Context, c *harness.Case, includeFlow bool, token string) ([]harness.Object, error) {
	marker := token
	if marker == "" {
		marker = "discovery"
	}
	project, err := c.Project(ctx, marker)
	if err != nil {
		return nil, err
	}
	objects := []harness.Object{project}
	kinds := []string{"workbook", "datasource"}
	if includeFlow {
		kinds = append(kinds, "flow")
	}
	for _, kind := range kinds {
		o, err := contentPublishSeed(ctx, c, kind, project, sharedContentFixtureName(c.NamePrefix, token, kind), "")
		if err != nil {
			return nil, err
		}
		objects = append(objects, o)
	}
	return objects, nil
}
func sharedSearch(kind string) harness.Scenario {
	return func(ctx context.Context, c *harness.Case) error {
		var expected []harness.Object
		var contrast harness.Object
		query := c.NamePrefix
		var err error
		switch kind {
		case "content":
			contrast, err = c.Project(ctx, "contrast")
			if err != nil {
				return err
			}
			query = harness.NewRunID()
			expected, err = sharedContentFixtures(ctx, c, c.Profile.Features["flows"] == "supported", query)
			if c.Profile.Features["flows"] != "supported" {
				c.Note("Flow discovery is not exercised because the target profile does not mark flows supported.")
			}
		case "admin":
			user, e := c.Seed(ctx, "user", "discovery")
			if e != nil {
				return e
			}
			token, e := sharedAdminSearchToken(c, user)
			if e != nil {
				return e
			}
			group, e := c.Seed(ctx, "group", "discovery-"+token)
			if e != nil {
				return e
			}
			query = token
			expected = []harness.Object{group, user}
		case "pulse":
			definition, e := c.Seed(ctx, "pulseDefinition", "discovery")
			if e != nil {
				return e
			}
			expected = []harness.Object{definition}
			metric, e := pulseDefault(ctx, c, definition.ID)
			if e != nil {
				return e
			}
			name := contentString(metric, "name")
			if name == "" {
				name = definition.Name
			}
			expected = append(expected, harness.Object{Kind: "metric", ID: pulseID(metric), Name: name})
		}
		if err != nil {
			return err
		}
		observe, cancel := context.WithTimeout(ctx, time.Duration(c.Profile.Budgets.ObservationSeconds)*time.Second)
		defer cancel()
		for {
			out, err := c.Run(observe, "search", query, "--type", kind, "--environment", c.Environment, "--limit", "100", "--full")
			if err != nil {
				return err
			}
			err = sharedSearchFacts(out, expected)
			if err == nil && kind == "content" {
				if contentString(out, "source") != "live" {
					return errors.New("live content search did not report its live source")
				}
				err = sharedSearchFixtureNamesIsolated(c.NamePrefix, query, contrast, expected)
			}
			if err == nil {
				return nil
			}
			if kind != "content" {
				return err
			}
			select {
			case <-observe.Done():
				return fmt.Errorf("indexed content search never establishes authored fixture identities: %w", err)
			case <-time.After(2 * time.Second):
			}
		}
	}
}

func sharedAdminSearchToken(c *harness.Case, user harness.Object) (string, error) {
	if c == nil || c.Journal == nil || user.Kind != "user" || user.ID == "" || user.Name == "" {
		return "", errors.New("admin search requires its exact controlled-user identity")
	}
	binding, err := c.Require("controlled_user")
	if err != nil {
		return "", err
	}
	expectedLogin, err := c.ControlledUserLogin("discovery")
	if err != nil {
		return "", err
	}
	if expectedLogin != user.Name {
		return "", errors.New("admin search user differs from its validated controlled-user login")
	}
	templatePrefix, templateSuffix, found := strings.Cut(binding.Name, "{prefix}")
	if !found {
		return "", errors.New("admin search controlled-user template is invalid")
	}
	token, found := strings.CutPrefix(user.Name, templatePrefix)
	if !found {
		return "", errors.New("admin search user does not contain its generated token")
	}
	token, found = strings.CutSuffix(token, templateSuffix)
	if !found || token == "" {
		return "", errors.New("admin search user does not contain its generated token")
	}

	ownerKey := c.NamePrefix + "-user-discovery"
	matches := 0
	recordedToken := ""
	for _, entry := range c.Journal.Entries {
		if entry.Kind != "user" || entry.Object == nil || entry.Object.ID != user.ID {
			continue
		}
		if entry.Done || entry.Name != ownerKey || entry.UserLogin != user.Name ||
			entry.UserTemplate != binding.Name || entry.UserToken != token {
			return "", errors.New("admin search user token differs from its exact ownership receipt")
		}
		matches++
		recordedToken = entry.UserToken
	}
	if matches != 1 || recordedToken == "" {
		return "", errors.New("admin search user token lacks one exact ownership receipt")
	}
	return recordedToken, nil
}

func sharedSearchFacts(out map[string]any, expected []harness.Object) error {
	if source := contentString(out, "source"); source != "live" && source != "cache" {
		return errors.New("search omits its live or cache source")
	}
	rows, ok := out["items"].([]any)
	if !ok {
		return errors.New("search omits typed item array")
	}
	page := contentMap(out["page"])
	limit, limitOK := searchPageInteger(page["limit"])
	returned, returnedOK := searchPageInteger(page["returned"])
	_, moreOK := page["more_available"].(bool)
	if !limitOK || limit < 1 || !returnedOK || returned != len(rows) || len(rows) > limit || !moreOK {
		return errors.New("search result omits bounded page coverage metadata")
	}
	found := map[string]map[string]any{}
	for _, raw := range rows {
		item := contentMap(raw)
		id := contentString(item, "luid")
		if _, duplicate := found[id]; duplicate {
			return errors.New("search repeats an identity")
		}
		found[id] = item
	}
	for _, want := range expected {
		item, ok := found[want.ID]
		if !ok {
			return fmt.Errorf("search omits independently seeded %s", want.Kind)
		}
		actualType := contentString(item, "type")
		expectedType := strings.TrimPrefix(want.Kind, "pulse")
		if strings.HasPrefix(want.Kind, "pulse") {
			expectedType = strings.ToLower(expectedType)
		}
		if actualType != want.Kind && actualType != expectedType && actualType != "pulse_"+expectedType {
			return errors.New("search changes resource type")
		}
		if contentString(item, "name") != want.Name {
			return errors.New("search name differs from independent fixture")
		}
	}
	return nil
}

func searchPageInteger(value any) (int, bool) {
	switch number := value.(type) {
	case float64:
		return int(number), number >= 0 && number == float64(int(number))
	case int:
		return number, number >= 0
	case json.Number:
		parsed, err := strconv.Atoi(number.String())
		return parsed, err == nil && parsed >= 0
	default:
		return 0, false
	}
}

func sharedSearchFixtureNamesIsolated(prefix, token string, contrast harness.Object, expected []harness.Object) error {
	if !sharedOpaqueSearchToken(token) || contrast.Name == "" || strings.Contains(contrast.Name, token) || strings.Contains(contrast.Description, token) || strings.Contains(contrast.ProjectID, token) {
		return errors.New("opaque search fixture token leaks into contrast project identity or description")
	}
	for _, key := range []string{"name", "description", "path", "project_path", "parentProjectPath", "parentProjectId"} {
		if strings.Contains(contentString(contrast.Fields, key), token) {
			return errors.New("opaque search fixture token leaks into contrast project fields")
		}
	}
	parent := contentMap(contrast.Fields["parentProject"])
	for _, key := range []string{"name", "path"} {
		if strings.Contains(contentString(parent, key), token) {
			return errors.New("opaque search fixture token leaks into contrast parent project fields")
		}
	}
	for _, fixture := range expected {
		if fixture.Name == "" || !strings.Contains(fixture.Name, token) {
			return errors.New("opaque search fixture token is absent from a matching fixture name")
		}
	}
	return nil
}

func sharedOpaqueSearchToken(token string) bool {
	if len(token) != 20 {
		return false
	}
	for _, char := range token {
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

func sharedBatch(ctx context.Context, c *harness.Case) error {
	workbook, err := c.Content(ctx, "workbook", "")
	if err != nil {
		return err
	}
	missing := "00000000-0000-4000-8000-000000000099"
	response, e := c.API.Do(ctx, "GET", c.API.REST("workbooks/"+missing), nil, "")
	if e == nil || response.Status != 404 {
		return harness.Blocked("batch missing fixture ID is not independently absent")
	}
	name := c.NamePrefix + "-batch-updated"
	out, commandErr := contentRun(ctx, c, "workbook", "update", "--id", workbook.ID, "--id", missing, "--new-name", name)
	if commandErr == nil {
		return errors.New("partial batch reports aggregate success")
	}
	if err = sharedValidateBatchOutcomes(out, workbook.ID, missing); err != nil {
		return err
	}
	if _, err = contentAwaitObject(ctx, c, "workbook", workbook.ID, func(after harness.Object) error {
		if after.Name != name {
			return errors.New("batch discards or rolls back the confirmed first update")
		}
		return nil
	}); err != nil {
		return err
	}
	response, e = c.API.Do(ctx, "GET", c.API.REST("workbooks/"+missing), nil, "")
	if e == nil || response.Status != 404 {
		return errors.New("batch invents missing second identity")
	}
	return nil
}

func sharedValidateBatchOutcomes(out map[string]any, succeededID, failedID string) error {
	items, ok := out["items"].([]any)
	if !ok || len(items) != 2 {
		return errors.New("partial batch omits two ordered item outcomes")
	}
	first, second := contentMap(items[0]), contentMap(items[1])
	if contentString(first, "selector") != "id="+succeededID || contentString(second, "selector") != "id="+failedID || !contentHasIdentity(first, succeededID) {
		return errors.New("batch changes ordered selected identities")
	}
	if contentString(first, "status") != "succeeded" || contentString(second, "status") != "failed" {
		return errors.New("batch does not preserve confirmed success plus failure")
	}
	return nil
}

func sharedPreview(ctx context.Context, c *harness.Case) error {
	workbook, err := c.Content(ctx, "workbook", "")
	if err != nil {
		return err
	}
	path, err := sharedConfig(c, "without-consent", func(doc map[string]any) error {
		delete(doc, "site_mutations")
		delete(doc, "mutations_enabled")
		return nil
	})
	if err != nil {
		return err
	}
	status, err := c.Run(ctx, "mutation", "status", "--environment", c.Environment, "--config", path)
	if err != nil {
		return err
	}
	if status["enabled"] == true {
		return errors.New("isolated no-consent config still reports enabled")
	}
	args := []string{"--id", workbook.ID, "--new-name", c.NamePrefix + "-preview", "--config", path}
	preview, err := contentRun(ctx, c, "workbook", "update", append(slices.Clone(args), "--preview")...)
	if err != nil {
		return err
	}
	if !contentHasIdentity(preview, workbook.ID) {
		return errors.New("preview loses authoritative selected identity")
	}
	if err = contentUnchanged(ctx, c, workbook); err != nil {
		return err
	}
	out, commandErr := contentRun(ctx, c, "workbook", "update", args...)
	if commandErr == nil {
		return errors.New("execution succeeds without persisted site consent")
	}
	if contentString(contentMap(out["error"]), "id") != "mutation.disabled" {
		return errors.New("no-consent execution fails outside the exact consent gate")
	}
	c.Note("Independent unchanged-state checks complement offline zero-write request tests; transient writes are not inferred from final state.")
	return contentUnchanged(ctx, c, workbook)
}

func sharedPermissionRefusal(ctx context.Context, c *harness.Case) error {
	binding, err := c.Require("restricted_actor")
	if err != nil {
		return err
	}
	if binding.Path == "" || binding.ID == "" || binding.Params["environment"] == "" || binding.Params["allowed_id"] == "" {
		return harness.Blocked("restricted actor requires isolated config source, environment, forbidden ID, and allowed_id")
	}
	forbidden, err := c.API.Get(ctx, "workbook", binding.ID)
	if err != nil {
		return err
	}
	before, err := c.API.Download(ctx, "workbook", forbidden.ID)
	if err != nil {
		return err
	}
	configBytes, err := os.ReadFile(binding.Path)
	if err != nil {
		return err
	}
	path := filepath.Join(c.RunDir, "restricted-actor.yaml")
	if err = os.WriteFile(path, configBytes, 0600); err != nil {
		return err
	}
	auth, err := c.Run(ctx, "auth", "check", "--config", path, "--environment", binding.Params["environment"])
	if err != nil {
		return err
	}
	if contentString(auth, "status") != "authenticated" || contentString(auth, "site_luid") != c.API.SiteID || !sharedSameServer(contentString(auth, "server_url"), c.Profile.ServerURL) {
		return errors.New("restricted actor does not authenticate to exact site")
	}
	allowed, err := c.Run(ctx, "content", "workbook", "inspect", "--id", binding.Params["allowed_id"], "--config", path, "--environment", binding.Params["environment"])
	if err != nil {
		return err
	}
	if !contentHasIdentity(allowed, binding.Params["allowed_id"]) {
		return errors.New("restricted actor does not read allowed fixture")
	}
	if err = c.RestoreRequest("PUT", c.API.REST("workbooks/"+forbidden.ID), map[string]any{"workbook": map[string]any{"name": forbidden.Name}}); err != nil {
		return err
	}
	out, commandErr := c.Run(ctx, "content", "workbook", "update", "--id", forbidden.ID, "--new-name", c.NamePrefix+"-forbidden", "--config", path, "--environment", binding.Params["environment"])
	if commandErr == nil {
		return errors.New("restricted actor unexpectedly updates forbidden workbook")
	}
	failure := contentMap(out["error"])
	if !sharedAuthorizationFailure(failure) {
		return errors.New("restricted actor failure does not establish Tableau authorization refusal")
	}
	if contentString(failure, "environment") != binding.Params["environment"] {
		return errors.New("authorization refusal changes selected target context")
	}
	if err = contentUnchanged(ctx, c, forbidden); err != nil {
		return err
	}
	after, err := c.API.Download(ctx, "workbook", forbidden.ID)
	if err != nil {
		return err
	}
	return harness.CompareNative("workbook", before.Bytes, after.Bytes)
}
func sharedAuthorizationFailure(failure map[string]any) bool {
	code := strings.ToLower(contentString(failure, "kind", "code"))
	if code == "authorization" || code == "forbidden" {
		return true
	}
	for _, key := range []string{"upstream_status", "status", "http_status", "http_status_code"} {
		if fmt.Sprint(failure[key]) == "403" {
			return true
		}
	}
	detail := strings.ToLower(contentString(failure, "summary", "upstream_cause"))
	return strings.Contains(detail, "403") || strings.Contains(detail, "forbidden") || strings.Contains(detail, "permission denied")
}

func sharedOutput(ctx context.Context, c *harness.Case) error {
	workbook, _, err := contentRetainedWorkbookPair(ctx, c)
	if err != nil {
		return err
	}
	if err = contentAwaitWorkbookMetadata(ctx, c, workbook); err != nil {
		return err
	}
	oracle := workbook
	toon, err := c.Command(ctx, "content", "workbook", "inspect", "--id", workbook.ID, "--environment", c.Environment)
	if err != nil {
		return err
	}
	facts, err := sharedTOONFacts(toon.Stdout)
	if err != nil {
		return err
	}
	if facts["status"] != "found" || facts["workbook.luid"] != oracle.ID || facts["workbook.name"] != oracle.Name {
		return errors.New("TOON known fields differ from independent workbook identity")
	}
	compact, err := contentRun(ctx, c, "workbook", "inspect", "--id", workbook.ID)
	if err != nil {
		return err
	}
	full, err := contentRun(ctx, c, "workbook", "inspect", "--id", workbook.ID, "--full")
	if err != nil {
		return err
	}
	if contentString(compact, "status") != "found" || contentString(full, "status") != "found" {
		return errors.New("JSON inspection changes operation outcome")
	}
	if err = contentIdentity(contentMap(full["workbook"]), oracle); err != nil {
		return err
	}
	if contentString(contentMap(compact["workbook"]), "luid") != oracle.ID {
		return errors.New("compact JSON changes identity")
	}
	return sharedOutputDetails(contentMap(full["workbook"]), contentMap(compact["workbook"]), oracle)
}

func sharedOutputDetails(full, compact map[string]any, oracle harness.Object) error {
	if oracle.OwnerID == "" || contentString(full, "owner_luid") != oracle.OwnerID {
		return errors.New("full output owner differs from independent workbook truth")
	}
	createdAt, ok := oracle.Fields["createdAt"].(string)
	if !ok || createdAt == "" {
		return errors.New("independent workbook truth omits created timestamp")
	}
	fullCreatedAt, ok := full["created_at"].(string)
	if !ok || !sameTimestamp(fullCreatedAt, createdAt) {
		return errors.New("full output created timestamp differs from independent workbook truth")
	}
	for _, key := range []string{"owner_luid", "created_at"} {
		if _, exists := compact[key]; exists {
			return fmt.Errorf("compact output includes expanded workbook field %s", key)
		}
	}
	return nil
}

func sameTimestamp(left, right string) bool {
	leftTime, leftErr := time.Parse(time.RFC3339Nano, left)
	rightTime, rightErr := time.Parse(time.RFC3339Nano, right)
	return leftErr == nil && rightErr == nil && leftTime.Equal(rightTime)
}

// sharedTOONFacts independently decodes scalar object fields needed by this
// contract. Unrelated arrays remain outside this deliberately bounded decoder.
func sharedTOONFacts(text string) (map[string]string, error) {
	facts := map[string]string{}
	type level struct {
		indent int
		key    string
	}
	stack := []level{}
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
			stack = stack[:len(stack)-1]
		}
		if strings.ContainsAny(key, "[{,") {
			continue
		}
		path := []string{}
		for _, entry := range stack {
			path = append(path, entry.key)
		}
		path = append(path, key)
		value = strings.TrimSpace(value)
		if value == "" {
			stack = append(stack, level{indent, key})
			continue
		}
		if strings.HasPrefix(value, "\"") {
			unquoted, err := strconv.Unquote(value)
			if err != nil {
				return nil, errors.New("TOON scalar string is invalid")
			}
			value = unquoted
		}
		name := strings.Join(path, ".")
		if _, exists := facts[name]; exists {
			return nil, errors.New("TOON repeats a scalar field")
		}
		facts[name] = value
	}
	return facts, scanner.Err()
}

type sharedGeneration struct {
	Key                   int64
	ID, Environment, Site string
	Complete              int
}

func sharedOpenCache(c *harness.Case) (*sql.DB, error) {
	paths, err := filepath.Glob(filepath.Join(filepath.Dir(c.ConfigPath), "catalog", "target-*.sqlite"))
	if err != nil {
		return nil, err
	}
	if len(paths) != 1 {
		return nil, errors.New("isolated config does not contain exactly one target-bound cache database")
	}
	path := filepath.ToSlash(paths[0])
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	u := url.URL{Scheme: "file", Path: path}
	db, err := sql.Open("sqlite", u.String()+"?mode=ro")
	if err != nil {
		return nil, err
	}
	return db, nil
}
func sharedCurrent(ctx context.Context, db *sql.DB, c *harness.Case) (sharedGeneration, error) {
	var g sharedGeneration
	err := db.QueryRowContext(ctx, `SELECT g.generation_key,g.id,g.environment,g.site,g.complete FROM current_generations AS c JOIN generations AS g USING(generation_key) WHERE c.environment=? AND c.site=?`, c.Environment, c.Profile.SiteContentURL).Scan(&g.Key, &g.ID, &g.Environment, &g.Site, &g.Complete)
	if err != nil {
		return g, err
	}
	if g.ID == "" || g.Complete != 1 {
		return g, errors.New("cache current generation is not committed and complete")
	}
	return g, nil
}

func sharedCache(which string) harness.Scenario {
	return func(ctx context.Context, c *harness.Case) error {
		if which == "content" && c.Profile.Features["flows"] == "unsupported" {
			return harness.Unsupported("complete content cache qualification requires flow inventory, which the target profile marks unsupported")
		}
		var fixtures []harness.Object
		var scopes []string
		switch which {
		case "content":
			objects, err := sharedContentFixtures(ctx, c, false, "")
			if err != nil {
				return err
			}
			fixtures = objects
			scopes = sharedContentCacheScopes()
		case "admin":
			group, err := c.Seed(ctx, "group", "cache")
			if err != nil {
				return err
			}
			fixtures = []harness.Object{group}
			scopes = []string{"users", "groups"}
		case "permissions":
			workbook, err := c.Content(ctx, "workbook", "")
			if err != nil {
				return err
			}
			fixtures = []harness.Object{workbook}
			group, err := c.Seed(ctx, "group", "cache-permissions")
			if err != nil {
				return err
			}
			payload := map[string]any{"permissions": map[string]any{"granteeCapabilities": []any{map[string]any{"group": map[string]any{"id": group.ID}, "capabilities": map[string]any{"capability": []any{map[string]any{"name": "Read", "mode": "Deny"}}}}}}}
			if _, err = c.API.JSON(ctx, "PUT", c.API.REST("workbooks/"+workbook.ID+"/permissions"), payload); err != nil {
				return err
			}
			scopes = []string{"permissions"}
		}
		args := []string{"cache", "refresh", "--environment", c.Environment}
		for _, scope := range scopes {
			args = append(args, "--scope", scope)
		}
		if _, err := c.Run(ctx, args...); err != nil {
			return err
		}
		status, err := c.Run(ctx, "cache", "status", "--environment", c.Environment, "--full")
		if err != nil {
			return err
		}
		db, err := sharedOpenCache(c)
		if err != nil {
			return err
		}
		defer db.Close()
		g, err := sharedCurrent(ctx, db, c)
		if err != nil {
			return err
		}
		if !contentHasIdentity(status, g.ID) && !sharedContainsString(status, g.ID) {
			return errors.New("cache status omits independently persisted generation identity")
		}
		for _, scope := range scopes {
			var requested, complete int
			if err = db.QueryRowContext(ctx, `SELECT requested,complete FROM generation_scopes WHERE generation_key=? AND scope=?`, g.Key, scope).Scan(&requested, &complete); err != nil {
				return err
			}
			if requested != 1 || (complete != 1 && scope != "permissions") {
				return fmt.Errorf("requested cache scope %s is incomplete", scope)
			}
		}
		if which == "permissions" {
			return sharedCachePermissions(ctx, c, db, g, fixtures[0], status)
		}
		for _, scope := range scopes {
			if err = sharedCacheRows(ctx, c, db, g, strings.TrimSuffix(scope, "s")); err != nil {
				return err
			}
		}
		if which == "admin" {
			c.Note("The current cache schema persists direct group metadata and user roles; it does not persist group membership relationships.")
			return nil
		}
		return sharedCachedSearch(ctx, c, fixtures)
	}
}

func sharedContentCacheScopes() []string {
	return []string{"projects", "workbooks", "datasources", "flows", "views"}
}

func sharedContainsString(v any, want string) bool {
	switch value := v.(type) {
	case string:
		return value == want
	case map[string]any:
		for _, item := range value {
			if sharedContainsString(item, want) {
				return true
			}
		}
	case []any:
		for _, item := range value {
			if sharedContainsString(item, want) {
				return true
			}
		}
	}
	return false
}

func sharedCacheRows(ctx context.Context, c *harness.Case, db *sql.DB, g sharedGeneration, kind string) error {
	oracle, err := c.API.List(ctx, kind, nil)
	if err != nil {
		return err
	}
	var query string
	switch kind {
	case "project":
		query = `SELECT id,name,parent_project_id,owner_id FROM projects WHERE generation_key=?`
	case "workbook", "datasource", "flow":
		query = `SELECT id,name,project_id,owner_id FROM ` + kind + `s WHERE generation_key=?`
	case "view":
		query = `SELECT id,name,workbook_id,'' FROM views WHERE generation_key=?`
	case "user":
		query = `SELECT id,name,site_role,'' FROM users WHERE generation_key=?`
	case "group":
		query = `SELECT id,name,domain,'' FROM groups WHERE generation_key=?`
	default:
		return errors.New("unsupported typed cache comparison")
	}
	rows, err := db.QueryContext(ctx, query, g.Key)
	if err != nil {
		return err
	}
	defer rows.Close()
	actual := map[string][3]string{}
	for rows.Next() {
		var id, name, parent, owner string
		if err = rows.Scan(&id, &name, &parent, &owner); err != nil {
			return err
		}
		actual[id] = [3]string{name, parent, owner}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if len(actual) != len(oracle) {
		return fmt.Errorf("cache %s identity count differs from independent REST", kind)
	}
	for _, o := range oracle {
		parent := o.ProjectID
		owner := o.OwnerID
		switch kind {
		case "project":
			parent = contentParent(o)
		case "user":
			parent = contentString(o.Fields, "siteRole")
			owner = ""
		case "group":
			parent = contentString(contentMap(o.Fields["domain"]), "name")
			owner = ""
		case "view":
			parent = contentString(contentMap(o.Fields["workbook"]), "id")
			owner = ""
		}
		if actual[o.ID] != [3]string{o.Name, parent, owner} {
			return fmt.Errorf("cache %s identity or relationship differs from independent REST", kind)
		}
	}
	return nil
}

func sharedCachePermissions(ctx context.Context, c *harness.Case, db *sql.DB, g sharedGeneration, o harness.Object, status map[string]any) error {
	oracle, err := c.API.JSON(ctx, "GET", c.API.REST("workbooks/"+o.ID+"/permissions"), nil)
	if err != nil {
		return err
	}
	expected := []string{}
	permissions := contentMap(oracle["permissions"])
	grantees, _ := permissions["granteeCapabilities"].([]any)
	for _, raw := range grantees {
		grantee := contentMap(raw)
		principalType := "user"
		principal := contentString(contentMap(grantee["user"]), "id")
		if group := contentString(contentMap(grantee["group"]), "id"); group != "" {
			principalType = "group"
			principal = group
		}
		caps, _ := contentMap(grantee["capabilities"])["capability"].([]any)
		for _, rawCap := range caps {
			capability := contentMap(rawCap)
			expected = append(expected, strings.Join([]string{principalType, principal, contentString(capability, "name"), contentString(capability, "mode")}, "\x00"))
		}
	}
	if len(expected) == 0 {
		return harness.Blocked("permission-cache fixture requires independently observed explicit capabilities")
	}
	rows, err := db.QueryContext(ctx, `SELECT grantee_type,grantee_id,capability,mode FROM permissions WHERE generation_key=? AND content_type='workbook' AND content_id=?`, g.Key, o.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	actual := []string{}
	for rows.Next() {
		var kind, id, capability, mode string
		if err = rows.Scan(&kind, &id, &capability, &mode); err != nil {
			return err
		}
		actual = append(actual, strings.Join([]string{kind, id, capability, mode}, "\x00"))
	}
	if err = rows.Err(); err != nil {
		return err
	}
	slices.Sort(actual)
	slices.Sort(expected)
	if !slices.Equal(actual, expected) {
		return errors.New("persisted permission principal/capability/mode tuples differ from independent REST")
	}
	for _, scope := range []string{"projects", "workbooks"} {
		var complete int
		if err = db.QueryRowContext(ctx, `SELECT complete FROM generation_scopes WHERE generation_key=? AND scope=?`, g.Key, scope).Scan(&complete); err != nil {
			return err
		}
		if complete != 1 {
			return errors.New("implicit permission inventory scope is not complete")
		}
	}
	var complete int
	if err = db.QueryRowContext(ctx, `SELECT complete FROM generation_scopes WHERE generation_key=? AND scope='permissions'`, g.Key).Scan(&complete); err != nil {
		return err
	}
	coverage, _ := contentMap(status["generation"])["coverage"].([]any)
	found := false
	for _, raw := range coverage {
		item := contentMap(raw)
		if contentString(item, "scope") == "permissions" {
			found = true
			if item["complete"] != (complete == 1) {
				return errors.New("cache status misreports independently persisted permission coverage")
			}
		}
	}
	if !found {
		return errors.New("cache status omits explicit permission coverage")
	}
	if complete == 0 {
		warnings, _ := status["warnings"].([]any)
		if len(warnings) == 0 {
			return errors.New("partial permission generation omits coverage diagnostics")
		}
		c.Note("The permission generation retains accessible rules and reports incomplete coverage; target-wide denied resources are not treated as empty rule sets.")
	}
	return nil
}

func sharedCachedSearch(ctx context.Context, c *harness.Case, fixtures []harness.Object) error {
	path, err := sharedInvalidConfig(c, "cache-offline")
	if err != nil {
		return err
	}
	var egress atomic.Int64
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { egress.Add(1); w.WriteHeader(http.StatusForbidden) }))
	defer proxy.Close()
	saved := c.ExtraEnv
	c.ExtraEnv = append(slices.Clone(saved), "HTTPS_PROXY="+proxy.URL, "HTTP_PROXY="+proxy.URL, "ALL_PROXY="+proxy.URL, "NO_PROXY=", "TADX_LIVE_INVALID_NAME=cache-no-credential", "TADX_LIVE_INVALID_SECRET=cache-no-credential")
	defer func() { c.ExtraEnv = saved }()
	out, err := c.Run(ctx, "search", c.NamePrefix, "--type", "content", "--cache", "--environment", c.Environment, "--limit", "100", "--config", path)
	if err != nil {
		return err
	}
	if egress.Load() != 0 {
		return errors.New("cache-only search attempts Tableau proxy egress")
	}
	if contentString(out, "source") != "cache" {
		return errors.New("explicit cache search claims another source")
	}
	if err = sharedSearchFacts(out, fixtures); err != nil {
		return err
	}
	c.Note("Cached search succeeds without usable PAT references and makes zero requests through the process proxy; OS firewall isolation is not asserted.")
	return nil
}

func sharedCacheReplace(ctx context.Context, c *harness.Case) error {
	project, err := c.Project(ctx, "cache-replace")
	if err != nil {
		return err
	}
	a, err := contentPublishSeed(ctx, c, "workbook", project, c.NamePrefix+"-a", "")
	if err != nil {
		return err
	}
	b, err := contentPublishSeed(ctx, c, "workbook", project, c.NamePrefix+"-b", "")
	if err != nil {
		return err
	}
	if _, err = c.Run(ctx, "cache", "refresh", "--scope", "projects", "--scope", "workbooks", "--environment", c.Environment); err != nil {
		return err
	}
	db, err := sharedOpenCache(c)
	if err != nil {
		return err
	}
	first, err := sharedCurrent(ctx, db, c)
	if err != nil {
		db.Close()
		return err
	}
	var preserved string
	err = db.QueryRowContext(ctx, `SELECT generation_id FROM resource_scope_snapshots WHERE environment=? AND site=? AND kind='project'`, c.Environment, c.Profile.SiteContentURL).Scan(&preserved)
	db.Close()
	if err != nil {
		return err
	}
	name := c.NamePrefix + "-renamed"
	if _, err = c.API.Update(ctx, "workbook", a.ID, map[string]any{"name": name}); err != nil {
		return err
	}
	if err = c.API.Delete(ctx, "workbook", b.ID); err != nil {
		return err
	}
	if err = contentAwaitAbsent(ctx, c, "workbook", b.ID); err != nil {
		return err
	}
	if err = contentObserve(ctx, c, func() error {
		objects, err := c.API.List(ctx, "workbook", nil)
		if err != nil {
			return err
		}
		found := false
		for _, o := range objects {
			if o.ID == b.ID {
				return errors.New("deleted fixture remains in inventory")
			}
			if o.ID == a.ID {
				found = true
				if o.Name != name {
					return errors.New("renamed fixture inventory remains stale")
				}
			}
		}
		if !found {
			return errors.New("renamed fixture is missing from inventory")
		}
		return nil
	}); err != nil {
		return err
	}
	if _, err = c.Run(ctx, "cache", "refresh", "--scope", "workbooks", "--environment", c.Environment); err != nil {
		return err
	}
	db, err = sharedOpenCache(c)
	if err != nil {
		return err
	}
	defer db.Close()
	second, err := sharedCurrent(ctx, db, c)
	if err != nil {
		return err
	}
	if first.ID == second.ID {
		return errors.New("refresh does not commit a new generation")
	}
	if err = sharedCacheRows(ctx, c, db, second, "workbook"); err != nil {
		return err
	}
	var after string
	if err = db.QueryRowContext(ctx, `SELECT generation_id FROM resource_scope_snapshots WHERE environment=? AND site=? AND kind='project'`, c.Environment, c.Profile.SiteContentURL).Scan(&after); err != nil {
		return err
	}
	if after != preserved {
		return errors.New("workbook refresh replaces unselected project scope generation")
	}
	out, err := c.Run(ctx, "search", c.NamePrefix, "--type", "workbook", "--cache", "--environment", c.Environment, "--limit", "100")
	if err != nil {
		return err
	}
	if contentHasIdentity(out, b.ID) {
		return errors.New("deleted workbook remains in replaced cached search")
	}
	a.Name = name
	return sharedSearchFacts(out, []harness.Object{a})
}
