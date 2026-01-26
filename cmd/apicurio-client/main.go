package main

import (
	"os"

	"github.com/ymocode/apicurio-client/internal/cli"
)

// Version is set via ldflags during build
var Version = "dev"

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
