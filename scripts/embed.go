// Package scripts embeds the same supported installers shipped for one-line setup.
package scripts

import _ "embed"

//go:embed install.ps1
var WindowsInstaller string

//go:embed install.sh
var UnixInstaller string
