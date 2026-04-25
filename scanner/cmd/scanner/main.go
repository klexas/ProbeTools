package main

import (
	"fmt"
	"os"

	"github.com/klexas/ProbeTools/scanner/internal/app"
	"github.com/klexas/ProbeTools/scanner/internal/config"
)

func main() {
	cfg, err := config.ParseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	if err := app.Run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
