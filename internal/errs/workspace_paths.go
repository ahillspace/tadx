package errs

import (
	"cmp"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// workspaceRelativeError renders a workspace failure with every path under a
// workspace root shown workspace-relative with forward slashes. Operating
// system errors embed absolute paths, and rendered paths must never reveal the
// machine-local workspace root or home directory. The original chain stays
// available to errors.Is and errors.As so classification and retry advice are
// unchanged.
type workspaceRelativeError struct {
	err     error
	message string
}

func (e *workspaceRelativeError) Error() string { return e.message }

func (e *workspaceRelativeError) Unwrap() error { return e.err }

// WithWorkspaceRelativePaths renders paths under the supplied roots relative
// to those roots while retaining the original error chain for classification.
func WithWorkspaceRelativePaths(err error, roots ...string) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	relative := relativizeWorkspacePaths(message, workspaceRootForms(roots))
	if relative == message {
		return err
	}
	return &workspaceRelativeError{err: err, message: relative}
}

// workspaceRootForms returns each root as supplied, resolved through symbolic
// links, with forward slashes, and as it appears inside a Go-quoted string,
// longest first so the most specific form wins.
func workspaceRootForms(roots []string) []string {
	seen := map[string]bool{}
	var forms []string
	add := func(value string) {
		for _, form := range []string{value, strings.ReplaceAll(value, `\`, "/"), strings.Trim(strconv.Quote(value), `"`)} {
			if form != "" && form != "/" && !seen[form] {
				seen[form] = true
				forms = append(forms, form)
			}
		}
	}
	for _, root := range roots {
		root = strings.TrimRight(root, `/\`)
		if root == "" {
			continue
		}
		add(root)
		if resolved, err := filepath.EvalSymlinks(root); err == nil {
			add(strings.TrimRight(resolved, `/\`))
		}
	}
	slices.SortStableFunc(forms, func(left, right string) int { return cmp.Compare(len(right), len(left)) })
	return forms
}

func relativizeWorkspacePaths(message string, forms []string) string {
	if len(forms) == 0 {
		return message
	}
	var builder strings.Builder
	for index := 0; index < len(message); {
		form, ok := rootFormAt(message, index, forms)
		if !ok {
			builder.WriteByte(message[index])
			index++
			continue
		}
		end := index + len(form)
		tokenEnd := workspacePathTokenEnd(message, end, forms)
		relative := strings.TrimLeft(strings.ReplaceAll(strings.ReplaceAll(message[end:tokenEnd], `\\`, "/"), `\`, "/"), "/")
		if relative == "" {
			relative = "."
		}
		builder.WriteString(relative)
		index = tokenEnd
	}
	return builder.String()
}

func rootFormAt(message string, index int, forms []string) (string, bool) {
	for _, form := range forms {
		if !strings.HasPrefix(message[index:], form) {
			continue
		}
		end := index + len(form)
		if end == len(message) || strings.ContainsRune(`/\"': ;,)]`+"\n\t", rune(message[end])) {
			return form, true
		}
	}
	return "", false
}

// workspacePathTokenEnd finds where a path that starts at a workspace root
// ends: at a quote, a line break, a ": ", "; ", or ", " separator, the end of
// the message, or a space that begins another rooted path (as in the old and
// new paths of a rename error). Other spaces belong to the path.
func workspacePathTokenEnd(message string, start int, forms []string) int {
	for index := start; index < len(message); index++ {
		switch message[index] {
		case '"', '\'', '\n', '\t':
			return index
		case ':', ';', ',':
			if index+1 == len(message) || message[index+1] == ' ' {
				return index
			}
		case ' ':
			if _, ok := rootFormAt(message, index+1, forms); ok {
				return index
			}
		}
	}
	return len(message)
}
