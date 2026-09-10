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
		if selector, ok := deps.BatchSelectors[cmd.Annotations[CapabilityAnnotation]]; ok && cmd.RunE != nil {
			attachBatch(root, cmd, selector, deps.Renderer, factory)
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
	var file string
	var repeated *repeatedSelector
	if flag := command.Flags().Lookup(selector); selector != "" && flag != nil && flag.Value.Type() == "string" {
		repeated = &repeatedSelector{Value: flag.Value}
		flag.Value = repeated
		flag.Usage += "; repeat for a sequential batch"
	}
	command.Flags().StringVar(&file, "batch-file", "", "JSON items with per-item flags for this action (1-100); shared environment and preview stay outside the file")
	originalArgs, originalRun := command.Args, command.RunE
	var rows [][]string
	command.Args = func(cmd *cobra.Command, args []string) error {
		rows = nil
		isFile := cmd.Flags().Changed("batch-file")
		isRepeated := repeated != nil && len(repeated.values) > 1
		if !isFile && !isRepeated {
			if originalArgs != nil {
				return originalArgs(cmd, args)
			}
			return nil
		}
		if len(args) != 0 {
			return clierr.Usage(cmd.Annotations[CapabilityAnnotation], errors.New("batch items use named flags, not positional arguments"))
		}
		if isFile && isRepeated {
			return clierr.Usage(cmd.Annotations[CapabilityAnnotation], errors.New("use --batch-file or repeated selectors, not both"))
		}
		var items []map[string]json.RawMessage
		var err error
		if isFile {
			items, err = readBatchItems(file)
		} else {
			if err = contentbatch.Validate(repeated.values); err == nil {
				for _, id := range repeated.values {
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
			argv, count, err := batchArguments(cmd, item)
			if err != nil {
				return clierr.Usage(cmd.Annotations[CapabilityAnnotation], fmt.Errorf("batch item %d: %w", index+1, err))
			}
			work += count
			if work > contentbatch.MaxItems {
				return clierr.Usage(cmd.Annotations[CapabilityAnnotation], fmt.Errorf("batch expands beyond %d selections", contentbatch.MaxItems))
			}
			key, _ := json.Marshal(argv)
			if seen[string(key)] {
				return clierr.Usage(cmd.Annotations[CapabilityAnnotation], fmt.Errorf("duplicate batch item %d", index+1))
			}
			seen[string(key)] = true
			argv = append(append([]string(nil), path...), argv...)
			// Parse and validate all flag types, selectors in Args, and required
			// groups before dispatch. RunE/action validation still owns semantics.
			check := factory(&batchCollector{})
			leaf, _, err := check.Find(path)
			if err != nil {
				return err
			}
			leaf.RunE = func(*cobra.Command, []string) error { return nil }
			check.SetOut(io.Discard)
			check.SetErr(io.Discard)
			check.SetArgs(argv)
			if err := check.ExecuteContext(cmd.Context()); err != nil {
				return clierr.Usage(cmd.Annotations[CapabilityAnnotation], fmt.Errorf("batch item %d: %w", index+1, err))
			}
			rows = append(rows, argv)
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
			child.SetOut(io.Discard)
			child.SetErr(cmd.ErrOrStderr())
			child.SetArgs(rows[i-1])
			err := child.ExecuteContext(ctx)
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
	values := map[string][]string{}
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
		switch name {
		case "environment", "config", "preview", "json", "full", "batch-file", "help", "version", "raw", "force":
			return nil, 0, fmt.Errorf("--%s must be selected on the command, not in a batch item", name)
		}
		flag := command.Flags().Lookup(name)
		if flag == nil || flag.Name != name {
			return nil, 0, fmt.Errorf("unknown or noncanonical flag %q", name)
		}
		var value any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			return nil, 0, err
		}
		var entries []any
		if list, ok := value.([]any); ok {
			if _, ok := flag.Value.(pflag.SliceValue); !ok {
				return nil, 0, fmt.Errorf("--%s is not repeatable", name)
			}
			if len(list) == 0 || len(list) > contentbatch.MaxItems {
				return nil, 0, fmt.Errorf("--%s requires 1-%d values", name, contentbatch.MaxItems)
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
				return nil, 0, fmt.Errorf("--%s requires scalar values", name)
			}
			values[name] = append(values[name], str)
		}
	}
	names := make([]string, 0, len(values))
	for name, entries := range values {
		if len(entries) > work {
			work = len(entries)
		}
		names = append(names, name)
	}
	sort.Strings(names)
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
	return argv, work, nil
}
