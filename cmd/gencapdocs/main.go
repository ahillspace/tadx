// Command gencapdocs renders capability reference documentation from the registry.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"

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
			err = updateMap(*mapOut, data)
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

// updateMap replaces only the data island; authored layout and behavior stay intact.
func updateMap(path string, data []byte) error {
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
	updated := append([]byte(nil), document[:start]...)
	updated = append(updated, '\n')
	updated = append(updated, bytes.TrimSpace(data)...)
	updated = append(updated, '\n')
	updated = append(updated, document[start+end:]...)
	return writeFile(path, updated)
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(filepath.Clean(path)), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Clean(path), data, 0o644)
}
