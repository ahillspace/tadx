// Command gencapdocs renders capability reference documentation from the registry.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ahillspace/tadx/internal/capability"
)

func main() {
	out := flag.String("out", "", "write generated Markdown to this file instead of stdout")
	jsonOut := flag.String("json-out", "", "also write generated capability data for visualizations")
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

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(filepath.Clean(path)), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Clean(path), data, 0o644)
}
