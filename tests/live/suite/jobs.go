package suite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ahillspace/tadx/tests/live/harness"
)

func contentJob(action string) harness.Scenario {
	return func(ctx context.Context, c *harness.Case) error {
		binding, err := c.Require("job-" + action)
		if err != nil {
			return err
		}
		method, path := binding.Params["submit_method"], binding.Params["submit_path"]
		if method != "POST" || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
			return errors.New("fresh job binding requires POST and a server-relative submit_path")
		}
		var body any
		if raw := binding.Params["submit_body"]; raw != "" {
			if err = json.Unmarshal([]byte(raw), &body); err != nil {
				return errors.New("fresh job binding submit_body is not valid JSON")
			}
		}
		accepted, err := c.API.JSON(ctx, method, path, body)
		if err != nil {
			return err
		}
		id := contentString(contentMap(accepted["job"]), "id")
		if id == "" {
			return errors.New("fresh independent job submission omits authoritative job ID")
		}
		if err = c.AcceptJob(id); err != nil {
			return err
		}
		before, err := c.API.Get(ctx, "job", id)
		if err != nil {
			return err
		}
		switch action {
		case "inspect":
			out, err := c.Run(ctx, "job", "inspect", "--environment", c.Environment, "--id", id)
			if err != nil {
				return err
			}
			after, err := c.API.Get(ctx, "job", id)
			if err != nil {
				return err
			}
			return contentJobMatches(out, before, after)
		case "wait":
			if contentJobTerminal(before) {
				c.Note("The first independent job read is terminal; receipt replay is covered, and active monitoring is unavailable.")
			}
			out, err := c.Run(ctx, "job", "wait", "--environment", c.Environment, "--id", id)
			if err != nil {
				return err
			}
			receipt := contentString(out, "receipt_path")
			if receipt == "" {
				return errors.New("job wait omits durable receipt path")
			}
			oracle, err := c.API.Get(ctx, "job", id)
			if err != nil {
				return err
			}
			if !contentJobTerminal(oracle) {
				return errors.New("job wait did not establish terminal outcome within observation budget")
			}
			if err = contentJobMatches(out, oracle, oracle); err != nil {
				return err
			}
			replay, err := c.Run(ctx, "job", "wait", "--receipt", receipt)
			if err != nil {
				return err
			}
			if contentString(replay, "site") != contentString(out, "site") || contentString(replay, "environment") != contentString(out, "environment") {
				return errors.New("fresh-process receipt replay changes exact target")
			}
			return contentJobMatches(replay, oracle, oracle)
		case "cancel":
			if contentJobTerminal(before) {
				return harness.Inconclusive("cancellation fixture finishes before preview; no cancellation coverage")
			}
			preview, err := c.Run(ctx, "job", "cancel", "--environment", c.Environment, "--id", id, "--preview")
			if err != nil {
				return err
			}
			if !contentHasIdentity(preview, id) {
				return errors.New("cancel preview loses job identity")
			}
			afterPreview, err := c.API.Get(ctx, "job", id)
			if err != nil {
				return err
			}
			if contentJobState(afterPreview) == "cancelled" {
				return errors.New("cancel preview changes active job into cancelled state")
			}
			if contentJobTerminal(afterPreview) {
				return harness.Inconclusive("cancellation fixture finishes before submission")
			}
			out, err := c.Run(ctx, "job", "cancel", "--environment", c.Environment, "--id", id)
			if err != nil {
				return err
			}
			if !contentHasIdentity(out, id) {
				return errors.New("cancel receipt loses exact job identity")
			}
			final, err := contentObserveJob(ctx, c, id)
			if err != nil {
				return err
			}
			if contentJobState(final) != "cancelled" {
				return fmt.Errorf("cancellation is not confirmed: independent terminal state is %s", contentJobState(final))
			}
			return nil
		}
		return errors.New("unsupported job scenario")
	}
}

func contentJobTerminal(o harness.Object) bool {
	return contentJobState(o) == "succeeded" || contentJobState(o) == "failed" || contentJobState(o) == "cancelled"
}
func contentJobState(o harness.Object) string {
	progress, _ := strconv.ParseFloat(fmt.Sprint(o.Fields["progress"]), 64)
	completed := contentString(o.Fields, "completedAt", "completed_at")
	jobType := strings.ToLower(contentString(o.Fields, "type"))
	if strings.Contains(jobType, "bridge") && fmt.Sprint(o.Fields["finishCode"]) == "0" {
		return "running"
	}
	if _, ok := o.Fields["progress"]; ok && progress < 100 && completed == "" {
		if progress == 0 {
			return "pending"
		}
		return "running"
	}
	if value, exists := o.Fields["finishCode"]; exists && value != nil && fmt.Sprint(value) != "" {
		if progress < 100 && completed == "" {
			return "unknown"
		}
		switch fmt.Sprint(value) {
		case "0":
			return "succeeded"
		case "1":
			return "failed"
		case "2":
			return "cancelled"
		case "3":
			if strings.Contains(jobType, "bridge") {
				return "succeeded"
			}
		}
	}
	status := strings.ToLower(contentString(o.Fields, "status"))
	if status == "success" || status == "completed" {
		return "succeeded"
	}
	if status == "canceled" {
		return "cancelled"
	}
	if status != "" {
		return status
	}
	return "running"
}
func contentJobMatches(out map[string]any, before, after harness.Object) error {
	job := contentMap(out["job"])
	if contentString(job, "id") != before.ID || after.ID != before.ID {
		return errors.New("CLI job identity differs from independent accepted ID")
	}
	expectedType := contentString(before.Fields, "type")
	if expectedType != "" && contentString(job, "type") != expectedType {
		return errors.New("CLI job type differs from independent REST")
	}
	state := contentString(job, "status")
	if state != contentJobState(before) && state != contentJobState(after) {
		return errors.New("CLI job status differs from independent observations")
	}
	for _, key := range []string{"progress"} {
		value, exists := job[key]
		if !exists {
			continue
		}
		n, err := strconv.ParseFloat(fmt.Sprint(value), 64)
		if err != nil || n < 0 || n > 100 {
			return errors.New("CLI job progress is invalid")
		}
		if contentJobTerminal(before) && fmt.Sprint(before.Fields[key]) != fmt.Sprint(value) {
			return errors.New("terminal job progress differs from independent REST")
		}
	}
	return nil
}
func contentObserveJob(ctx context.Context, c *harness.Case, id string) (harness.Object, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(c.Profile.Budgets.ObservationSeconds)*time.Second)
	defer cancel()
	for {
		job, err := c.API.Get(ctx, "job", id)
		if err != nil {
			return job, err
		}
		if contentJobTerminal(job) {
			return job, nil
		}
		select {
		case <-ctx.Done():
			return job, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func contentDetached(ctx context.Context, c *harness.Case, kind, action string) error {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(c.Profile.Budgets.OperationSeconds)*time.Second)
	defer cancel()
	var source harness.Object
	var args []string
	var project harness.Object
	var name string
	var binding harness.Binding
	var err error
	verb := "pull"
	if action == "detached-pull" {
		source, err = c.Content(ctx, kind, "")
		if err != nil {
			return err
		}
		args = []string{"--id", source.ID, "--workspace", c.Workspace, "--no-wait"}
	} else {
		verb = "publish"
		binding, err = contentBinding(c, kind, "")
		if err != nil {
			return err
		}
		project, err = c.Project(ctx, "destination")
		if err != nil {
			return err
		}
		name = c.NamePrefix + "-detached"
		if err = c.Intent(kind, name, project.ID); err != nil {
			return err
		}
		args = []string{"--file", binding.Path, "--project-id", project.ID, "--name", name, "--no-wait"}
		if kind == "datasource" {
			args = append(args, "--create")
		}
	}
	accepted, err := contentRun(ctx, c, kind, verb, args...)
	operationID := contentString(accepted, "operation_id")
	if operationID != "" {
		if journalErr := c.AcceptOperation(operationID); journalErr != nil {
			return errors.Join(err, journalErr)
		}
	}
	if err != nil {
		return err
	}
	if operationID == "" {
		return errors.New("detached acceptance omits local operation_id")
	}
	if contentString(accepted, "check_status") == "" {
		return errors.New("detached acceptance omits returned inspection command")
	}
	var terminal map[string]any
	for {
		out, err := c.Run(ctx, "job", "inspect", "--operation-id", operationID, "--full")
		if err != nil {
			return err
		}
		phase, alive, e := contentOperationTracking(out, operationID)
		if e != nil {
			return e
		}
		if phase == "failed" {
			return errors.New("detached operation reports failed terminal state")
		} else if phase == "completed" && !alive {
			terminal = out
			break
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("detached operation remains unresolved; retain operation %s: %w", operationID, ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
	replay, err := c.Run(ctx, "job", "inspect", "--operation-id", operationID, "--full")
	if err != nil {
		return err
	}
	phase, alive, err := contentOperationTracking(replay, operationID)
	if err != nil {
		return err
	}
	if phase != "completed" || alive {
		return errors.New("completed operation is not durable across fresh CLI processes")
	}
	terminal = replay
	if action == "detached-pull" {
		if contentContainsKey(terminal, "tableau_job_id") {
			return errors.New("local download invents remote Tableau job identity")
		}
		download, err := c.API.Download(ctx, kind, source.ID)
		if err != nil {
			return err
		}
		if _, err = contentVerifyPull(ctx, c, kind, source, terminal, download.Bytes); err != nil {
			return err
		}
		return contentUnchanged(ctx, c, source)
	}
	published, err := contentFindPublished(ctx, c, kind, project.ID, name, terminal)
	if err != nil {
		return err
	}
	if err = contentObserve(ctx, c, func() error {
		download, e := c.API.Download(ctx, kind, published.ID)
		if e != nil {
			return e
		}
		return harness.CheckNative(ctx, kind, download.Bytes, binding)
	}); err != nil {
		return err
	}
	if kind == "flow" {
		if contentContainsKey(terminal, "tableau_job_id") {
			return errors.New("synchronous flow publication invents remote job identity")
		}
		return nil
	}
	item := contentMap(terminal["result"])
	if len(item) == 0 {
		items, _ := terminal["items"].([]any)
		if len(items) != 1 {
			return errors.New("detached asynchronous publication omits accepted remote job result")
		}
		item = contentMap(items[0])
	}
	jobID := contentString(item, "tableau_job_id")
	receipt := contentString(item, "receipt_path")
	if jobID == "" || receipt == "" {
		return errors.New("detached publication omits real job ID or exact accepted receipt")
	}
	if err = c.AcceptJob(jobID); err != nil {
		return err
	}
	waited, err := c.Run(ctx, "job", "wait", "--receipt", receipt)
	if err != nil {
		return err
	}
	oracle, err := c.API.Get(ctx, "job", jobID)
	if err != nil {
		return err
	}
	if !contentJobTerminal(oracle) {
		return errors.New("accepted publication receipt does not recover terminal job")
	}
	if err = contentJobMatches(waited, oracle, oracle); err != nil {
		return err
	}
	if resource := contentString(contentMap(waited["job"]), "resource_id"); resource != "" && resource != published.ID {
		return errors.New("remote job recovery changes published resource identity")
	}
	return nil
}

func contentOperationTracking(out map[string]any, id string) (phase string, alive bool, err error) {
	if contentString(out, "operation_id") != id {
		return "", false, errors.New("fresh-process operation inspect changes local operation identity")
	}
	tracking := contentMap(out["tracking"])
	phase = contentString(tracking, "phase")
	alive, known := tracking["worker_alive"].(bool)
	if phase == "" || !known {
		return "", false, errors.New("full operation inspection omits phase or worker liveness")
	}
	return phase, alive, nil
}

func contentContainsKey(v any, key string) bool {
	switch item := v.(type) {
	case map[string]any:
		for name, value := range item {
			if name == key && value != "" && value != nil {
				return true
			}
			if contentContainsKey(value, key) {
				return true
			}
		}
	case []any:
		for _, value := range item {
			if contentContainsKey(value, key) {
				return true
			}
		}
	}
	return false
}
