//go:build workflowevidence

// This opt-in test emits deterministic source/evidence packets for a named
// workflow. It never calls an LLM and never runs by default. Use
// TADX_WORKFLOW=workbook.pull for the bounded hand-traced workflow or
// TADX_WORKFLOW=all for the capability-registry catalog.
package architecture_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/capability"
)

const workflowEvidenceModule = "github.com/ahillspace/tadx"

type workflowSelection struct {
	roles   map[string]map[string]struct{}
	reasons map[string]map[string]struct{}
}

func newWorkflowSelection() workflowSelection {
	return workflowSelection{
		roles:   make(map[string]map[string]struct{}),
		reasons: make(map[string]map[string]struct{}),
	}
}

func (s workflowSelection) add(path, role, reason string) {
	path = filepath.ToSlash(filepath.Clean(path))
	if path == "." || strings.HasPrefix(path, "../") || filepath.IsAbs(path) {
		return
	}
	if s.roles[path] == nil {
		s.roles[path] = make(map[string]struct{})
	}
	if s.reasons[path] == nil {
		s.reasons[path] = make(map[string]struct{})
	}
	if role != "" {
		s.roles[path][role] = struct{}{}
	}
	if reason != "" {
		s.reasons[path][reason] = struct{}{}
	}
}

type workflowEvidence struct {
	SchemaVersion  string                  `json:"schema_version"`
	Workflow       string                  `json:"workflow"`
	Summary        string                  `json:"summary"`
	GeneratedBy    string                  `json:"generated_by"`
	SelectionRules []string                `json:"selection_rules"`
	EvidenceRules  []string                `json:"evidence_rules"`
	Trace          []workflowTraceEdge     `json:"trace"`
	Files          []workflowFileEvidence  `json:"files"`
	PackageImports []workflowPackageImport `json:"package_imports"`
	Diagnostics    []string                `json:"diagnostics,omitempty"`
	Limitations    []string                `json:"limitations"`
}

type workflowCatalogEvidence struct {
	SchemaVersion  string                  `json:"schema_version"`
	GeneratedBy    string                  `json:"generated_by"`
	Workflows      []workflowCatalogItem   `json:"workflows"`
	Files          []workflowFileEvidence  `json:"files"`
	TextEvidence   []workflowTextEvidence  `json:"text_evidence,omitempty"`
	PackageImports []workflowPackageImport `json:"package_imports"`
	SelectionRules []string                `json:"selection_rules"`
	EvidenceRules  []string                `json:"evidence_rules"`
	Diagnostics    []string                `json:"diagnostics,omitempty"`
	Limitations    []string                `json:"limitations"`
}

type workflowCatalogItem struct {
	Workflow        string                 `json:"workflow"`
	Surface         string                 `json:"surface"`
	Outcome         string                 `json:"outcome"`
	Type            string                 `json:"type"`
	Disposition     string                 `json:"disposition"`
	Owner           string                 `json:"owner"`
	Implementation  string                 `json:"implementation"`
	Verification    string                 `json:"verification"`
	EvidenceLevel   string                 `json:"evidence_level"`
	Blocker         string                 `json:"blocker,omitempty"`
	CommandPath     []string               `json:"command_path,omitempty"`
	Selectors       string                 `json:"selectors"`
	Availability    string                 `json:"availability"`
	SafetyGuard     string                 `json:"safety_guard"`
	ArtifactEffect  string                 `json:"artifact_effect"`
	Upstream        string                 `json:"upstream_operation"`
	EvidencePaths   []workflowEvidencePath `json:"evidence_paths"`
	Files           []string               `json:"files"`
	LocalWrite      bool                   `json:"local_write"`
	RemoteMutation  bool                   `json:"remote_mutation"`
	SupportsPreview bool                   `json:"supports_preview"`
	SupportsBatch   bool                   `json:"supports_batch"`
	Administrative  bool                   `json:"administrative"`
}

type workflowEvidencePath struct {
	Reference string `json:"reference"`
	FilePath  string `json:"file_path,omitempty"`
	Kind      string `json:"kind"`
	Exists    bool   `json:"exists"`
}

type workflowTextEvidence struct {
	Path   string `json:"path"`
	Bytes  int    `json:"bytes"`
	Lines  int    `json:"physical_lines"`
	SHA256 string `json:"sha256"`
	Source string `json:"source"`
}

type workflowTraceEdge struct {
	From         string `json:"from"`
	To           string `json:"to"`
	Relationship string `json:"relationship"`
	Evidence     string `json:"evidence"`
}

type workflowPackageImport struct {
	FromFile      string   `json:"from_file"`
	Imported      string   `json:"imported"`
	ResolvedFiles []string `json:"resolved_files,omitempty"`
	Evidence      string   `json:"evidence"`
}

type workflowFileEvidence struct {
	Path                string   `json:"path"`
	Package             string   `json:"package"`
	Roles               []string `json:"roles"`
	SelectionReasons    []string `json:"selection_reasons"`
	Kind                string   `json:"kind"`
	Lines               int      `json:"physical_lines"`
	NonBlankLines       int      `json:"nonblank_lines"`
	Bytes               int      `json:"bytes"`
	SHA256              string   `json:"sha256"`
	BuildConstraints    []string `json:"build_constraints,omitempty"`
	GeneratedDirectives []string `json:"generated_directives,omitempty"`
	Imports             []string `json:"imports"`
	LocalImports        []string `json:"local_imports,omitempty"`
	ImportedBy          []string `json:"imported_by,omitempty"`
	SamePackageFiles    []string `json:"same_package_files,omitempty"`
	CompiledTargets     []string `json:"compiled_targets,omitempty"`
	Source              string   `json:"source"`
}

type goListEvidence struct {
	ImportPath      string
	Dir             string
	GoFiles         []string
	CgoFiles        []string
	CompiledGoFiles []string
	Error           *struct {
		Err string
	} `json:"Error"`
}

func TestWorkflowEvidence(t *testing.T) {
	root := workflowEvidenceRoot(t)
	workflow := strings.TrimSpace(os.Getenv("TADX_WORKFLOW"))
	if workflow == "" {
		workflow = "workbook.pull"
	}
	if workflow == "all" {
		writeWorkflowCatalogEvidence(t, root)
		return
	}
	if workflow != "workbook.pull" {
		t.Fatalf("unsupported workflow %q; use workbook.pull or all", workflow)
	}

	selection, trace, summary, selectionRules, limitations := workbookPullSelection(t, root)
	allGoFiles := allRepositoryGoFiles(t, root)
	packageFiles, packagePaths := packageIndexes(t, root, allGoFiles)
	packageConsumers, packageImportEdges := importIndexes(t, root, allGoFiles, packagePaths)
	compiledTargets, diagnostics := compiledTargetIndex(t, root)

	files := collectWorkflowFileEvidence(t, root, selection, packageFiles, packagePaths, packageConsumers, compiledTargets)

	evidence := workflowEvidence{
		SchemaVersion:  "1",
		Workflow:       workflow,
		Summary:        summary,
		GeneratedBy:    "go test -tags=workflowevidence ./internal/architecture -run TestWorkflowEvidence",
		SelectionRules: selectionRules,
		EvidenceRules: []string{
			"Imports and imported_by are direct Go package-import evidence, not proof of runtime execution.",
			"same_package_files records files compiled as one Go package; it is not a symbol-level consumer claim.",
			"compiled_targets records go list evidence for cmd/tadx dependencies on windows, linux, and darwin.",
			"An empty imported_by array means no direct package importer was identified by this scan, not that the file is unused.",
			"The generator does not assign essentiality, deletion safety, or behavioral equivalence.",
		},
		Trace:          trace,
		Files:          files,
		PackageImports: packageImportEdges,
		Diagnostics:    diagnostics,
		Limitations:    limitations,
	}

	outputDir := evidenceOutputDir(root)
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		t.Fatalf("create workflow evidence directory: %v", err)
	}
	jsonPath := filepath.Join(outputDir, "workbook-pull.json")
	data, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		t.Fatalf("encode workflow evidence: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(jsonPath, data, 0o600); err != nil {
		t.Fatalf("write workflow evidence %s: %v", jsonPath, err)
	}
	contextPath := filepath.Join(outputDir, "workbook-pull-context.md")
	if err := os.WriteFile(contextPath, []byte(renderWorkflowContext(evidence)), 0o600); err != nil {
		t.Fatalf("write workflow context %s: %v", contextPath, err)
	}

	t.Logf("workflow=%s files=%d production=%d tests=%d physical_lines=%d nonblank_lines=%d", workflow, len(files), countFiles(files, "production"), countFiles(files, "test"), totalLines(files, false), totalLines(files, true))
	t.Logf("evidence JSON: %s", evidenceDisplayPath(root, jsonPath))
	t.Logf("LLM context: %s", evidenceDisplayPath(root, contextPath))
}

func collectWorkflowFileEvidence(t *testing.T, root string, selection workflowSelection, packageFiles map[string][]string, packagePaths map[string]string, packageConsumers map[string][]string, compiledTargets map[string][]string) []workflowFileEvidence {
	t.Helper()
	paths := make([]string, 0, len(selection.roles))
	for path := range selection.roles {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	files := make([]workflowFileEvidence, 0, len(paths))
	for _, path := range paths {
		absolute := filepath.Join(root, filepath.FromSlash(path))
		data, err := os.ReadFile(absolute)
		if err != nil {
			t.Fatalf("read selected workflow file %s: %v", path, err)
		}
		packagePath := packagePaths[path]
		if packagePath == "" {
			packagePath = packageImportPath(filepath.Dir(path))
		}
		kind := "production"
		if strings.HasSuffix(path, "_test.go") {
			kind = "test"
		}
		imports, localImports := fileImports(t, absolute)
		files = append(files, workflowFileEvidence{
			Path:                path,
			Package:             packagePath,
			Roles:               sortedSet(selection.roles[path]),
			SelectionReasons:    sortedSet(selection.reasons[path]),
			Kind:                kind,
			Lines:               physicalLines(data),
			NonBlankLines:       nonBlankLines(data),
			Bytes:               len(data),
			SHA256:              sha256Hex(data),
			BuildConstraints:    buildConstraints(data),
			GeneratedDirectives: generatedDirectives(data),
			Imports:             imports,
			LocalImports:        localImports,
			ImportedBy:          packageConsumers[packagePath],
			SamePackageFiles:    packageFiles[packagePath],
			CompiledTargets:     compiledTargets[path],
			Source:              string(data),
		})
	}
	return files
}

func workflowEvidenceRoot(t *testing.T) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("get repository root: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return root
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatal("repository root with go.mod not found")
		}
		root = parent
	}
}

func evidenceOutputDir(root string) string {
	if value := strings.TrimSpace(os.Getenv("TADX_WORKFLOW_EVIDENCE_DIR")); value != "" {
		if filepath.IsAbs(value) {
			return value
		}
		return filepath.Join(root, value)
	}
	return filepath.Join(root, "workflow-evidence")
}

func relativeRootLabel(root string) string {
	if value := strings.TrimSpace(os.Getenv("TADX_WORKFLOW_EVIDENCE_DIR")); value != "" && filepath.IsAbs(value) {
		return "<configured-output>"
	}
	return "."
}

func evidenceDisplayPath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(filepath.Join(relativeRootLabel(root), relative))
}

func workbookPullSelection(t *testing.T, root string) (workflowSelection, []workflowTraceEdge, string, []string, []string) {
	t.Helper()
	selection := newWorkflowSelection()
	add := func(role, reason string, paths ...string) {
		for _, path := range paths {
			selection.add(path, role, reason)
		}
	}
	addDir := func(role, reason, directory string, includeTests bool) {
		entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(directory)))
		if err != nil {
			t.Fatalf("read workflow directory %s: %v", directory, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
				continue
			}
			if strings.HasSuffix(entry.Name(), "_test.go") != includeTests {
				continue
			}
			add(role, reason, filepath.ToSlash(filepath.Join(directory, entry.Name())))
		}
	}

	add("entrypoint", "the executable delegates command execution to app.Run", "cmd/tadx/main.go")
	add("composition root", "constructs the runtime, wires workbook.pull, and adapts the action to Tableau and artifact services", "internal/app/app.go")
	add("composition root", "constructs authenticated clients, workspace caching, and command-scoped sessions", "internal/app/command_runtime.go")
	add("workspace setup", "resolves the logical workspace before remote setup", "internal/app/workspace.go")
	add("local preview", "implements workbook.pull acquisition preview without writes", "internal/app/acquisition_preview.go")
	add("result persistence", "captures and saves the selected command result and recovery hint", "internal/app/last_result.go")
	add("background execution", "wraps native downloads and supports --no-wait worker recovery", "internal/app/publication_worker.go", "internal/app/publication_execution.go", "internal/app/publication.go")
	add("CLI wiring", "creates the root command and validates the selected capability before execution", "internal/cli/root.go")
	add("CLI wiring", "defines workbook pull flags, selectors, batch behavior, and renderer forwarding", "internal/cli/content/command.go", "internal/cli/content/batch.go")
	addDir("CLI wiring", "registers the content command tree that contains workbook.pull", "internal/cli/content", false)
	add("CLI support", "provides shared root argument, batch, help, and shorthand behavior", "internal/cli/arguments.go", "internal/cli/batch.go", "internal/cli/completion.go", "internal/cli/help.go", "internal/cli/help_content.go", "internal/cli/help_content_reference.go", "internal/cli/help_examples.go", "internal/cli/help_navigation.go", "internal/cli/help_reference.go", "internal/cli/help_values.go", "internal/cli/search.go", "internal/cli/shorthand.go")
	addDir("CLI support", "provides shared CLI error behavior used by the selected command", "internal/cli/clierr", false)
	addDir("CLI support", "provides progress labels emitted by the action adapters", "internal/cli/progress", false)
	addDir("output", "renders compact/full structured output and redacts output values", "internal/output", false)
	addDir("follow-up hints", "builds contextual follow-up command suggestions in the action result", "internal/commandhint", false)
	addDir("startup registry", "validates capability registration during every CLI startup", "internal/capability", false)
	addDir("guidance platform", "controls the executable's platform-specific guidance notice", "internal/guidancenotice", false)
	addDir("release notice", "controls the executable's release notice around app.Run", "internal/releasenotice", false)

	addDir("workflow action", "contains the workbook.pull validation, orchestration, contracts, and output behavior", "actions/workbook/pull", false)
	add("workbook resource adapter", "resolves authoritative workbooks and normalizes project identity", "internal/resources/workbook/adapter.go")
	add("published dependency adapter", "reads direct published datasource references", "internal/resources/workbook/references.go")
	add("lineage resource adapter", "captures and bounds Metadata API lineage", "internal/resources/lineage/adapter.go")
	add("datasource resource adapter", "downloads optional direct published datasource dependencies", "internal/resources/datasource/adapter.go")
	add("project resource adapter", "resolves exact Tableau project paths used by workbook identity", "internal/resources/project/adapter.go")
	add("workbook Tableau client", "performs workbook REST resolution and native download", "internal/tableau/workbook/client.go")
	add("metadata Tableau client", "performs direct dependency and lineage Metadata API requests", "internal/tableau/metadata/client.go", "internal/tableau/metadata/lineage.go")
	add("datasource Tableau client", "performs optional published datasource downloads", "internal/tableau/datasource/client.go")
	add("project Tableau client", "lists project hierarchy for exact project paths", "internal/tableau/project/client.go", "internal/tableau/project/types.go")
	addDir("Tableau transport", "performs authenticated bounded HTTP transport", "internal/tableau", false)
	addDir("Tableau authentication", "creates Tableau authentication requests for command sessions", "internal/tableau/auth", false)

	add("artifact persistence", "writes, validates, fingerprints, recovers, and replaces workbook artifacts", "internal/artifact/workbook.go", "internal/artifact/bundle.go", "internal/artifact/datasource.go", "internal/artifact/datasource_composition.go", "internal/artifact/flow.go", "internal/artifact/pull_preview.go", "internal/artifact/workspacelock.go")
	addDir("workspace lock", "serializes local artifact writes", "internal/lock", false)
	addDir("authentication", "loads command-scoped PAT sessions without exposing secrets", "internal/auth", false)
	add("configuration", "loads environment and workspace configuration", "internal/config/config.go", "internal/config/load_error.go", "internal/config/selection_error.go")
	add("workspace registry", "resolves registered logical workspaces without mutation", "internal/workspace/manager.go", "internal/workspace/recovery.go")
	addDir("identity", "resolves exact LUID/name/project selectors", "internal/identity", false)
	addDir("error contract", "carries operation, phase, retry, identity, and request diagnostics", "internal/errs", false)
	add("value contracts", "provides shared lineage, acquisition, source, and saved-result values", "internal/value/acquisition.go", "internal/value/lineage.go", "internal/value/saved_execution.go", "internal/value/source.go")
	addDir("path validation", "validates workspace-relative paths", "internal/pathspec", false)
	add("operation tracking", "records and launches bounded background operations", "internal/operationrun/launch.go", "internal/operationrun/store.go")
	addDir("last-result store", "persists the previous bounded command result", "internal/lastcommand", false)

	// These platform files are deliberately both in the context. The current
	// host is not evidence that the other supported targets can be removed.
	add("platform compatibility", "provides durable directory synchronization on supported Unix targets", "internal/artifact/fsync_unix.go")
	add("platform compatibility", "provides the Windows directory synchronization implementation", "internal/artifact/fsync_windows.go")
	add("platform compatibility", "provides platform-specific workspace locking implementations", "internal/lock/lock_unix.go", "internal/lock/lock_windows.go")
	add("background execution", "provides platform-specific detached process launching", "internal/operationrun/launch_unix.go", "internal/operationrun/launch_windows.go")

	// Tests are selected by an explicit workflow marker or by their ownership of
	// a branch-specific package. They are evidence, not automatic deletion
	// candidates.
	addDir("workflow tests", "tests the workbook.pull action directly", "actions/workbook/pull", true)
	for _, directory := range []string{
		"internal/resources/workbook", "internal/resources/lineage", "internal/resources/datasource", "internal/resources/project",
		"internal/tableau/workbook", "internal/tableau/metadata",
	} {
		addDir("workflow tests", "tests the selected resource or Tableau dependency", directory, true)
	}
	add("workflow tests", "tests the workbook artifact persistence contract", "internal/artifact/bundle_test.go", "internal/artifact/datasource_composition_test.go", "internal/artifact/datasource_test.go", "internal/artifact/lineage_test.go", "internal/artifact/pull_preview_test.go", "internal/artifact/workbook_internal_test.go", "internal/artifact/workbook_test.go")
	add("workflow tests", "tests the optional background worker and operation recovery branch", "internal/app/publication_worker_e2e_test.go", "internal/app/publication_batch_worker_e2e_test.go", "internal/app/publication_status_recovery_e2e_test.go", "internal/app/pull_quiet_e2e_test.go")
	terms := []string{"workbook.pull", "workbookpull", "WorkbookPull", "pullService", "include-pds", "nativeDownload"}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && ignoredWorkflowEvidenceDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if strings.HasPrefix(relative, "internal/architecture/") {
			return nil
		}
		for _, term := range terms {
			if strings.Contains(string(data), term) {
				selection.add(relative, "workflow test", "contains verified workbook.pull workflow evidence marker: "+term)
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan workflow tests: %v", err)
	}

	trace := []workflowTraceEdge{
		{From: "cmd/tadx/main.go", To: "internal/app/app.go", Relationship: "invokes app.Run after executable notices", Evidence: "main calls app.Run(ctx, args, stdout, app.Options{...})"},
		{From: "internal/app/app.go", To: "internal/cli/root.go", Relationship: "constructs the root command and supplies WorkbookPuller", Evidence: "Run calls cli.NewRoot with WorkbookPuller: &pullService{runtime: runtime}"},
		{From: "internal/cli/content/command.go", To: "actions/workbook/pull/action.go", Relationship: "forwards parsed input through Puller.Execute", Evidence: "newPull.RunE invokes deps.Puller.Execute(ctx, item)"},
		{From: "internal/app/app.go", To: "actions/workbook/pull/action.go", Relationship: "validates setup and constructs the action", Evidence: "pullService.Execute calls ValidateInput and workbookpull.New(...).Execute"},
		{From: "actions/workbook/pull/action.go", To: "internal/resources/workbook/adapter.go", Relationship: "resolves and downloads workbook data through Reader", Evidence: "Action.Reader contract is implemented by app.pullReader"},
		{From: "actions/workbook/pull/action.go", To: "internal/artifact/workbook.go", Relationship: "materializes the canonical local workbook artifact", Evidence: "Action.ArtifactWriter contract is implemented by app.artifactWriter"},
		{From: "actions/workbook/pull/action.go", To: "internal/artifact/bundle.go", Relationship: "materializes workbook plus direct datasource siblings when --include-pds is set", Evidence: "Action selects WriteBundle when dependencies were acquired"},
		{From: "internal/app/app.go", To: "internal/tableau/workbook/client.go", Relationship: "constructs the authenticated workbook client", Evidence: "runtime.clients creates tableauworkbook.NewClient"},
		{From: "internal/app/app.go", To: "internal/tableau/metadata/client.go", Relationship: "constructs the Metadata API client", Evidence: "runtime.clients creates tableaumetadata.NewClient"},
	}
	selectionRules := []string{
		"Include the executable entrypoint and the composition/CLI files that select workbook.pull.",
		"Include production source in the action, selected adapters, selected Tableau clients, artifact persistence, and shared services they directly use.",
		"Include both platform variants whenever the selected behavior has a platform-specific implementation.",
		"Include tests in selected packages and repository tests containing verified workbook.pull markers.",
	}
	limitations := []string{
		"The trace is a bounded, manually reviewed workflow trace; interface dispatch and same-package helper use are reported as evidence rather than inferred as a complete call graph.",
		"Package imports prove compile/package relationships, not that every imported function executes for every input branch.",
		"The composition root contains other command wiring; the LLM must distinguish startup/shared decisions from unrelated capabilities.",
		"The manifest intentionally includes platform variants, tests, and generator-adjacent shared support for review; none is classified as removable by this test.",
		"The LLM must verify repository-wide consumers before assigning repository_essential=\"no\".",
	}
	return selection, trace, "Workbook pull from CLI invocation through setup, exact Tableau identity resolution, native download, optional lineage/dependency enrichment, and transactional artifact persistence.", selectionRules, limitations
}

func writeWorkflowCatalogEvidence(t *testing.T, root string) {
	t.Helper()
	definitions := capability.All()
	allGoFiles := allRepositoryGoFiles(t, root)
	packageFiles, packagePaths := packageIndexes(t, root, allGoFiles)
	packageConsumers, packageImportEdges := importIndexes(t, root, allGoFiles, packagePaths)
	compiledTargets, diagnostics := compiledTargetIndex(t, root)
	selection := newWorkflowSelection()
	workflowPaths := make(map[string]map[string]struct{}, len(definitions))
	textPaths := make(map[string]struct{})

	shared := []string{
		"cmd/tadx/main.go",
		"internal/app/app.go",
		"internal/app/command_runtime.go",
		"internal/cli/root.go",
		"internal/cli/content/command.go",
		"internal/capability/registry.go",
		"internal/capability/definitions.go",
	}
	for _, definition := range definitions {
		workflowPaths[definition.ID] = make(map[string]struct{})
		for _, path := range shared {
			addCatalogWorkflowFile(selection, workflowPaths, definition.ID, path, "shared entry wiring", "shared executable, registry, and command composition context")
		}
	}

	goContents := make(map[string]string, len(allGoFiles))
	for _, path := range allGoFiles {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatalf("read repository source %s: %v", path, err)
		}
		goContents[path] = string(data)
	}

	items := make([]workflowCatalogItem, 0, len(definitions))
	for _, definition := range definitions {
		item := workflowCatalogItem{
			Workflow:        definition.ID,
			Surface:         definition.Surface,
			Outcome:         definition.Outcome,
			Type:            string(definition.Type),
			Disposition:     string(definition.Disposition),
			Owner:           string(definition.Owner),
			Implementation:  string(definition.Implementation),
			Verification:    string(definition.Verification),
			EvidenceLevel:   string(definition.EvidenceLevel),
			Blocker:         string(definition.Blocker),
			CommandPath:     append([]string(nil), definition.CommandPath...),
			Selectors:       definition.Selectors,
			Availability:    definition.Availability,
			SafetyGuard:     definition.SafetyGuard,
			ArtifactEffect:  definition.ArtifactEffect,
			Upstream:        definition.Upstream,
			LocalWrite:      definition.LocalWrite,
			RemoteMutation:  definition.RemoteMutation,
			SupportsPreview: definition.SupportsPreview,
			SupportsBatch:   definition.SupportsBatch,
			Administrative:  definition.Administrative,
		}

		actionDirectory := filepath.ToSlash(filepath.Join("actions", filepath.FromSlash(strings.ReplaceAll(definition.ID, ".", "/"))))
		if directoryExists(root, actionDirectory) {
			entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(actionDirectory)))
			if err != nil {
				t.Fatalf("read action directory %s: %v", actionDirectory, err)
			}
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
					continue
				}
				path := filepath.ToSlash(filepath.Join(actionDirectory, entry.Name()))
				addCatalogWorkflowFile(selection, workflowPaths, definition.ID, path, "workflow action", "matches the capability action directory")
			}
		}

		for _, rawPath := range strings.Split(definition.Evidence, ";") {
			reference := strings.TrimSpace(rawPath)
			if reference == "" {
				continue
			}
			path := filepath.ToSlash(filepath.Clean(reference))
			if fileExists(root, path) {
				item.EvidencePaths = append(item.EvidencePaths, workflowEvidencePath{Reference: reference, FilePath: path, Kind: "file", Exists: true})
				if strings.HasSuffix(path, ".go") {
					addCatalogWorkflowFile(selection, workflowPaths, definition.ID, path, "declared evidence", "listed by the capability registry")
				} else {
					textPaths[path] = struct{}{}
				}
			} else if looksLikeFileReference(path) {
				item.EvidencePaths = append(item.EvidencePaths, workflowEvidencePath{Reference: reference, FilePath: path, Kind: "missing-file", Exists: false})
			} else {
				item.EvidencePaths = append(item.EvidencePaths, workflowEvidencePath{Reference: reference, Kind: "description", Exists: false})
			}
		}

		for path, contents := range goContents {
			if strings.HasPrefix(path, "internal/architecture/") {
				continue
			}
			if strings.Contains(contents, definition.ID) {
				addCatalogWorkflowFile(selection, workflowPaths, definition.ID, path, "workflow reference", "contains the exact capability ID")
			}
		}
		item.EvidencePaths = sortEvidencePaths(item.EvidencePaths)
		item.Files = sortedWorkflowPaths(workflowPaths[definition.ID])
		items = append(items, item)
	}

	files := collectWorkflowFileEvidence(t, root, selection, packageFiles, packagePaths, packageConsumers, compiledTargets)
	textEvidence := collectTextEvidence(t, root, textPaths)
	evidence := workflowCatalogEvidence{
		SchemaVersion:  "1",
		GeneratedBy:    "go test -tags=workflowevidence ./internal/architecture -run TestWorkflowEvidence with TADX_WORKFLOW=all",
		Workflows:      items,
		Files:          files,
		TextEvidence:   textEvidence,
		PackageImports: packageImportEdges,
		SelectionRules: []string{
			"Enumerate workflow identities from the canonical capability registry rather than inventing command names.",
			"Include shared executable/CLI/registry wiring, matching action directories, declared registry evidence, and exact capability-ID references.",
			"Retain delegated and blocked capabilities in the catalog with their implementation and evidence state; absence of an action directory is not a deletion conclusion.",
		},
		EvidenceRules: []string{
			"Files and imports are deterministic evidence only; the catalog does not assign workflow or repository essentiality.",
			"Exact-ID text references are verified textual references, not proof of execution or ownership.",
			"Registry evidence paths are preserved even when a referenced file is missing so the LLM can identify evidence gaps.",
			"Platform-specific files, tests, and generators require responsibility review and are never discarded by selection heuristics.",
			"An empty consumer/reference list is not evidence of unused code.",
		},
		Diagnostics: diagnostics,
		Limitations: []string{
			"The all-workflow catalog is a deterministic routing manifest; named workflow traces such as workbook.pull remain more precise than exact-ID text matching.",
			"Interface dispatch, reflection, generated files, embedded assets, and runtime configuration are surfaced as limitations for LLM review rather than guessed.",
			"The composition root and shared packages contain unrelated workflows; the LLM must separate forwarding/startup code from meaningful domain decisions.",
			"Repository-wide repository_essential=\"no\" conclusions require the complete file inventory and broader consumer evidence, not this catalog alone.",
		},
	}
	outputDir := evidenceOutputDir(root)
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		t.Fatalf("create workflow evidence directory: %v", err)
	}
	jsonData, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		t.Fatalf("encode workflow catalog evidence: %v", err)
	}
	jsonData = append(jsonData, '\n')
	jsonPath := filepath.Join(outputDir, "tadx-workflows.json")
	if err := os.WriteFile(jsonPath, jsonData, 0o600); err != nil {
		t.Fatalf("write workflow catalog evidence: %v", err)
	}
	contextPath := filepath.Join(outputDir, "tadx-context.md")
	if err := os.WriteFile(contextPath, []byte(renderWorkflowCatalogContext(evidence)), 0o600); err != nil {
		t.Fatalf("write workflow catalog context: %v", err)
	}
	totalLines := 0
	for _, file := range files {
		totalLines += file.Lines
	}
	t.Logf("workflows=%d files=%d text_evidence=%d physical_lines=%d", len(items), len(files), len(textEvidence), totalLines)
	t.Logf("workflow catalog JSON: %s", evidenceDisplayPath(root, jsonPath))
	t.Logf("workflow catalog context: %s", evidenceDisplayPath(root, contextPath))
}

func addCatalogWorkflowFile(selection workflowSelection, workflowPaths map[string]map[string]struct{}, workflow, path, role, reason string) {
	path = filepath.ToSlash(filepath.Clean(path))
	selection.add(path, role, reason)
	if workflowPaths[workflow] == nil {
		workflowPaths[workflow] = make(map[string]struct{})
	}
	workflowPaths[workflow][path] = struct{}{}
}

func directoryExists(root, path string) bool {
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(path)))
	return err == nil && info.IsDir()
}

func fileExists(root, path string) bool {
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(path)))
	return err == nil && !info.IsDir()
}

func looksLikeFileReference(value string) bool {
	return strings.Contains(value, "/") || strings.Contains(value, `\\`) || strings.HasSuffix(value, ".go") || strings.HasSuffix(value, ".md") || strings.HasSuffix(value, ".json")
}

func sortEvidencePaths(paths []workflowEvidencePath) []workflowEvidencePath {
	sort.Slice(paths, func(i, j int) bool { return paths[i].Reference < paths[j].Reference })
	return paths
}

func sortedWorkflowPaths(paths map[string]struct{}) []string {
	result := make([]string, 0, len(paths))
	for path := range paths {
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}

func collectTextEvidence(t *testing.T, root string, paths map[string]struct{}) []workflowTextEvidence {
	t.Helper()
	ordered := sortedWorkflowPaths(paths)
	result := make([]workflowTextEvidence, 0, len(ordered))
	for _, path := range ordered {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatalf("read text evidence %s: %v", path, err)
		}
		result = append(result, workflowTextEvidence{Path: path, Bytes: len(data), Lines: physicalLines(data), SHA256: sha256Hex(data), Source: string(data)})
	}
	return result
}

func renderWorkflowCatalogContext(evidence workflowCatalogEvidence) string {
	var builder strings.Builder
	builder.WriteString("# Deterministic TADX workflow catalog evidence\n\n")
	builder.WriteString("This is evidence for a true LLM. It is not a deletion recommendation or behavioral classification.\n\n")
	builder.WriteString("## Workflows\n\n")
	for _, item := range evidence.Workflows {
		builder.WriteString("### `" + item.Workflow + "`\n\n")
		builder.WriteString("- Surface: " + item.Surface + "\n")
		builder.WriteString("- Outcome: " + item.Outcome + "\n")
		builder.WriteString("- Implementation: `" + item.Implementation + "`; verification: `" + item.Verification + "`; evidence level: `" + item.EvidenceLevel + "`\n")
		builder.WriteString("- Files selected: " + strconv.Itoa(len(item.Files)) + "\n")
		if len(item.EvidencePaths) > 0 {
			builder.WriteString("- Registry evidence: ")
			for index, path := range item.EvidencePaths {
				if index > 0 {
					builder.WriteString(", ")
				}
				builder.WriteString("`" + path.Reference + "`")
				if path.Kind == "missing-file" {
					builder.WriteString(" (missing file)")
				} else if path.Kind == "description" {
					builder.WriteString(" (description)")
				}
			}
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}
	builder.WriteString("## Source files\n\n")
	for _, file := range evidence.Files {
		builder.WriteString("### `" + file.Path + "`\n\n")
		builder.WriteString("- Kind: `" + file.Kind + "`; package: `" + file.Package + "`; roles: `" + strings.Join(file.Roles, "`, `") + "`\n")
		builder.WriteString("- Physical lines: " + strconv.Itoa(file.Lines) + "; nonblank: " + strconv.Itoa(file.NonBlankLines) + "\n\n")
		builder.WriteString("```go\n" + file.Source)
		if !strings.HasSuffix(file.Source, "\n") {
			builder.WriteByte('\n')
		}
		builder.WriteString("```\n\n")
	}
	for _, file := range evidence.TextEvidence {
		builder.WriteString("### `" + file.Path + "`\n\n")
		builder.WriteString("```text\n" + file.Source)
		if !strings.HasSuffix(file.Source, "\n") {
			builder.WriteByte('\n')
		}
		builder.WriteString("```\n\n")
	}
	return builder.String()
}

func ignoredWorkflowEvidenceDirectory(name string) bool {
	switch name {
	case ".git", ".tadx-explore", "archived", "jev-evals", "vendor", "workflow-evidence":
		return true
	default:
		return false
	}
}

func allRepositoryGoFiles(t *testing.T, root string) []string {
	t.Helper()
	var result []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && ignoredWorkflowEvidenceDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		result = append(result, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		t.Fatalf("scan repository Go files: %v", err)
	}
	sort.Strings(result)
	return result
}

func packageIndexes(t *testing.T, root string, files []string) (map[string][]string, map[string]string) {
	t.Helper()
	packageFiles := make(map[string][]string)
	packagePaths := make(map[string]string)
	for _, path := range files {
		directory := filepath.ToSlash(filepath.Dir(path))
		packagePath := packageImportPath(directory)
		packageFiles[packagePath] = append(packageFiles[packagePath], path)
		packagePaths[path] = packagePath
	}
	for packagePath := range packageFiles {
		sort.Strings(packageFiles[packagePath])
	}
	return packageFiles, packagePaths
}

func importIndexes(t *testing.T, root string, files []string, packagePaths map[string]string) (map[string][]string, []workflowPackageImport) {
	t.Helper()
	consumers := make(map[string][]string)
	var edges []workflowPackageImport
	seenEdges := make(map[string]struct{})
	for _, path := range files {
		absolute := filepath.Join(root, filepath.FromSlash(path))
		imports, _ := fileImports(t, absolute)
		for _, imported := range imports {
			if !strings.HasPrefix(imported, workflowEvidenceModule) {
				continue
			}
			consumers[imported] = append(consumers[imported], path)
			key := path + "\x00" + imported
			if _, exists := seenEdges[key]; exists {
				continue
			}
			seenEdges[key] = struct{}{}
			resolved := append([]string(nil), packagePathsForImport(imported, packagePaths)...)
			edges = append(edges, workflowPackageImport{FromFile: path, Imported: imported, ResolvedFiles: resolved, Evidence: "parsed import declaration"})
		}
	}
	for packagePath := range consumers {
		consumers[packagePath] = uniqueSorted(consumers[packagePath])
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].FromFile != edges[j].FromFile {
			return edges[i].FromFile < edges[j].FromFile
		}
		return edges[i].Imported < edges[j].Imported
	})
	return consumers, edges
}

func packagePathsForImport(imported string, packagePaths map[string]string) []string {
	var result []string
	for path, packagePath := range packagePaths {
		if packagePath == imported {
			result = append(result, path)
		}
	}
	sort.Strings(result)
	return result
}

func compiledTargetIndex(t *testing.T, root string) (map[string][]string, []string) {
	t.Helper()
	result := make(map[string][]string)
	var diagnostics []string
	for _, goos := range []string{"windows", "linux", "darwin"} {
		command := exec.Command("go", "list", "-json", "-deps", "./cmd/tadx")
		command.Dir = root
		command.Env = append(os.Environ(), "GOOS="+goos, "GOARCH=amd64", "CGO_ENABLED=0")
		output, err := command.Output()
		if err != nil {
			diagnostics = append(diagnostics, fmt.Sprintf("go list for %s failed: %v", goos, err))
			continue
		}
		decoder := json.NewDecoder(strings.NewReader(string(output)))
		for decoder.More() {
			var item goListEvidence
			if err := decoder.Decode(&item); err != nil {
				diagnostics = append(diagnostics, fmt.Sprintf("decode go list for %s failed: %v", goos, err))
				break
			}
			if item.Error != nil || !strings.HasPrefix(item.ImportPath, workflowEvidenceModule) {
				continue
			}
			paths := item.CompiledGoFiles
			if len(paths) == 0 {
				paths = append(append([]string{}, item.GoFiles...), item.CgoFiles...)
			}
			for _, path := range paths {
				if !filepath.IsAbs(path) {
					path = filepath.Join(item.Dir, path)
				}
				relative, err := filepath.Rel(root, path)
				if err != nil || strings.HasPrefix(relative, "..") {
					continue
				}
				relative = filepath.ToSlash(relative)
				result[relative] = append(result[relative], goos)
			}
		}
	}
	for path := range result {
		result[path] = uniqueSorted(result[path])
	}
	sort.Strings(diagnostics)
	return result, diagnostics
}

func fileImports(t *testing.T, path string) ([]string, []string) {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse imports in %s: %v", path, err)
	}
	imports := make([]string, 0, len(file.Imports))
	local := make([]string, 0)
	for _, spec := range file.Imports {
		value, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			t.Fatalf("unquote import in %s: %v", path, err)
		}
		imports = append(imports, value)
		if strings.HasPrefix(value, workflowEvidenceModule) {
			local = append(local, value)
		}
	}
	sort.Strings(imports)
	sort.Strings(local)
	return uniqueSorted(imports), uniqueSorted(local)
}

func packageImportPath(directory string) string {
	directory = filepath.ToSlash(filepath.Clean(directory))
	if directory == "." || directory == "" {
		return workflowEvidenceModule
	}
	return workflowEvidenceModule + "/" + directory
}

func physicalLines(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	return strings.Count(string(data), "\n") + 1
}

func nonBlankLines(data []byte) int {
	count := 0
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

func buildConstraints(data []byte) []string {
	var result []string
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//go:build ") || strings.HasPrefix(trimmed, "// +build ") {
			result = append(result, trimmed)
		}
	}
	return uniqueSorted(result)
}

func generatedDirectives(data []byte) []string {
	var result []string
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//go:generate") || strings.HasPrefix(trimmed, "// Code generated") {
			result = append(result, trimmed)
		}
	}
	return uniqueSorted(result)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func sortedSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func uniqueSorted(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := append([]string(nil), values...)
	sort.Strings(result)
	write := 0
	for _, value := range result {
		if write == 0 || result[write-1] != value {
			result[write] = value
			write++
		}
	}
	return result[:write]
}

func countFiles(files []workflowFileEvidence, kind string) int {
	count := 0
	for _, file := range files {
		if file.Kind == kind {
			count++
		}
	}
	return count
}

func totalLines(files []workflowFileEvidence, nonBlank bool) int {
	total := 0
	for _, file := range files {
		if nonBlank {
			total += file.NonBlankLines
		} else {
			total += file.Lines
		}
	}
	return total
}

func renderWorkflowContext(evidence workflowEvidence) string {
	var builder strings.Builder
	builder.WriteString("# Deterministic TADX workflow evidence\n\n")
	builder.WriteString("This file is generated evidence. It is not an LLM classification and does not authorize deletion.\n\n")
	builder.WriteString("- Workflow: `" + evidence.Workflow + "`\n")
	builder.WriteString("- Summary: " + evidence.Summary + "\n")
	builder.WriteString("- Files: " + strconv.Itoa(len(evidence.Files)) + "\n\n")
	builder.WriteString("## Evidence rules\n\n")
	for _, rule := range evidence.EvidenceRules {
		builder.WriteString("- " + rule + "\n")
	}
	builder.WriteString("\n## Workflow trace\n\n")
	for _, edge := range evidence.Trace {
		builder.WriteString("- `" + edge.From + "` -> `" + edge.To + "`: " + edge.Relationship + ". Evidence: " + edge.Evidence + "\n")
	}
	builder.WriteString("\n## Files\n\n")
	for _, file := range evidence.Files {
		builder.WriteString("### `" + file.Path + "`\n\n")
		builder.WriteString("- Kind: `" + file.Kind + "`\n")
		builder.WriteString("- Package: `" + file.Package + "`\n")
		builder.WriteString("- Roles: `" + strings.Join(file.Roles, "`, `") + "`\n")
		builder.WriteString("- Physical lines: " + strconv.Itoa(file.Lines) + "; nonblank: " + strconv.Itoa(file.NonBlankLines) + "\n")
		if len(file.BuildConstraints) > 0 {
			builder.WriteString("- Build constraints: `" + strings.Join(file.BuildConstraints, "`, `") + "`\n")
		}
		if len(file.GeneratedDirectives) > 0 {
			builder.WriteString("- Generated directives: `" + strings.Join(file.GeneratedDirectives, "`, `") + "`\n")
		}
		builder.WriteString("\n```go\n")
		builder.WriteString(file.Source)
		if !strings.HasSuffix(file.Source, "\n") {
			builder.WriteByte('\n')
		}
		builder.WriteString("```\n\n")
	}
	return builder.String()
}
