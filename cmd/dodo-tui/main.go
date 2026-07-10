package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	configPath := flag.String("config", "", "path to config.json")
	url := flag.String("url", "", "API base URL")
	token := flag.String("token", "", "API bearer token")
	help := flag.Bool("h", false, "show help")
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, `dodo-tui - terminal UI client for dodo

Usage:
  dodo-tui [--url <api>] [--token <token>] [--config <path>]

Flags:
  --url
  --token
  --config path to config.json (default ~/.config/dodo/config.json)
`)
	}
	flag.Parse()
	_ = configPath
	_ = url
	_ = token
	if *help {
		flag.Usage()
		return
	}
	fmt.Fprintln(os.Stderr, "not implemented")
	os.Exit(1)
}
