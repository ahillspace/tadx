package capability

// implementationManifest is the checked-in inventory of executable CLI work.
// Contract generation describes product intent. This manifest records code that
// is actually present and keeps implementation state out of the generator.
var implementationManifest = map[string]struct {
	CommandPath []string
	RawCapable  bool
}{
	"agent.install":             {CommandPath: []string{"agent", "install"}},
	"agent.uninstall":           {CommandPath: []string{"agent", "uninstall"}},
	"auth.check":                {CommandPath: []string{"auth", "check"}},
	"auth.login":                {CommandPath: []string{"auth", "login"}},
	"auth.logout":               {CommandPath: []string{"auth", "logout"}},
	"auth.status":               {CommandPath: []string{"auth", "status"}},
	"admin.group.create":        {CommandPath: []string{"admin", "group", "create"}},
	"admin.group.delete":        {CommandPath: []string{"admin", "group", "delete"}},
	"admin.group.inspect":       {CommandPath: []string{"admin", "group", "inspect"}},
	"admin.group.list":          {CommandPath: []string{"admin", "group", "list"}},
	"admin.group.member.add":    {CommandPath: []string{"admin", "group", "member", "add"}},
	"admin.group.member.remove": {CommandPath: []string{"admin", "group", "member", "remove"}},
	"admin.group.update":        {CommandPath: []string{"admin", "group", "update"}},
	"admin.permission.inspect":  {CommandPath: []string{"admin", "permission", "inspect"}},
	"admin.permission.create":   {CommandPath: []string{"admin", "permission", "create"}},
	"admin.permission.delete":   {CommandPath: []string{"admin", "permission", "delete"}},
	"admin.user.create":         {CommandPath: []string{"admin", "user", "create"}},
	"admin.user.delete":         {CommandPath: []string{"admin", "user", "delete"}},
	"admin.user.inspect":        {CommandPath: []string{"admin", "user", "inspect"}},
	"admin.user.list":           {CommandPath: []string{"admin", "user", "list"}},
	"admin.user.update":         {CommandPath: []string{"admin", "user", "update"}},
	"capability.get":            {CommandPath: []string{"capability", "get"}},
	"capability.list":           {CommandPath: []string{"capability", "list"}},
	"catalog.refresh":           {CommandPath: []string{"catalog", "refresh"}},
	"catalog.status":            {CommandPath: []string{"catalog", "status"}},
	"datasource.inspect":        {CommandPath: []string{"content", "datasource", "inspect"}},
	"datasource.list":           {CommandPath: []string{"content", "datasource", "list"}},
	"datasource.move":           {CommandPath: []string{"content", "datasource", "move"}},
	"datasource.pull":           {CommandPath: []string{"content", "datasource", "pull"}},
	"datasource.publish":        {CommandPath: []string{"content", "datasource", "publish"}},
	"datasource.delete":         {CommandPath: []string{"content", "datasource", "delete"}},
	"datasource.schema":         {CommandPath: []string{"content", "datasource", "schema"}},
	"datasource.update":         {CommandPath: []string{"content", "datasource", "update"}},
	"doctor.run":                {CommandPath: []string{"doctor"}},
	"env.profile.add":           {CommandPath: []string{"env", "add"}},
	"env.profile.get":           {CommandPath: []string{"env", "get"}},
	"env.profile.list":          {CommandPath: []string{"env", "list"}},
	"env.profile.remove":        {CommandPath: []string{"env", "remove"}},
	"env.profile.set-default":   {CommandPath: []string{"env", "default"}},
	"env.profile.update":        {CommandPath: []string{"env", "update"}},
	"flow.delete":               {CommandPath: []string{"content", "flow", "delete"}},
	"flow.inspect":              {CommandPath: []string{"content", "flow", "inspect"}},
	"flow.list":                 {CommandPath: []string{"content", "flow", "list"}},
	"flow.move":                 {CommandPath: []string{"content", "flow", "move"}},
	"flow.publish":              {CommandPath: []string{"content", "flow", "publish"}},
	"flow.pull":                 {CommandPath: []string{"content", "flow", "pull"}},
	"flow.update":               {CommandPath: []string{"content", "flow", "update"}},
	"lineage.pull":              {CommandPath: []string{"content", "lineage", "pull"}},
	"project.inspect":           {CommandPath: []string{"content", "project", "inspect"}},
	"project.list":              {CommandPath: []string{"content", "project", "list"}},
	"project.move":              {CommandPath: []string{"content", "project", "move"}},
	"project.create":            {CommandPath: []string{"content", "project", "create"}},
	"project.update":            {CommandPath: []string{"content", "project", "update"}},
	"project.delete":            {CommandPath: []string{"content", "project", "delete"}},
	"pulse.definition.create":   {CommandPath: []string{"pulse", "definition", "create"}},
	"pulse.definition.inspect":  {CommandPath: []string{"pulse", "definition", "inspect"}},
	"pulse.definition.delete":   {CommandPath: []string{"pulse", "definition", "delete"}},
	"pulse.definition.list":     {CommandPath: []string{"pulse", "definition", "list"}},
	"pulse.definition.pull":     {CommandPath: []string{"pulse", "definition", "pull"}},
	"pulse.metric.follow":       {CommandPath: []string{"pulse", "metric", "follow"}},
	"pulse.metric.followers":    {CommandPath: []string{"pulse", "metric", "followers"}},
	"pulse.metric.fork":         {CommandPath: []string{"pulse", "metric", "fork"}},
	"pulse.metric.inspect":      {CommandPath: []string{"pulse", "metric", "inspect"}},
	"pulse.metric.delete":       {CommandPath: []string{"pulse", "metric", "delete"}},
	"pulse.metric.list":         {CommandPath: []string{"pulse", "metric", "list"}},
	"pulse.metric.unfollow":     {CommandPath: []string{"pulse", "metric", "unfollow"}},
	"workspace.artifact.delete": {CommandPath: []string{"workspace", "artifact", "delete"}},
	"workspace.clean":           {CommandPath: []string{"workspace", "clean"}},
	"workspace.clone":           {CommandPath: []string{"workspace", "clone"}},
	"workspace.create":          {CommandPath: []string{"workspace", "create"}},
	"workspace.list":            {CommandPath: []string{"workspace", "list"}},
	"workspace.delete":          {CommandPath: []string{"workspace", "delete"}},
	"workspace.register":        {CommandPath: []string{"workspace", "register"}},
	"workspace.move":            {CommandPath: []string{"workspace", "artifact", "move"}},
	"workspace.set-default":     {CommandPath: []string{"workspace", "set-default"}},
	"workspace.status":          {CommandPath: []string{"workspace", "status"}},
	"workspace.unregister":      {CommandPath: []string{"workspace", "unregister"}},
	"workbook.publish":          {CommandPath: []string{"content", "workbook", "publish"}},
	"workbook.delete":           {CommandPath: []string{"content", "workbook", "delete"}},
	"workbook.inspect":          {CommandPath: []string{"content", "workbook", "inspect"}},
	"workbook.list":             {CommandPath: []string{"content", "workbook", "list"}},
	"workbook.move":             {CommandPath: []string{"content", "workbook", "move"}},
	"workbook.pull":             {CommandPath: []string{"content", "workbook", "pull"}},
	"workbook.update":           {CommandPath: []string{"content", "workbook", "update"}},
	"search.run":                {CommandPath: []string{"search"}},
	"version.get":               {CommandPath: []string{"version"}},
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
