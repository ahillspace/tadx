package content

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestContentPublicationHelpExplainsNoWaitAndFlowSync(t *testing.T) {
	for _, test := range []struct {
		resource string
		want     string
		avoid    string
	}{
		{resource: "workbook", want: "--no-wait: one single/batch status command", avoid: "flow publication is synchronous"},
		{resource: "flow", want: "Flow publication can complete synchronously", avoid: "flow publication is synchronous;"},
	} {
		t.Run(test.resource, func(t *testing.T) {
			resource := &cobra.Command{Use: test.resource}
			publish := &cobra.Command{Use: "publish"}
			resource.AddCommand(publish)
			var output bytes.Buffer
			WriteCompactReferenceNotes(&output, resource, []*cobra.Command{publish})
			if !strings.Contains(output.String(), test.want) || strings.Contains(output.String(), test.avoid) {
				t.Fatalf("publication notes = %s", output.String())
			}
		})
	}
}
