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
	flag.Parse()

	document, err := capability.GenerateMarkdown(capability.All())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *out == "" {
		if _, err := os.Stdout.Write(document); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := os.MkdirAll(filepath.Dir(filepath.Clean(*out)), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Clean(*out), document, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
