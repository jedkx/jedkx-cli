// Package app holds product identity: name, version and the mascot.
// Everything user-visible that names the product reads from here.
package app

// Version is set at build time: -ldflags "-X github.com/jedkx/jedkx-cli/internal/app.Version=v0.1.0".
var Version = "dev"

const (
	Name      = "jedkx-cli"
	Repo      = "jedkx/jedkx-cli"
	Mascot    = "Nox"
	Publisher = "jedkx"
	Homepage  = "https://github.com/" + Repo
)
