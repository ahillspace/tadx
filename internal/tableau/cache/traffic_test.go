package cache

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ahillspace/tadx/internal/tableau"
)

type cooldownError struct{ delay time.Duration }

func (e cooldownError) Error() string                     { return "temporarily unavailable" }
func (e cooldownError) HTTPStatus() int                   { return 429 }
func (e cooldownError) Retryable() bool                   { return true }
func (e cooldownError) RetryAfter() (time.Duration, bool) { return e.delay, true }

func TestDefaultRunSkipsPermissionsAndExplicitScopeReusesInventory(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(fmt.Sprint(explicit), func(t *testing.T) {
			var permissions, workbooks atomic.Int32
			engine, err := NewEngine(executorFunc(func(_ context.Context, req Request) (Response, error) {
				if req.Scope == ScopePermissions {
					permissions.Add(1)
					return Response{StatusCode: 200, Body: []byte(`<tsResponse><permissions><workbook id="w1"/></permissions></tsResponse>`)}, nil
				}
				def := collectors[req.Scope]
				items, count := "", 0
				if req.Scope == ScopeWorkbooks {
					workbooks.Add(1)
					items, count = `<workbook id="w1" name="Workbook"/>`, 1
				}
				return Response{StatusCode: 200, Body: []byte(listXML(def.container, def.item, 1, 1000, count, items))}, nil
			}), Config{})
			if err != nil {
				t.Fatal(err)
			}
			var scopes []Scope
			if explicit {
				scopes = []Scope{ScopePermissions}
			}
			result, err := engine.Run(context.Background(), RunRequest{RequestedScopes: scopes}, &memoryWriter{})
			if err != nil {
				t.Fatal(err)
			}
			want := int32(0)
			if explicit {
				want = 1
			}
			if permissions.Load() != want || workbooks.Load() != 1 {
				t.Fatalf("permission calls=%d, workbook calls=%d", permissions.Load(), workbooks.Load())
			}
			if containsScope(result.RequestedScopes, ScopePermissions) != explicit {
				t.Fatalf("requested=%v", result.RequestedScopes)
			}
		})
	}
}

func TestRunHonorsFullSharedCooldown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		var calls atomic.Int32
		engine, _ := NewEngine(executorFunc(func(_ context.Context, req Request) (Response, error) {
			if calls.Add(1) == 1 {
				return Response{}, cooldownError{2 * time.Minute}
			}
			if time.Since(start) < 2*time.Minute {
				t.Errorf("request %s escaped cooldown after %s", req.Scope, time.Since(start))
			}
			def := collectors[req.Scope]
			return Response{StatusCode: 200, Body: []byte(listXML(def.container, def.item, 1, 1000, 0, ""))}, nil
		}), Config{InitialConcurrency: 1, MaxConcurrency: 4})
		if _, err := engine.Run(context.Background(), RunRequest{}, &memoryWriter{}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestRunCooldownCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var calls atomic.Int32
		engine, _ := NewEngine(executorFunc(func(context.Context, Request) (Response, error) {
			calls.Add(1)
			return Response{}, cooldownError{time.Hour}
		}), Config{InitialConcurrency: 1, MaxConcurrency: 4})
		done := make(chan error, 1)
		go func() { _, err := engine.Run(ctx, RunRequest{}, &memoryWriter{}); done <- err }()
		synctest.Wait()
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("error=%v", err)
		}
		if calls.Load() != 1 {
			t.Fatalf("requests during cooldown=%d", calls.Load())
		}
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestTransportRetryAfterSecondsAndDateReachSharedCooldown(t *testing.T) {
	for _, format := range []string{"seconds", "date"} {
		t.Run(format, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				start := time.Now()
				var calls atomic.Int32
				client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					status, header, body := 200, make(http.Header), listXML("users", "user", 1, 1000, 0, "")
					if calls.Add(1) == 1 {
						status, body = 503, `<tsResponse><error code="503000"><summary>Busy</summary></error></tsResponse>`
						value := "120"
						if format == "date" {
							value = start.Add(2 * time.Minute).UTC().Format(http.TimeFormat)
						}
						header.Set("Retry-After", value)
					} else if time.Since(start) < 2*time.Minute {
						t.Errorf("retry after %s", time.Since(start))
					}
					return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
				})}
				transport := tableau.NewTransport(client, "3.29", nil)
				engine, _ := NewEngine(executorFunc(func(ctx context.Context, request Request) (Response, error) {
					response, err := transport.Do(ctx, nil, tableau.Request{Method: http.MethodGet, ServerURL: "https://tableau.example.com", Path: request.Path, Query: request.Query, Operation: request.Operation})
					return Response{StatusCode: response.StatusCode, Body: response.Body}, err
				}), Config{})
				if _, err := engine.Run(context.Background(), RunRequest{RequestedScopes: []Scope{ScopeUsers}}, &memoryWriter{}); err != nil {
					t.Fatal(err)
				}
				if calls.Load() != 2 {
					t.Fatalf("calls=%d", calls.Load())
				}
			})
		})
	}
}

func TestRunLaterInflightResponseExtendsCooldownAndCancellation(t *testing.T) {
	for _, cancelRun := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelRun), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				start := time.Now()
				var calls atomic.Int32
				firstPair := make(chan struct{})
				engine, _ := NewEngine(executorFunc(func(_ context.Context, req Request) (Response, error) {
					switch calls.Add(1) {
					case 1:
						<-firstPair
						return Response{}, cooldownError{time.Minute}
					case 2:
						close(firstPair)
						time.Sleep(10 * time.Second)
						return Response{}, cooldownError{2 * time.Minute}
					}
					if time.Since(start) < 130*time.Second {
						t.Errorf("escaped extended delay at %s", time.Since(start))
					}
					def := collectors[req.Scope]
					return Response{StatusCode: 200, Body: []byte(listXML(def.container, def.item, 1, 1000, 0, ""))}, nil
				}), Config{InitialConcurrency: 2, MaxConcurrency: 4})
				done := make(chan error, 1)
				go func() { _, err := engine.Run(ctx, RunRequest{}, &memoryWriter{}); done <- err }()
				if cancelRun {
					time.Sleep(11 * time.Second)
					synctest.Wait()
					cancel()
					if err := <-done; !errors.Is(err, context.Canceled) {
						t.Fatal(err)
					}
					if calls.Load() != 2 {
						t.Fatalf("requests=%d", calls.Load())
					}
				} else if err := <-done; err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}

func TestRunConcurrencyRampsToConfiguredCeiling(t *testing.T) {
	for _, ceiling := range []int{1, 3, 8, 32} {
		t.Run(fmt.Sprint(ceiling), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var active, maximum atomic.Int32
				engine, _ := NewEngine(executorFunc(func(_ context.Context, request Request) (Response, error) {
					current := active.Add(1)
					defer active.Add(-1)
					for old := maximum.Load(); current > old; old = maximum.Load() {
						if maximum.CompareAndSwap(old, current) {
							break
						}
					}
					time.Sleep(time.Second)
					return Response{StatusCode: 200, Body: []byte(listXML("users", "user", request.PageNumber, 1, 1100, fmt.Sprintf(`<user id="u%d" name="User"/>`, request.PageNumber)))}, nil
				}), Config{PageSize: 1, MaxConcurrency: ceiling})
				if engine.config.InitialConcurrency != min(4, ceiling) {
					t.Fatalf("initial=%d", engine.config.InitialConcurrency)
				}
				result, err := engine.Run(context.Background(), RunRequest{RequestedScopes: []Scope{ScopeUsers}}, &memoryWriter{})
				if err != nil {
					t.Fatal(err)
				}
				if maximum.Load() != int32(ceiling) || result.FinalConcurrency != ceiling {
					t.Fatalf("maximum=%d final=%d ceiling=%d", maximum.Load(), result.FinalConcurrency, ceiling)
				}
			})
		})
	}
}

func TestReadRetryBackoffIsBoundedAndJittered(t *testing.T) {
	seen := map[time.Duration]bool{}
	for attempt := 1; attempt <= 20; attempt++ {
		ceiling := min(time.Second*time.Duration(1<<min(attempt-1, 5)), 30*time.Second)
		for sample := 0; sample < 10; sample++ {
			delay := retryDelay(errors.New("no server delay"), attempt)
			if delay < ceiling/2 || delay > ceiling {
				t.Fatalf("attempt%d delay=%s", attempt, delay)
			}
			if attempt == 1 {
				seen[delay] = true
			}
		}
	}
	if len(seen) < 2 {
		t.Fatal("backoff lacks jitter")
	}
}

func TestExplicitPermissionDenialPreservesInventoryWithoutRetry(t *testing.T) {
	var calls atomic.Int32
	engine, _ := NewEngine(executorFunc(func(_ context.Context, req Request) (Response, error) {
		if req.Scope == ScopePermissions {
			calls.Add(1)
			return Response{StatusCode: http.StatusForbidden, TableauRequestID: "permission-denied"}, nil
		}
		def := collectors[req.Scope]
		items, count := "", 0
		if req.Scope == ScopeWorkbooks {
			items, count = `<workbook id="w1" name="Workbook"/>`, 1
		}
		return Response{StatusCode: 200, Body: []byte(listXML(def.container, def.item, 1, 1000, count, items))}, nil
	}), Config{})
	writer := &memoryWriter{}
	result, err := engine.Run(context.Background(), RunRequest{RequestedScopes: []Scope{ScopePermissions}}, writer)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || result.DeniedPermissions != 1 || len(writer.rows(ScopeWorkbooks)) != 1 {
		t.Fatalf("calls=%d result=%+v", calls.Load(), result)
	}
	if fmt.Sprint(result.TableauRequestIDs) != "[permission-denied]" {
		t.Fatalf("request IDs=%v", result.TableauRequestIDs)
	}
}

func TestEngineConcurrencyBoundsAndDefaults(t *testing.T) {
	for _, limit := range []int{-1, 0, 1, 256, 257} {
		engine, err := NewEngine(executorFunc(nil), Config{MaxConcurrency: limit})
		if limit < 0 || limit > 256 {
			if err == nil {
				t.Fatalf("accepted max%d", limit)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		want := limit
		if want == 0 {
			want = 32
		}
		if engine.config.MaxConcurrency != want || engine.config.InitialConcurrency != min(4, want) {
			t.Fatalf("config=%+v", engine.config)
		}
	}
}
