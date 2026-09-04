package capability

// implementationManifest is the checked-in inventory of executable CLI work.
// Contract generation describes product intent. This manifest records code that
// is actually present and keeps implementation state out of the generator.
var implementationManifest = map[string]struct {
	CommandPath []string
	RawCapable  bool
}{
	"auth.check":                {CommandPath: []string{"auth", "check"}},
	"auth.status":               {CommandPath: []string{"auth", "status"}},
	"admin.group.create":        {CommandPath: []string{"admin", "group", "create"}},
	"admin.group.delete":        {CommandPath: []string{"admin", "group", "delete"}},
	"admin.group.get":           {CommandPath: []string{"admin", "group", "get"}},
	"admin.group.list":          {CommandPath: []string{"admin", "group", "list"}},
	"admin.group.update":        {CommandPath: []string{"admin", "group", "update"}},
	"admin.permission.get":      {CommandPath: []string{"admin", "permission", "get"}},
	"admin.user.create":         {CommandPath: []string{"admin", "user", "create"}},
	"admin.user.delete":         {CommandPath: []string{"admin", "user", "delete"}},
	"admin.user.get":            {CommandPath: []string{"admin", "user", "get"}},
	"admin.user.list":           {CommandPath: []string{"admin", "user", "list"}},
	"admin.user.update":         {CommandPath: []string{"admin", "user", "update"}},
	"capability.get":            {CommandPath: []string{"capability", "get"}},
	"capability.list":           {CommandPath: []string{"capability", "list"}},
	"catalog.search":            {CommandPath: []string{"catalog", "search"}},
	"catalog.refresh":           {CommandPath: []string{"catalog", "refresh"}},
	"catalog.status":            {CommandPath: []string{"catalog", "status"}},
	"datasource.get":            {CommandPath: []string{"content", "datasource", "get"}},
	"datasource.list":           {CommandPath: []string{"content", "datasource", "list"}},
	"datasource.pull":           {CommandPath: []string{"content", "datasource", "pull"}},
	"datasource.publish":        {CommandPath: []string{"content", "datasource", "publish"}},
	"datasource.delete":         {CommandPath: []string{"content", "datasource", "delete"}},
	"datasource.schema":         {CommandPath: []string{"content", "datasource", "schema"}},
	"doctor.run":                {CommandPath: []string{"doctor"}},
	"env.profile.add":           {CommandPath: []string{"env", "add"}},
	"env.profile.get":           {CommandPath: []string{"env", "get"}},
	"env.profile.list":          {CommandPath: []string{"env", "list"}},
	"env.profile.remove":        {CommandPath: []string{"env", "remove"}},
	"env.profile.set-default":   {CommandPath: []string{"env", "default"}},
	"env.profile.update":        {CommandPath: []string{"env", "update"}},
	"flow.delete":               {CommandPath: []string{"content", "flow", "delete"}},
	"flow.get":                  {CommandPath: []string{"content", "flow", "get"}},
	"flow.list":                 {CommandPath: []string{"content", "flow", "list"}},
	"flow.move":                 {CommandPath: []string{"content", "flow", "move"}},
	"flow.publish":              {CommandPath: []string{"content", "flow", "publish"}},
	"flow.pull":                 {CommandPath: []string{"content", "flow", "pull"}},
	"lineage.pull":              {CommandPath: []string{"content", "lineage", "pull"}},
	"project.get":               {CommandPath: []string{"content", "project", "get"}},
	"project.list":              {CommandPath: []string{"content", "project", "list"}},
	"project.create":            {CommandPath: []string{"content", "project", "create"}},
	"project.update":            {CommandPath: []string{"content", "project", "update"}},
	"pulse.definition.create":   {CommandPath: []string{"pulse", "definition", "create"}},
	"pulse.definition.get":      {CommandPath: []string{"pulse", "definition", "get"}},
	"pulse.definition.list":     {CommandPath: []string{"pulse", "definition", "list"}},
	"pulse.definition.pull":     {CommandPath: []string{"pulse", "definition", "pull"}},
	"pulse.metric.follow":       {CommandPath: []string{"pulse", "metric", "follow"}},
	"pulse.metric.followers":    {CommandPath: []string{"pulse", "metric", "followers"}},
	"pulse.metric.fork":         {CommandPath: []string{"pulse", "metric", "fork"}},
	"pulse.metric.get":          {CommandPath: []string{"pulse", "metric", "get"}},
	"pulse.metric.list":         {CommandPath: []string{"pulse", "metric", "list"}},
	"pulse.metric.unfollow":     {CommandPath: []string{"pulse", "metric", "unfollow"}},
	"workspace.artifact.delete": {CommandPath: []string{"workspace", "artifact", "delete"}},
	"workspace.clean":           {CommandPath: []string{"workspace", "clean"}},
	"workspace.clone":           {CommandPath: []string{"workspace", "clone"}},
	"workspace.create":          {CommandPath: []string{"workspace", "create"}},
	"workspace.list":            {CommandPath: []string{"workspace", "list"}},
	"workspace.register":        {CommandPath: []string{"workspace", "register"}},
	"workspace.move":            {CommandPath: []string{"workspace", "move"}},
	"workspace.status":          {CommandPath: []string{"workspace", "status"}},
	"workbook.publish":          {CommandPath: []string{"content", "workbook", "publish"}},
	"workbook.delete":           {CommandPath: []string{"content", "workbook", "delete"}},
	"workbook.get":              {CommandPath: []string{"content", "workbook", "get"}},
	"workbook.list":             {CommandPath: []string{"content", "workbook", "list"}},
	"workbook.pull":             {CommandPath: []string{"content", "workbook", "pull"}},
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
