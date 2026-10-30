// Command gencapdocs renders capability reference documentation from the registry.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ahillspace/tadx/internal/capability"
)

func main() {
	out := flag.String("out", "", "write generated Markdown to this file instead of stdout")
	jsonOut := flag.String("json-out", "", "also write generated capability data for visualizations")
	mapOut := flag.String("map-out", "", "update the embedded registry snapshot in an existing capability map")
	flag.Parse()

	document, err := capability.GenerateMarkdown(capability.All())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *jsonOut != "" {
		data, err := capability.GenerateJSON(capability.All())
		if err == nil {
			err = writeFile(*jsonOut, data)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if *mapOut != "" {
		data, err := capability.GenerateJSON(capability.All())
		if err == nil {
			err = updateMap(*mapOut, data, commandCounts(capability.All()))
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if *out == "" {
		if _, err := os.Stdout.Write(document); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := writeFile(*out, document); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// mapCount is one static command total the authored map renders before its script runs.
type mapCount struct {
	id    string
	value int
}

// commandCounts derives the map's static totals from CLI-owned commands; delegated
// registry entries are discoverable but are not TADX commands.
func commandCounts(definitions []capability.Definition) []mapCount {
	var total, implemented, planned, blocked, remote int
	for _, definition := range definitions {
		if definition.Owner != capability.OwnerCLI {
			continue
		}
		total++
		switch definition.Implementation {
		case capability.ImplementationImplemented:
			implemented++
		case capability.ImplementationPlanned:
			planned++
		}
		if definition.Verification == capability.VerificationBlocked {
			blocked++
		}
		if definition.RemoteMutation {
			remote++
		}
	}
	return []mapCount{
		{"sourceTotal", total}, {"totalCount", total}, {"footerTotal", total},
		{"implementedCount", implemented}, {"plannedCount", planned}, {"blockedCount", blocked},
		{"remoteMutationCount", remote},
	}
}

// updateMap replaces the registry data island and the static command totals;
// authored layout and behavior stay intact.
func updateMap(path string, data []byte, counts []mapCount) error {
	document, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return err
	}
	const marker = `<script id="capability-data" type="application/json">`
	if bytes.Count(document, []byte(marker)) != 1 {
		return fmt.Errorf("capability map must contain exactly one registry snapshot marker")
	}
	start := bytes.Index(document, []byte(marker)) + len(marker)
	end := bytes.Index(document[start:], []byte("</script>"))
	if end < 0 {
		return fmt.Errorf("capability map registry snapshot has no closing script tag")
	}
	authored, err := replaceCounts(document[:start], counts)
	if err != nil {
		return err
	}
	updated := append(authored, '\n')
	updated = append(updated, bytes.TrimSpace(data)...)
	updated = append(updated, '\n')
	updated = append(updated, document[start+end:]...)
	return writeFile(path, updated)
}

// replaceCounts returns a copy of the authored page head with each static total
// rewritten; every slot must appear once and hold only ASCII digits.
func replaceCounts(head []byte, counts []mapCount) ([]byte, error) {
	updated := append([]byte(nil), head...)
	for _, count := range counts {
		opening := []byte(`id="` + count.id + `">`)
		if occurrences := bytes.Count(updated, opening); occurrences != 1 {
			return nil, fmt.Errorf("capability map must contain exactly one %s total before the registry snapshot; found %d", count.id, occurrences)
		}
		valueStart := bytes.Index(updated, opening) + len(opening)
		valueEnd := bytes.IndexByte(updated[valueStart:], '<')
		if valueEnd <= 0 || !asciiDigits(updated[valueStart:valueStart+valueEnd]) {
			return nil, fmt.Errorf("capability map %s total must contain only digits", count.id)
		}
		value := []byte(strconv.Itoa(count.value))
		updated = append(updated[:valueStart:valueStart], append(value, updated[valueStart+valueEnd:]...)...)
	}
	return updated, nil
}

func asciiDigits(value []byte) bool {
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(filepath.Clean(path)), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Clean(path), data, 0o644)
}
