package main

import (
	"fmt"
	"os"

	"github.com/abdul-hamid-achik/file.cheap/internal/fcheap/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
