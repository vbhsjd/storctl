package main

import (
	"os"

	"storctl/internal/storctl"
)

func main() {
	os.Exit(storctl.Main(os.Args[1:], os.Stdout, os.Stderr))
}
