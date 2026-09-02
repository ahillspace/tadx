package catalog

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ahillspace/tadx/internal/tableau/catalog/tabxml"
)

const (
	maximumPageSize        = 1000
	maximumConcurrency     = 256
	defaultPageSize        = 1000
	defaultMaxConcurrency  = 32
	defaultInitialLimit    = 4
	defaultMaxRetries      = 4
	defaultMaxResponseSize = 32 * 1024 * 1024
	maximumQueueSize       = 65_536
	maximumRowsPerScope    = 10_000_000
)

// Engine runs fixed Tableau catalog collectors through an injected executor.
type Engine struct {
	executor Executor
	config   Config
}

// NewEngine validates collection bounds and creates an engine.
func NewEngine(executor Executor, config Config) (*Engine, error) {
	if executor == nil {
		return nil, errors.New("catalog request executor is required")
	}
	if config.PageSize == 0 {
		config.PageSize = defaultPageSize
	}
	if config.PageSize < 1 || config.PageSize > maximumPageSize {
		return nil, fmt.Errorf("catalog page size must be between 1 and %d", maximumPageSize)
	}
	if config.MaxConcurrency == 0 {
		config.MaxConcurrency = defaultMaxConcurrency
	}
	if config.MaxConcurrency < 1 || config.MaxConcurrency > maximumConcurrency {
		return nil, fmt.Errorf("catalog maximum concurrency must be between 1 and %d", maximumConcurrency)
	}
	if config.InitialConcurrency == 0 {
		config.InitialConcurrency = min(defaultInitialLimit, config.MaxConcurrency)
	}
	if config.InitialConcurrency < 1 || config.InitialConcurrency > config.MaxConcurrency {
		return nil, errors.New("catalog initial concurrency must be positive and no greater than maximum concurrency")
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = defaultMaxRetries
	}
	if config.MaxRetries < 1 || config.MaxRetries > 20 {
		return nil, errors.New("catalog maximum retries must be between 1 and 20")
	}
	if config.RequestQueueSize == 0 {
		config.RequestQueueSize = min(config.MaxConcurrency*2, maximumQueueSize)
	}
	if config.RequestQueueSize < 1 || config.RequestQueueSize > maximumQueueSize {
		return nil, fmt.Errorf("catalog request queue size must be between 1 and %d", maximumQueueSize)
	}
	if config.BatchQueueSize == 0 {
		config.BatchQueueSize = min(config.MaxConcurrency*2, maximumQueueSize)
	}
	if config.BatchQueueSize < 1 || config.BatchQueueSize > maximumQueueSize {
		return nil, fmt.Errorf("catalog batch queue size must be between 1 and %d", maximumQueueSize)
	}
	if config.MaxBatchRows == 0 {
		config.MaxBatchRows = maximumPageSize
	}
	if config.MaxBatchRows < 1 || config.MaxBatchRows > maximumQueueSize {
		return nil, fmt.Errorf("catalog maximum batch rows must be between 1 and %d", maximumQueueSize)
	}
	if config.MaxResponseBytes == 0 {
		config.MaxResponseBytes = defaultMaxResponseSize
	}
	if config.MaxResponseBytes < 1 || config.MaxResponseBytes > 256*1024*1024 {
		return nil, errors.New("catalog maximum response bytes must be between 1 and 268435456")
	}
	return &Engine{executor: executor, config: config}, nil
}

// Run collects a complete requested scope closure and streams fixed batches.
func (e *Engine) Run(ctx context.Context, input RunRequest, writer BatchWriter) (Result, error) {
	if e == nil || e.executor == nil {
		return Result{}, errors.New("catalog engine is not configured")
	}
	if writer == nil {
		return Result{}, errors.New("catalog batch writer is required")
	}
	plan, err := PlanScopes(input.RequestedScopes)
	if err != nil {
		return Result{}, err
	}
	requested, implicit, collected := plan.Requested, plan.Implicit, plan.Collected
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	pump := newBatchPump(runCtx, cancel, writer, e.config.BatchQueueSize)
	limiter := newAdaptiveLimiter(e.config.InitialConcurrency, e.config.MaxConcurrency)
	state := newRunState(collected)
	runner := runExecutor{engine: e, limiter: limiter, pump: pump, state: state}

	var firstTasks []collectTask
	for _, scope := range collected {
		definition, ok := collectors[scope]
		if !ok {
			continue
		}
		firstTasks = append(firstTasks, collectTask{definition: definition, request: e.listRequest(definition, 1)})
	}
	firstResults, runErr := runner.runTasks(runCtx, firstTasks)
	if runErr == nil {
		var remaining []collectTask
		for _, result := range firstResults {
			pageCount := pageCount(result.page.Total, result.page.Size)
			for pageNumber := 2; pageNumber <= pageCount; pageNumber++ {
				request := e.listRequest(result.task.definition, pageNumber)
				remaining = append(remaining, collectTask{definition: result.task.definition, request: request, baseline: &result.page})
			}
		}
		_, runErr = runner.runTasks(runCtx, remaining)
	}
	if runErr == nil && containsScope(collected, ScopePermissions) {
		workbookIDs := state.identitiesFor(ScopeWorkbooks)
		permissionTasks := make([]collectTask, 0, len(workbookIDs))
		for _, workbookID := range workbookIDs {
			permissionTasks = append(permissionTasks, collectTask{request: e.permissionRequest(workbookID), permission: true})
		}
		_, runErr = runner.runTasks(runCtx, permissionTasks)
	}
	if runErr != nil {
		cancel()
	}
	pumpErr := pump.Close()
	if pumpErr != nil {
		return Result{}, pumpErr
	}
	if runErr != nil {
		return Result{}, runErr
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	return Result{
		RequestedScopes:   requested,
		ImplicitScopes:    implicit,
		Counts:            state.countsCopy(),
		Requests:          state.requests.Load(),
		TableauRequestIDs: state.sortedRequestIDs(),
		FinalConcurrency:  limiter.Limit(),
	}, nil
}

func (e *Engine) listRequest(definition collectorDefinition, pageNumber int) Request {
	query := url.Values{
		"pageNumber": {strconv.Itoa(pageNumber)},
		"pageSize":   {strconv.Itoa(e.config.PageSize)},
	}
	return Request{
		Scope: definition.scope, Path: definition.path, Query: query,
		Operation:  "catalog." + string(definition.scope) + ".list",
		PageNumber: pageNumber, PageSize: e.config.PageSize,
		MaxResponseBytes: e.config.MaxResponseBytes,
	}
}

func (e *Engine) permissionRequest(workbookID string) Request {
	return Request{
		Scope: ScopePermissions, Path: "/workbooks/" + url.PathEscape(workbookID) + "/permissions",
		Operation: "catalog.permissions.get", ItemID: workbookID,
		MaxResponseBytes: e.config.MaxResponseBytes,
	}
}

type collectTask struct {
	definition collectorDefinition
	request    Request
	baseline   *tabxml.Pagination
	permission bool
}

type taskResult struct {
	task collectTask
	page tabxml.Pagination
	err  error
}

type runExecutor struct {
	engine  *Engine
	limiter *adaptiveLimiter
	pump    *batchPump
	state   *runState
}

func (r *runExecutor) runTasks(ctx context.Context, tasks []collectTask) ([]taskResult, error) {
	if len(tasks) == 0 {
		return nil, nil
	}
	phaseCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	workers := min(r.engine.config.MaxConcurrency, len(tasks))
	jobs := make(chan collectTask, min(r.engine.config.RequestQueueSize, len(tasks)))
	results := make(chan taskResult, min(r.engine.config.RequestQueueSize, len(tasks)))
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for {
				select {
				case <-phaseCtx.Done():
					return
				case task, ok := <-jobs:
					if !ok {
						return
					}
					page, err := r.executeTask(phaseCtx, task)
					select {
					case results <- taskResult{task: task, page: page, err: err}:
					case <-phaseCtx.Done():
						return
					}
				}
			}
		}()
	}

	next := 0
	completed := 0
	collected := make([]taskResult, 0, len(tasks))
	for completed < len(tasks) {
		var output chan collectTask
		var task collectTask
		if next < len(tasks) {
			output = jobs
			task = tasks[next]
		}
		select {
		case <-ctx.Done():
			cancel()
			close(jobs)
			wait.Wait()
			return nil, ctx.Err()
		case output <- task:
			next++
		case result := <-results:
			completed++
			if result.err != nil {
				cancel()
				close(jobs)
				wait.Wait()
				return nil, result.err
			}
			collected = append(collected, result)
		}
	}
	close(jobs)
	wait.Wait()
	return collected, nil
}

func (r *runExecutor) executeTask(ctx context.Context, task collectTask) (tabxml.Pagination, error) {
	response, err := r.fetch(ctx, task.request)
	if err != nil {
		return tabxml.Pagination{}, fmt.Errorf("collect %s: %w", task.request.Scope, err)
	}
	if int64(len(response.Body)) > task.request.MaxResponseBytes {
		return tabxml.Pagination{}, newProtocolError(task.request.Operation, response.TableauRequestID, fmt.Errorf("response exceeded %d-byte limit", task.request.MaxResponseBytes))
	}
	r.state.addRequestID(response.TableauRequestID)

	if task.permission {
		rows, identities, err := parsePermissions(task.request.ItemID, response.Body)
		if err != nil {
			return tabxml.Pagination{}, newProtocolError(task.request.Operation, response.TableauRequestID, err)
		}
		if err := r.state.register(ScopePermissions, identities, response.TableauRequestID, task.request.Operation); err != nil {
			return tabxml.Pagination{}, err
		}
		if err := r.emit(ctx, ScopePermissions, rows, response.TableauRequestID); err != nil {
			return tabxml.Pagination{}, err
		}
		return tabxml.Pagination{}, nil
	}

	parsed, err := parseList(task.definition, response.Body)
	if err != nil {
		return tabxml.Pagination{}, newProtocolError(task.request.Operation, response.TableauRequestID, err)
	}
	if err := validatePage(task.request, parsed.page, len(parsed.rows), task.baseline); err != nil {
		return tabxml.Pagination{}, newProtocolError(task.request.Operation, response.TableauRequestID, err)
	}
	if err := r.state.register(task.request.Scope, parsed.identities, response.TableauRequestID, task.request.Operation); err != nil {
		return tabxml.Pagination{}, err
	}
	if err := r.emit(ctx, task.request.Scope, parsed.rows, response.TableauRequestID); err != nil {
		return tabxml.Pagination{}, err
	}
	return parsed.page, nil
}

func (r *runExecutor) emit(ctx context.Context, scope Scope, rows [][]any, requestID string) error {
	columns, ok := ColumnsForScope(scope)
	if !ok {
		return fmt.Errorf("catalog scope %q has no fixed schema", scope)
	}
	for start := 0; start < len(rows); start += r.engine.config.MaxBatchRows {
		end := min(start+r.engine.config.MaxBatchRows, len(rows))
		batch := Batch{Scope: scope, Columns: columns, Rows: rows[start:end], TableauRequestID: requestID}
		if err := r.pump.Emit(ctx, batch); err != nil {
			return fmt.Errorf("write %s catalog batch: %w", scope, err)
		}
	}
	return nil
}

func (r *runExecutor) fetch(ctx context.Context, request Request) (Response, error) {
	var last error
	for attempt := 1; attempt <= r.engine.config.MaxRetries; attempt++ {
		if err := r.limiter.Acquire(ctx); err != nil {
			return Response{}, err
		}
		response, err := r.engine.executor.Do(ctx, request)
		r.state.requests.Add(1)
		if err == nil && (response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices) {
			err = &responseStatusError{status: response.StatusCode, requestID: response.TableauRequestID}
		}
		throttled := statusCode(err) == http.StatusTooManyRequests || statusCode(err) == http.StatusServiceUnavailable
		r.limiter.Release(err == nil, throttled)
		if err == nil {
			return response, nil
		}
		last = err
		if ctx.Err() != nil {
			return Response{}, ctx.Err()
		}
		if !isRetryable(err) || attempt == r.engine.config.MaxRetries {
			return Response{}, err
		}
		delay := retryDelay(err, attempt)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return Response{}, ctx.Err()
		case <-timer.C:
		}
	}
	return Response{}, last
}

func validatePage(request Request, page tabxml.Pagination, itemCount int, baseline *tabxml.Pagination) error {
	if page.Number != request.PageNumber || page.Number < 1 {
		return fmt.Errorf("response returned page number %d; expected %d", page.Number, request.PageNumber)
	}
	if page.Size < 1 || page.Size > request.PageSize || page.Size > maximumPageSize {
		return fmt.Errorf("response returned invalid page size %d", page.Size)
	}
	if page.Total < 0 || page.Total > maximumRowsPerScope {
		return fmt.Errorf("response returned invalid totalAvailable %d", page.Total)
	}
	if baseline != nil && (page.Size != baseline.Size || page.Total != baseline.Total) {
		return fmt.Errorf("response changed pagination from size %d and total %d to size %d and total %d", baseline.Size, baseline.Total, page.Size, page.Total)
	}
	pageIndex := int64(page.Number - 1)
	if pageIndex > math.MaxInt64/int64(page.Size) {
		return errors.New("response page offset exceeded the pagination bound")
	}
	offset := pageIndex * int64(page.Size)
	remaining := int64(page.Total) - offset
	if remaining < 0 {
		return fmt.Errorf("response total %d is inconsistent with page %d", page.Total, page.Number)
	}
	expected := min(int64(page.Size), remaining)
	if int64(itemCount) != expected {
		return fmt.Errorf("response returned %d items for page %d; expected %d from total %d and size %d", itemCount, page.Number, expected, page.Total, page.Size)
	}
	return nil
}

func pageCount(total, size int) int {
	if total == 0 {
		return 1
	}
	return (total + size - 1) / size
}

// PlanScopes returns a stable dependency closure without running requests.
func PlanScopes(input []Scope) (ScopePlan, error) {
	requestedSet := make(map[Scope]struct{})
	if len(input) == 0 {
		input = canonicalScopes
	}
	for _, scope := range input {
		if _, ok := fixedColumns[scope]; !ok {
			return ScopePlan{}, fmt.Errorf("unsupported catalog scope %q", scope)
		}
		requestedSet[scope] = struct{}{}
	}
	collectedSet := make(map[Scope]struct{}, len(requestedSet))
	var add func(Scope)
	add = func(scope Scope) {
		if _, exists := collectedSet[scope]; exists {
			return
		}
		for _, dependency := range dependencies(scope) {
			add(dependency)
		}
		collectedSet[scope] = struct{}{}
	}
	for scope := range requestedSet {
		add(scope)
	}
	requested := scopeSetSlice(requestedSet)
	collected := scopeSetSlice(collectedSet)
	var implicit []Scope
	for _, scope := range collected {
		if _, requested := requestedSet[scope]; !requested {
			implicit = append(implicit, scope)
		}
	}
	return ScopePlan{Requested: requested, Implicit: implicit, Collected: collected}, nil
}

func dependencies(scope Scope) []Scope {
	switch scope {
	case ScopeWorkbooks, ScopeDatasources, ScopeFlows:
		return []Scope{ScopeProjects}
	case ScopeViews:
		return []Scope{ScopeWorkbooks}
	case ScopePermissions:
		return []Scope{ScopeWorkbooks}
	default:
		return nil
	}
}

func scopeSetSlice(set map[Scope]struct{}) []Scope {
	result := make([]Scope, 0, len(set))
	for scope := range set {
		result = append(result, scope)
	}
	sortScopes(result)
	return result
}

func containsScope(scopes []Scope, target Scope) bool {
	for _, scope := range scopes {
		if scope == target {
			return true
		}
	}
	return false
}

type runState struct {
	mu         sync.Mutex
	seen       map[Scope]map[string]struct{}
	counts     map[Scope]int64
	requestIDs map[string]struct{}
	requests   atomic.Int64
}

func newRunState(scopes []Scope) *runState {
	state := &runState{seen: make(map[Scope]map[string]struct{}), counts: make(map[Scope]int64), requestIDs: make(map[string]struct{})}
	for _, scope := range scopes {
		state.counts[scope] = 0
	}
	return state
}

func (s *runState) register(scope Scope, identities []string, requestID, operation string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen[scope] == nil {
		s.seen[scope] = make(map[string]struct{})
	}
	for _, identity := range identities {
		if strings.TrimSpace(identity) == "" {
			return newProtocolError(operation, requestID, fmt.Errorf("%s response returned an empty authoritative identity", scope))
		}
		if _, exists := s.seen[scope][identity]; exists {
			return newProtocolError(operation, requestID, fmt.Errorf("duplicate %s identity %q", scope, printableIdentity(identity)))
		}
		s.seen[scope][identity] = struct{}{}
		s.counts[scope]++
	}
	return nil
}

func (s *runState) identitiesFor(scope Scope) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]string, 0, len(s.seen[scope]))
	for identity := range s.seen[scope] {
		result = append(result, identity)
	}
	sort.Strings(result)
	return result
}

func (s *runState) countsCopy() map[Scope]int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[Scope]int64, len(s.counts))
	for scope, count := range s.counts {
		result[scope] = count
	}
	return result
}

func (s *runState) addRequestID(requestID string) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return
	}
	s.mu.Lock()
	s.requestIDs[requestID] = struct{}{}
	s.mu.Unlock()
}

func (s *runState) sortedRequestIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]string, 0, len(s.requestIDs))
	for requestID := range s.requestIDs {
		result = append(result, requestID)
	}
	sort.Strings(result)
	return result
}

func printableIdentity(identity string) string {
	if index := strings.IndexByte(identity, 0); index >= 0 {
		return strings.ReplaceAll(identity, "\x00", "/")
	}
	return identity
}

type protocolError struct {
	operation string
	requestID string
	cause     error
}

func newProtocolError(operation, requestID string, cause error) *protocolError {
	return &protocolError{operation: operation, requestID: requestID, cause: cause}
}

func (e *protocolError) Error() string {
	return fmt.Sprintf("invalid Tableau %s response: %v", e.operation, e.cause)
}
func (e *protocolError) Unwrap() error     { return e.cause }
func (e *protocolError) RequestID() string { return e.requestID }
func (e *protocolError) HTTPStatus() int   { return http.StatusOK }
func (e *protocolError) TableauCode() string {
	return ""
}
func (e *protocolError) TableauSummary() string {
	return ""
}
func (e *protocolError) TableauDetail() string {
	return ""
}
func (e *protocolError) Retryable() bool { return true }
func (e *protocolError) CorrectiveAction() string {
	return "Retry after Tableau returns a complete valid response."
}

type responseStatusError struct {
	status    int
	requestID string
}

func (e *responseStatusError) Error() string {
	return fmt.Sprintf("Tableau catalog request returned HTTP %d", e.status)
}
func (e *responseStatusError) RequestID() string { return e.requestID }
func (e *responseStatusError) HTTPStatus() int   { return e.status }
func (e *responseStatusError) TableauCode() string {
	return ""
}
func (e *responseStatusError) TableauSummary() string {
	return ""
}
func (e *responseStatusError) TableauDetail() string {
	return ""
}
func (e *responseStatusError) Retryable() bool {
	return e.status == http.StatusRequestTimeout || e.status == http.StatusTooEarly || e.status == http.StatusTooManyRequests || e.status >= http.StatusInternalServerError
}
func (e *responseStatusError) CorrectiveAction() string {
	if e.Retryable() {
		return "Retry after Tableau or the intermediary service recovers."
	}
	return "Review the upstream status and Tableau permissions before retrying."
}

func statusCode(err error) int {
	var carrier interface{ HTTPStatus() int }
	if errors.As(err, &carrier) {
		return carrier.HTTPStatus()
	}
	return 0
}

func isRetryable(err error) bool {
	var carrier interface{ Retryable() bool }
	return errors.As(err, &carrier) && carrier.Retryable()
}

func retryDelay(err error, attempt int) time.Duration {
	var carrier interface{ RetryAfter() (time.Duration, bool) }
	if errors.As(err, &carrier) {
		if delay, ok := carrier.RetryAfter(); ok && delay >= 0 {
			return min(delay, 30*time.Second)
		}
	}
	shift := min(attempt-1, 6)
	return min(time.Duration(1<<shift)*50*time.Millisecond, 2*time.Second)
}
