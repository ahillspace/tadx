package workbook

import (
	"testing"

	"github.com/ahillspace/tadx/internal/identity"
)

func TestDeleteExplorationWorkbookDeletePreviewKeepsExactNameSelector(t *testing.T) {
	input, err := deleteNormalizeInput(DeleteInput{Selector: identity.Selector{Name: "Finance ", ProjectPath: "Ops"}})
	if err != nil || input.Selector.Name != "Finance " {
		t.Fatalf("exact selector changed: %#v, %v", input.Selector, err)
	}
}
