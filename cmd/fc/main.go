package main

import (
	"fmt"
	"os"

	"github.com/abdul-hamid-achik/file-processor/internal/fc/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
