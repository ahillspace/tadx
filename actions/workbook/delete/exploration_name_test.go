package delete

import (
	"testing"

	"github.com/ahillspace/tadx/internal/identity"
)

func TestExplorationWorkbookDeletePreviewKeepsExactNameSelector(t *testing.T) {
	input, err := normalizeInput(Input{Selector: identity.Selector{Name: "Finance ", ProjectPath: "Ops"}})
	if err != nil || input.Selector.Name != "Finance " {
		t.Fatalf("exact selector changed: %#v, %v", input.Selector, err)
	}
}
