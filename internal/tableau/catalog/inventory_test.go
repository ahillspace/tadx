package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCollectInventoryReturnsCompleteDeterministicSnapshot(t *testing.T) {
	var firstFinished atomic.Bool
	var remainingStarted atomic.Int32
	var earlyRemainingPage atomic.Bool
	executor := executorFunc(func(ctx context.Context, request Request) (Response, error) {
		if request.Scope == ScopeProjects {
			return Response{StatusCode: 200, Body: []byte(listXML("projects", "project", 1, 1, 1, `<project id="project-1" name="Operations"/>`)), TableauRequestID: "request-projects"}, nil
		}
		if request.PageNumber > 1 && !firstFinished.Load() {
			earlyRemainingPage.Store(true)
		}
		if request.PageNumber == 1 {
			firstFinished.Store(true)
		} else {
			remainingStarted.Add(1)
		}
		if request.PageNumber == 2 {
			select {
			case <-time.After(20 * time.Millisecond):
			case <-ctx.Done():
				return Response{}, ctx.Err()
			}
		}
		ids := map[int]string{1: "workbook-b", 2: "workbook-c", 3: "workbook-a"}
		id := ids[request.PageNumber]
		item := fmt.Sprintf(`<workbook id="%s" name="%s"><project id="project-1"/></workbook>`, id, id)
		return Response{StatusCode: 200, Body: []byte(listXML("workbooks", "workbook", request.PageNumber, 1, 3, item)), TableauRequestID: fmt.Sprintf("request-workbooks-%d", request.PageNumber)}, nil
	})
	engine, err := NewEngine(executor, Config{PageSize: 1, InitialConcurrency: 2, MaxConcurrency: 2})
	if err != nil {
		t.Fatal(err)
	}

	snapshot, err := engine.CollectInventory(context.Background(), ScopeWorkbooks)
	if err != nil {
		t.Fatal(err)
	}
	if earlyRemainingPage.Load() || remainingStarted.Load() != 2 || snapshot.Scope != ScopeWorkbooks || snapshot.Total != 3 || snapshot.Requests != 4 {
		t.Fatalf("snapshot = %#v; remaining pages = %d", snapshot, remainingStarted.Load())
	}
	if got := fmt.Sprint(snapshot.TableauRequestIDs); got != "[request-projects request-workbooks-1 request-workbooks-2 request-workbooks-3]" {
		t.Fatalf("request evidence = %s", got)
	}
	if len(snapshot.Columns) != 7 || snapshot.Columns[0].Name != "id" || snapshot.Columns[6].Name != "list_payload" {
		t.Fatalf("columns = %#v", snapshot.Columns)
	}
	if len(snapshot.Dependencies) != 1 || snapshot.Dependencies[0].Scope != ScopeProjects || len(snapshot.Dependencies[0].Rows) != 1 || snapshot.Dependencies[0].Rows[0][0] != "project-1" {
		t.Fatalf("dependency tables = %#v", snapshot.Dependencies)
	}
	if got := fmt.Sprint([]any{snapshot.Rows[0][0], snapshot.Rows[1][0], snapshot.Rows[2][0]}); got != "[workbook-a workbook-b workbook-c]" {
		t.Fatalf("row order = %s", got)
	}
}

func TestCollectInventoryFailsClosedAndCancelsRemainingPages(t *testing.T) {
	pageThreeStarted := make(chan struct{})
	pageThreeCanceled := make(chan struct{})
	executor := executorFunc(func(ctx context.Context, request Request) (Response, error) {
		switch request.PageNumber {
		case 1:
			return Response{StatusCode: 200, Body: []byte(listXML("users", "user", 1, 1, 3, `<user id="user-1" name="One"/>`)), TableauRequestID: "page-1"}, nil
		case 2:
			<-pageThreeStarted
			return Response{}, errors.New("page two failed")
		case 3:
			close(pageThreeStarted)
			<-ctx.Done()
			close(pageThreeCanceled)
			return Response{}, ctx.Err()
		default:
			return Response{}, errors.New("unexpected page")
		}
	})
	engine, _ := NewEngine(executor, Config{PageSize: 1, InitialConcurrency: 2, MaxConcurrency: 2, MaxRetries: 1})

	snapshot, err := engine.CollectInventory(context.Background(), ScopeUsers)
	if err == nil || !strings.Contains(err.Error(), "page two failed") {
		t.Fatalf("error = %v", err)
	}
	if snapshot.Rows != nil || snapshot.Total != 0 || snapshot.Requests != 0 {
		t.Fatalf("partial snapshot escaped: %#v", snapshot)
	}
	select {
	case <-pageThreeCanceled:
	case <-time.After(time.Second):
		t.Fatal("remaining request was not canceled")
	}
}

func TestCollectInventoryRejectsMissingAndDuplicateLUIDs(t *testing.T) {
	for _, test := range []struct {
		name  string
		total int
		item  string
		want  string
	}{
		{name: "missing", total: 1, item: `<group name="One"/>`, want: "incomplete authoritative identity"},
		{name: "duplicate", total: 2, item: `<group id="group-1" name="One"/><group id="group-1" name="Two"/>`, want: "duplicate groups identity"},
	} {
		t.Run(test.name, func(t *testing.T) {
			executor := executorFunc(func(context.Context, Request) (Response, error) {
				return Response{StatusCode: 200, Body: []byte(listXML("groups", "group", 1, 2, test.total, test.item)), TableauRequestID: "invalid-page"}, nil
			})
			engine, _ := NewEngine(executor, Config{PageSize: 2})
			if _, err := engine.CollectInventory(context.Background(), ScopeGroups); err == nil || !strings.Contains(err.Error(), test.want) || requestID(err) != "invalid-page" {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestCollectInventoryRejectsNonResourceScope(t *testing.T) {
	engine, _ := NewEngine(executorFunc(func(context.Context, Request) (Response, error) {
		return Response{}, errors.New("must not execute")
	}), Config{})
	if _, err := engine.CollectInventory(context.Background(), ScopePermissions); err == nil {
		t.Fatal("permission rows were accepted as a resource inventory")
	}
}
