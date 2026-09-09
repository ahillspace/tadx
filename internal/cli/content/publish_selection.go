package content

import "errors"

// publishSelections retains path batches while making convenient selectors singular.
func publishSelections(paths []string, kind, targetName, file, id, name string) ([]string, error) {
	count := 0
	for _, value := range []string{file, id, name} {
		if value != "" {
			count++
		}
	}
	if len(paths) > 0 {
		count++
	}
	if count != 1 {
		return nil, errors.New("use exactly one of --file, --id, --artifact-name, or --artifact")
	}
	if len(paths) > 0 {
		return paths, validateArtifactSelection(paths, kind, targetName)
	}
	// One empty path invokes the normal single-item boundary with the typed selector.
	return []string{""}, nil
}
