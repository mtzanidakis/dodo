package main

import (
	"fmt"
	"os"

	"github.com/mtzanidakis/dodo/internal/admin"
	"github.com/mtzanidakis/dodo/internal/api"
	"github.com/mtzanidakis/dodo/internal/backup"
	"github.com/mtzanidakis/dodo/internal/config"
)

var (
	version = "dev"
	commit  = "none"
)

func main() {
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "serve":
			cfg, err := config.Load()
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				os.Exit(1)
			}
			if err := api.Serve(cfg, version, commit); err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				os.Exit(1)
			}
		case "admin":
			os.Exit(admin.Run(os.Args[2:], version, commit))
		case "backup":
			os.Exit(backup.Run(os.Args[2:]))
		case "-h", "--help":
			fmt.Print(usage)
		default:
			fmt.Print(usage)
			os.Exit(2)
		}
		return
	}

	fmt.Print(usage)
	os.Exit(2)
}

const usage = `dodo - todo service

Usage:
  dodo serve   Run the HTTP API server + scheduler + telegram pollers
  dodo admin   Manage users and api tokens (direct DB access)
  dodo backup  Online backup of the database (-dump <dest>)

Flags:
  -h, --help   Show this help

Version: dev
`
