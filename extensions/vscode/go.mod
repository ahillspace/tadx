// This tooling boundary keeps Go source in Node dependencies outside root CLI checks.
// The extension has no production Go package. Its host fixture has a separate module.
module github.com/ahillspace/tadx/extensions/vscode

go 1.26
