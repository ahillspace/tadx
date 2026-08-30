package toon_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/toon"
)

type upstreamFixture struct {
	Version  string         `json:"version"`
	Category string         `json:"category"`
	Tests    []upstreamCase `json:"tests"`
}

type upstreamCase struct {
	Name        string          `json:"name"`
	Input       json.RawMessage `json:"input"`
	Expected    json.RawMessage `json:"expected"`
	ShouldError bool            `json:"shouldError"`
	MinVersion  string          `json:"minSpecVersion"`
	Options     struct {
		Delimiter string `json:"delimiter"`
		Indent    int    `json:"indentSize"`
		Strict    *bool  `json:"strict"`
	} `json:"options"`
}

type conformanceCounts struct {
	encode          int
	strictDecode    int
	nonStrictDecode int
}

func TestUpstreamV411Conformance(t *testing.T) {
	root := os.Getenv("TOON_FIXTURES")
	vendored := root == ""
	if root == "" {
		root = filepath.Join("testdata", "upstream-v4.1.1")
	}

	var counts conformanceCounts
	for _, category := range []string{"encode", "decode"} {
		entries, err := os.ReadDir(filepath.Join(root, category))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			data, err := os.ReadFile(filepath.Join(root, category, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			var fixture upstreamFixture
			if err := json.Unmarshal(data, &fixture); err != nil {
				t.Fatal(err)
			}
			if fixture.Category != category {
				t.Fatalf("%s: category %q does not match directory %q", entry.Name(), fixture.Category, category)
			}
			if !supportsSpecVersion(fixture.Version) {
				t.Fatalf("%s: fixture baseline version %q is not compatible with codec version %q", entry.Name(), fixture.Version, toon.SpecVersion)
			}
			for index, test := range fixture.Tests {
				if test.MinVersion != "" && !supportsSpecVersion(test.MinVersion) {
					t.Fatalf("%s case %d: minimum spec version %q is not compatible with codec version %q", entry.Name(), index, test.MinVersion, toon.SpecVersion)
				}
				if category == "encode" {
					counts.encode++
				} else if test.Options.Strict != nil && !*test.Options.Strict {
					counts.nonStrictDecode++
				} else {
					counts.strictDecode++
				}

				name := fmt.Sprintf("%s/%s/%03d_%s", category, entry.Name(), index, strings.ReplaceAll(test.Name, "/", "_"))
				t.Run(name, func(t *testing.T) {
					if category == "encode" {
						var delimiter rune
						if test.Options.Delimiter != "" {
							delimiter = []rune(test.Options.Delimiter)[0]
						}
						got, err := toon.EncodeWithOptions(test.Input, toon.EncodeOptions{IndentSize: test.Options.Indent, Delimiter: delimiter})
						if test.ShouldError {
							if err == nil {
								t.Fatalf("case %d: expected error", index)
							}
							return
						}
						if err != nil {
							t.Fatalf("case %d: %v", index, err)
						}
						var want string
						if err := json.Unmarshal(test.Expected, &want); err != nil {
							t.Fatal(err)
						}
						if string(got) != want {
							t.Fatalf("case %d\nwant:\n%s\ngot:\n%s", index, want, got)
						}
						return
					}
					var input string
					if err := json.Unmarshal(test.Input, &input); err != nil {
						t.Fatal(err)
					}
					strict := true
					if test.Options.Strict != nil {
						strict = *test.Options.Strict
					}
					got, err := toon.DecodeWithOptions([]byte(input), toon.DecodeOptions{IndentSize: test.Options.Indent, Strict: strict})
					if test.ShouldError {
						if err == nil {
							t.Fatalf("case %d: expected error", index)
						}
						return
					}
					if err != nil {
						t.Fatalf("case %d: %v", index, err)
					}
					var want any
					if err := json.Unmarshal(test.Expected, &want); err != nil {
						t.Fatal(err)
					}
					gotJSON, err := json.Marshal(got)
					if err != nil {
						t.Fatal(err)
					}
					var comparable any
					if err := json.Unmarshal(gotJSON, &comparable); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(comparable, want) {
						t.Fatalf("case %d\nwant: %#v\ngot: %#v", index, want, comparable)
					}
				})
			}
		}
	}

	if vendored {
		want := conformanceCounts{encode: 179, strictDecode: 335, nonStrictDecode: 24}
		if counts != want {
			t.Fatalf("vendored fixture inventory changed: want %+v, got %+v", want, counts)
		}
	}
}

func supportsSpecVersion(required string) bool {
	requiredMajor, requiredMinor, ok := parseSpecVersion(required)
	if !ok {
		return false
	}
	supportedMajor, supportedMinor, ok := parseSpecVersion(toon.SpecVersion)
	return ok && requiredMajor == supportedMajor && requiredMinor <= supportedMinor
}

func parseSpecVersion(version string) (major int, minor int, ok bool) {
	parts := strings.Split(version, ".")
	if len(parts) != 2 {
		return 0, 0, false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, false
	}
	return major, minor, true
}
