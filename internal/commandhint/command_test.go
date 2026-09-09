package commandhint_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/commandhint"
)

func TestShellQuoting(t *testing.T) {
	for _, test := range []struct {
		shell commandhint.Shell
		want  string
	}{
		{commandhint.POSIX, `tadx inspect --id 'one'"'"'two $HOME; "three"' --workspace 'a folder/path'`},
	} {
		got := commandhint.CommandFor(test.shell, "inspect", "--id", `one'two $HOME; "three"`, "--workspace", "a folder/path")
		if got != test.want {
			t.Fatalf("%s: got %q want %q", test.shell, got, test.want)
		}
	}
	if got := commandhint.CommandFor(commandhint.POSIX, "inspect", "--id", ""); got != "tadx inspect --id ''" {
		t.Fatal(got)
	}
}

func TestEnvironmentUsesResolvedValue(t *testing.T) {
	got := commandhint.Environment("production", "content", "workbook", "inspect", "--id", "wb-1")
	if got != "tadx content workbook inspect --id wb-1 --environment production" {
		t.Fatal(got)
	}
	if got := commandhint.Environment("", "version"); got != "tadx version" {
		t.Fatal(got)
	}
}

func TestHostShellReceivesLiteralArguments(t *testing.T) {
	value := "space ' $HOME `echo evil` ; & @name,other"
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "function tadx { [Console]::Write($args[0]) }; "+commandhint.Command(value))
	} else {
		cmd = exec.Command("sh", "-c", "tadx() { printf '%s' \"$1\"; }; "+commandhint.Command(value))
	}
	output, err := cmd.CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) != value {
		t.Fatalf("output=%q err=%v", output, err)
	}
}

func TestMain(m *testing.M) {
	if os.Getenv("TADX_HINT_ARGV_HELPER") == "1" {
		_ = json.NewEncoder(os.Stdout).Encode(os.Args[1:])
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestHostNativeProcessReceivesLiteralArguments(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows legacy native marshalling regression")
	}
	want := []string{`one"two`, `space ' " $HOME; &`, `slash\"quote`, "", "--full", "-test.run=example", "a.b", "foo:bar", `space ending\`}
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	script := "& '" + strings.ReplaceAll(path, "'", "''") + "' " + strings.TrimPrefix(commandhint.Command(want...), "tadx ")
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.Env = append(os.Environ(), "TADX_HINT_ARGV_HELPER=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("native argument probe: %v %s", err, output)
	}
	var got []string
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatal(fmt.Sprintf("decode native argv: %v output=%s", err, output))
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("native argv=%q want=%q", got, want)
	}
}
