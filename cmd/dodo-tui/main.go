package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/mtzanidakis/dodo/internal/clientconfig"
	"github.com/mtzanidakis/dodo/internal/selfupdate"
	"github.com/mtzanidakis/dodo/internal/tui"
)

// version is injected at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	configPath := flag.String("config", "", "path to config.json")
	url := flag.String("url", "", "API base URL")
	token := flag.String("token", "", "API bearer token")
	help := flag.Bool("h", false, "show help")
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, `dodo-tui - terminal UI client for dodo

Usage:
  dodo-tui [--url <api>] [--token <token>] [--config <path>]
  dodo-tui version    Print the installed version
  dodo-tui upgrade    Update to the latest release if a newer one exists

Flags:
  --url, --token, --config path to config.json (default ~/.config/dodo/config.json)
`)
	}
	flag.Parse()
	if *help {
		flag.Usage()
		return
	}

	switch flag.Arg(0) {
	case "version":
		fmt.Println(selfupdate.DisplayVersion(version))
		return
	case "upgrade":
		if err := selfupdate.Run(os.Stderr, selfupdate.Options{CurrentVersion: version, BinaryName: "dodo-tui"}); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}
	cfg, err := clientconfig.Load(clientconfig.Flags{ConfigPath: *configPath, URL: *url, Token: *token})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if cfg.URL == "" || cfg.Token == "" {
		fmt.Fprintln(os.Stderr, "error: missing url or token (run dodo-cli init or pass --url/--token)")
		os.Exit(5)
	}
	if err := tui.Run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
