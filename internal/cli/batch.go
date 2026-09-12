package cli

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/ahillspace/tadx/internal/batchspec"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/ahillspace/tadx/internal/contentbatch"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const maxBatchBytes = 1 << 20

type batchCollector struct{ value any }

func (c *batchCollector) Render(value any) error { c.value = value; return nil }

// Each item gets fresh flag bindings but shares the invocation's services,
// configuration snapshot, authenticated sessions and credential coordination.
func enableBatches(root *cobra.Command, deps Dependencies) {
	factory := func(renderer Renderer) *cobra.Command {
		child := deps
		child.Renderer = renderer
		if deps.RenderOptions != nil {
			copy := *deps.RenderOptions
			child.RenderOptions = &copy
		}
		if deps.ConfigPath != nil {
			copy := *deps.ConfigPath
			child.ConfigPath = &copy
		}
		return newRoot(child, false)
	}
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		id := cmd.Annotations[CapabilityAnnotation]
		options, ok := deps.BatchOptions[id]
		if !ok {
			if selector, legacy := deps.BatchSelectors[id]; legacy {
				ok = true
				if selector != "" {
					options.Selectors = []string{selector}
				}
			}
		}
		if ok && cmd.RunE != nil {
			attachBatchWithOptions(root, cmd, options, deps.Renderer, factory)
		}
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(root)
}

type repeatedSelector struct {
	pflag.Value
	values []string
}

func (r *repeatedSelector) Set(value string) error {
	if len(r.values) >= contentbatch.MaxItems {
		return fmt.Errorf("select at most %d items", contentbatch.MaxItems)
	}
	if err := r.Value.Set(value); err != nil {
		return err
	}
	r.values = append(r.values, value)
	return nil
}

func attachBatch(root, command *cobra.Command, selector string, renderer Renderer, factory func(Renderer) *cobra.Command) {
	options := batchspec.Options{}
	if selector != "" {
		options.Selectors = []string{selector}
	}
	attachBatchWithOptions(root, command, options, renderer, factory)
}

type batchRow struct {
	argv       []string
	emptyLists []string
}

func attachBatchWithOptions(root, command *cobra.Command, options batchspec.Options, renderer Renderer, factory func(Renderer) *cobra.Command) {
	var file string
	for _, selector := range options.Selectors {
		if flag := command.Flags().Lookup(selector); flag != nil && flag.Value.Type() == "string" {
			flag.Value = &repeatedSelector{Value: flag.Value}
			flag.Usage += "; repeat for a sequential batch"
		}
	}
	if command.Annotations == nil {
		command.Annotations = map[string]string{}
	}
	command.Annotations["tadx.batch.file"] = "true"
	command.Annotations["tadx.batch.selectors"] = strings.Join(options.Selectors, ",")
	command.Annotations["tadx.batch.native-selections"] = strings.Join(options.NativeSelections, ",")
	command.Annotations["tadx.batch.positional"] = strconv.FormatBool(options.Positional)
	command.Annotations["tadx.batch.environment"] = strconv.FormatBool(options.AllowEnvironment)
	command.Annotations["tadx.batch.max-items"] = strconv.Itoa(contentbatch.MaxItems)
	usage := "JSON items with per-item flags for this action (1-100); preview and invocation controls stay outside the file"
	if options.Positional {
		usage += "; positional values use args:[...]"
	}
	if !options.AllowEnvironment {
		usage += "; environment is shared"
	}
	command.Flags().StringVar(&file, "batch-file", "", usage)
	originalArgs, originalRun := command.Args, command.RunE
	var rows []batchRow
	command.Args = func(cmd *cobra.Command, args []string) error {
		rows = nil
		isFile := cmd.Flags().Changed("batch-file")
		selector, values, err := varyingBatchSelector(cmd, options)
		if err != nil {
			return clierr.Usage(cmd.Annotations[CapabilityAnnotation], err)
		}
		positional := options.Positional && len(args) > 1
		if positional && selector != "" {
			return clierr.Usage(cmd.Annotations[CapabilityAnnotation], errors.New("repeat only one selector dimension; use explicit batch-file rows for different pairs"))
		}
		// Existing native selector slices keep their single invocation contract.
		// They still participate in the ambiguous-dimension check above.
		generic := false
		if selector != "" {
			_, generic = cmd.Flags().Lookup(selector).Value.(*repeatedSelector)
		}
		if !isFile && !generic && !positional {
			if originalArgs != nil {
				return originalArgs(cmd, args)
			}
			return nil
		}
		if len(args) != 0 && !options.Positional {
			return clierr.Usage(cmd.Annotations[CapabilityAnnotation], errors.New("this action's batch items do not accept positional arguments"))
		}
		if isFile && (generic || positional) {
			return clierr.Usage(cmd.Annotations[CapabilityAnnotation], errors.New("use --batch-file or repeated selectors, not both"))
		}
		var items []map[string]json.RawMessage
		if isFile {
			items, err = readBatchItems(file)
		} else if positional {
			if err = contentbatch.Validate(args); err == nil {
				for _, value := range args {
					encoded, _ := json.Marshal([]string{value})
					items = append(items, map[string]json.RawMessage{"args": encoded})
				}
			}
		} else {
			if err = contentbatch.Validate(values); err == nil {
				for _, id := range values {
					encoded, _ := json.Marshal(id)
					items = append(items, map[string]json.RawMessage{selector: encoded})
				}
			}
		}
		if err != nil {
			return clierr.Usage(cmd.Annotations[CapabilityAnnotation], err)
		}
		path := strings.Fields(strings.TrimPrefix(cmd.CommandPath(), root.Name()+" "))
		seen := map[string]bool{}
		work := 0
		for index, item := range items {
			row, count, err := batchRowArguments(cmd, item, args, options)
			if err != nil {
				return clierr.Usage(cmd.Annotations[CapabilityAnnotation], fmt.Errorf("batch item %d: %w", index+1, err))
			}
			work += count
			if work > contentbatch.MaxItems {
				return clierr.Usage(cmd.Annotations[CapabilityAnnotation], fmt.Errorf("batch expands beyond %d selections", contentbatch.MaxItems))
			}
			key, _ := json.Marshal(struct {
				Args  []string
				Empty []string
			}{row.argv, row.emptyLists})
			if seen[string(key)] {
				return clierr.Usage(cmd.Annotations[CapabilityAnnotation], fmt.Errorf("duplicate batch item %d", index+1))
			}
			seen[string(key)] = true
			row.argv = append(append([]string(nil), path...), row.argv...)
			// Parse and validate all flag types, selectors in Args, and required
			// groups before dispatch. RunE/action validation still owns semantics.
			check := factory(&batchCollector{})
			leaf, _, err := check.Find(path)
			if err != nil {
				return err
			}
			if err := applyEmptyBatchLists(leaf, row.emptyLists); err != nil {
				return err
			}
			checkArgs := leaf.Args
			leaf.Args = func(cmd *cobra.Command, args []string) error {
				if _, _, err := varyingBatchSelector(cmd, options); err != nil {
					return err
				}
				if checkArgs != nil {
					return checkArgs(cmd, args)
				}
				return nil
			}
			leaf.RunE = func(*cobra.Command, []string) error { return nil }
			check.SetOut(io.Discard)
			check.SetErr(io.Discard)
			check.SetArgs(row.argv)
			if err := check.ExecuteContext(cmd.Context()); err != nil {
				return clierr.Usage(cmd.Annotations[CapabilityAnnotation], fmt.Errorf("batch item %d: %w", index+1, err))
			}
			rows = append(rows, row)
		}
		return nil
	}
	command.RunE = func(cmd *cobra.Command, args []string) error {
		if rows == nil {
			return originalRun(cmd, args)
		}
		indices := make([]string, len(rows))
		for i := range rows {
			indices[i] = strconv.Itoa(i + 1)
		}
		out, err := contentbatch.Run(cmd.Context(), cmd.Annotations[CapabilityAnnotation], indices, func(ctx context.Context, index string) (any, error) {
			i, _ := strconv.Atoi(index)
			capture := &batchCollector{}
			child := factory(capture)
			row := rows[i-1]
			path := strings.Fields(strings.TrimPrefix(cmd.CommandPath(), root.Name()+" "))
			leaf, _, err := child.Find(path)
			if err != nil {
				return nil, err
			}
			if err := applyEmptyBatchLists(leaf, row.emptyLists); err != nil {
				return nil, err
			}
			child.SetOut(io.Discard)
			child.SetErr(cmd.ErrOrStderr())
			child.SetArgs(row.argv)
			err = child.ExecuteContext(ctx)
			var partial interface{ OperationOutput() any }
			if capture.value == nil && errors.As(err, &partial) {
				capture.value = partial.OperationOutput()
			}
			return capture.value, err
		})
		if renderErr := renderer.Render(out); renderErr != nil {
			return renderErr
		}
		if err != nil {
			return clierr.Rendered(err)
		}
		return nil
	}
}

// varyingBatchSelector distinguishes independent target dimensions from native
// action properties such as repeated capabilities, filters or desired members.
func varyingBatchSelector(command *cobra.Command, options batchspec.Options) (string, []string, error) {
	var selected string
	var values []string
	for _, name := range options.Selectors {
		flag := command.Flags().Lookup(name)
		if flag == nil {
			continue
		}
		var entries []string
		switch value := flag.Value.(type) {
		case *repeatedSelector:
			entries = value.values
		case pflag.SliceValue:
			entries = value.GetSlice()
		}
		if len(entries) <= 1 {
			continue
		}
		if selected != "" {
			return "", nil, errors.New("repeat only one selector dimension; use explicit batch-file rows for different pairs")
		}
		selected, values = name, entries
	}
	return selected, values, nil
}

func applyEmptyBatchLists(command *cobra.Command, names []string) error {
	for _, name := range names {
		flag := command.Flags().Lookup(name)
		if flag == nil {
			return fmt.Errorf("unknown list flag %q", name)
		}
		value, ok := flag.Value.(pflag.SliceValue)
		if !ok {
			return fmt.Errorf("--%s is not a list", name)
		}
		if err := value.Replace([]string{}); err != nil {
			return err
		}
		flag.Changed = true
	}
	return nil
}

func readBatchItems(path string) ([]map[string]json.RawMessage, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("--batch-file requires a regular JSON file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read batch file: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxBatchBytes {
		return nil, errors.New("batch file must be a regular file no larger than 1 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBatchBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxBatchBytes {
		return nil, errors.New("batch file exceeds 1 MiB")
	}
	if err := uniqueJSONKeys(data); err != nil {
		return nil, fmt.Errorf("invalid batch JSON: %w", err)
	}
	var document struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode batch file: %w", err)
	}
	if len(document.Items) == 0 || len(document.Items) > contentbatch.MaxItems {
		return nil, fmt.Errorf("batch file requires 1-%d items", contentbatch.MaxItems)
	}
	for _, item := range document.Items {
		if item == nil {
			return nil, errors.New("batch items must be flag objects")
		}
	}
	return document.Items, nil
}

func uniqueJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 8 {
			return errors.New("JSON nesting exceeds 8")
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		seen := map[string]bool{}
		for decoder.More() {
			if delim == '{' {
				key, err := decoder.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok {
					return errors.New("invalid object key")
				}
				if seen[name] {
					return fmt.Errorf("duplicate key %q", name)
				}
				seen[name] = true
			}
			if err := walk(depth + 1); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	}
	if err := walk(0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("expected exactly one JSON document")
	}
	return nil
}

func batchArguments(command *cobra.Command, item map[string]json.RawMessage) ([]string, int, error) {
	row, count, err := batchRowArguments(command, item, nil, batchspec.Options{})
	return row.argv, count, err
}

func batchRowArguments(command *cobra.Command, item map[string]json.RawMessage, inheritedArgs []string, options batchspec.Options) (batchRow, int, error) {
	values := map[string][]string{}
	positionals := append([]string(nil), inheritedArgs...)
	var row batchRow
	command.Flags().Visit(func(flag *pflag.Flag) {
		if flag.Name == "batch-file" {
			return
		}
		if list, ok := flag.Value.(pflag.SliceValue); ok {
			values[flag.Name] = list.GetSlice()
		} else {
			values[flag.Name] = []string{flag.Value.String()}
		}
	})
	work := 1
	for name, raw := range item {
		if name == "args" {
			if !options.Positional {
				return row, 0, errors.New("this action does not accept positional batch args")
			}
			var value any
			if err := json.Unmarshal(raw, &value); err != nil {
				return row, 0, err
			}
			entries, ok := value.([]any)
			if !ok || len(entries) > contentbatch.MaxItems {
				return row, 0, fmt.Errorf("args requires an array of at most %d strings", contentbatch.MaxItems)
			}
			positionals = nil
			for _, entry := range entries {
				value, ok := entry.(string)
				if !ok {
					return row, 0, errors.New("args requires string values")
				}
				positionals = append(positionals, value)
			}
			continue
		}
		switch name {
		case "environment":
			if !options.AllowEnvironment {
				return row, 0, errors.New("--environment must be selected on the command, not in a batch item")
			}
		case "config", "preview", "json", "full", "batch-file", "help", "version", "raw", "force":
			return row, 0, fmt.Errorf("--%s must be selected on the command, not in a batch item", name)
		}
		flag := command.Flags().Lookup(name)
		if flag == nil || flag.Name != name {
			return row, 0, fmt.Errorf("unknown or noncanonical flag %q", name)
		}
		var value any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			return row, 0, err
		}
		var entries []any
		if list, ok := value.([]any); ok {
			if _, ok := flag.Value.(pflag.SliceValue); !ok {
				return row, 0, fmt.Errorf("--%s is not a list; use separate batch items for scalar selectors", name)
			}
			entries = list
		} else {
			entries = []any{value}
		}
		values[name] = nil
		for _, entry := range entries {
			var str string
			switch v := entry.(type) {
			case string:
				str = v
			case bool:
				str = strconv.FormatBool(v)
			case json.Number:
				str = string(v)
			default:
				return row, 0, fmt.Errorf("--%s requires scalar values", name)
			}
			values[name] = append(values[name], str)
		}
	}
	names := make([]string, 0, len(values))
	for name, entries := range values {
		if len(entries) == 0 {
			row.emptyLists = append(row.emptyLists, name)
		}
		names = append(names, name)
	}
	// Count native target/rule actions, not property values within one action.
	// Row and byte bounds remain global; each action validates its own lists.
	for _, name := range append(append([]string(nil), options.Selectors...), options.NativeSelections...) {
		flag := command.Flags().Lookup(name)
		if flag == nil {
			continue
		}
		if _, ok := flag.Value.(pflag.SliceValue); !ok {
			continue
		}
		entries, provided := values[name]
		count := len(entries)
		if !provided {
			count = len(flag.Value.(pflag.SliceValue).GetSlice())
		}
		if count > contentbatch.MaxItems || (count > 0 && work > contentbatch.MaxItems/count) {
			return row, 0, fmt.Errorf("batch expands beyond %d selections", contentbatch.MaxItems)
		}
		if count > 1 {
			work *= count
		}
	}
	sort.Strings(names)
	sort.Strings(row.emptyLists)
	var argv []string
	for _, name := range names {
		flag := command.Flags().Lookup(name)
		for _, value := range values[name] {
			if flag.Value.Type() == "stringSlice" {
				var buf bytes.Buffer
				writer := csv.NewWriter(&buf)
				_ = writer.Write([]string{value})
				writer.Flush()
				value = strings.TrimSuffix(buf.String(), "\n")
			}
			argv = append(argv, "--"+name+"="+value)
		}
	}
	if len(positionals) != 0 {
		argv = append(argv, "--")
		argv = append(argv, positionals...)
	}
	row.argv = argv
	return row, work, nil
}
