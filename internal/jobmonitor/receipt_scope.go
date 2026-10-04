package jobmonitor

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// ReceiptScope binds an intent to the exact pre-write target.
func ReceiptScope(receipt Receipt) string {
	scope := struct {
		Operation, Environment, Server, Site, SiteID, ConfigPath, SourcePath, ProjectID, Name, CoordinationKey string
	}{receipt.Operation, receipt.Environment, receipt.Server, receipt.Site, receipt.SiteID, receipt.ConfigPath, receipt.SourcePath, receipt.ProjectID, receipt.Name, receipt.CoordinationKey}
	data, _ := json.Marshal(scope)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
