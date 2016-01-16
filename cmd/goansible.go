package main

import (
	"os"

	"github.com/masahide/goansible"
	_ "github.com/masahide/goansible/net"
	_ "github.com/masahide/goansible/package"
	_ "github.com/masahide/goansible/procmgmt"
)

var Release string
var version string

func main() {
	if Release != "" {
		goansible.Release = Release
	}
	if version != "" {
		goansible.Version = version
	}

	os.Exit(goansible.Main(os.Args))
}
