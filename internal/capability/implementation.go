package capability

// implementationManifest is the checked-in inventory of executable CLI work.
// Contract generation describes product intent. This manifest records code that
// is actually present and keeps implementation state out of the generator.
var implementationManifest = map[string]struct {
	CommandPath []string
	RawCapable  bool
}{
	"auth.check":       {CommandPath: []string{"auth", "check"}},
	"capability.get":   {CommandPath: []string{"capability", "get"}},
	"capability.list":  {CommandPath: []string{"capability", "list"}},
	"catalog.search":   {CommandPath: []string{"catalog", "search"}},
	"workbook.publish": {CommandPath: []string{"content", "workbook", "publish"}},
	"workbook.pull":    {CommandPath: []string{"content", "workbook", "pull"}},
}

func applyImplementationManifest(definition Definition) Definition {
	implemented, ok := implementationManifest[definition.ID]
	if !ok {
		return definition
	}
	definition.Implementation = ImplementationImplemented
	definition.CommandPath = slicesClone(implemented.CommandPath)
	definition.RawCapable = implemented.RawCapable
	return definition
}

func slicesClone(values []string) []string {
	return append([]string(nil), values...)
}
