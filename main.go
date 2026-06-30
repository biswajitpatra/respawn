package main

import "github.com/biswajitpatra/respawn/cmd"

// version is overridden at release time via -ldflags "-X main.version=...".
// For local `go build`/`go install`, it stays "dev" and the CLI enriches it
// with the embedded VCS revision.
var version = "dev"

func main() {
	cmd.Execute(version)
}
