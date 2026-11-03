package artifact

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func BenchmarkInventoryPayloadReads(b *testing.B) {
	workspace := b.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "tadx.yaml"), []byte("version: 1\n"), 0o600); err != nil {
		b.Fatal(err)
	}
	manager := NewDatasourceManager(time.Now)
	payload := append(bytes.Repeat([]byte(" "), 4*1024*1024), []byte("<datasource/>")...)
	for i := range 8 {
		if _, err := manager.Pull(b.Context(), DatasourcePull{Workspace: workspace, Filename: "Sales.tds", Metadata: validDatasourceMetadata(fmt.Sprintf("Sales %d", i), fmt.Sprintf("ds-%d", i)), PayloadPath: PayloadFile(b, payload)}); err != nil {
			b.Fatal(err)
		}
	}
	b.Run("status", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := Inventory(b.Context(), workspace, InventoryOptions{Limit: 1}); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("exact-identity", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := Resolve(b.Context(), workspace, Selector{Kind: "datasource", LUID: "ds-0"}); err != nil {
				b.Fatal(err)
			}
		}
	})
}
