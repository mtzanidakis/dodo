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
	pretty := flag.Bool("pretty", false, "pretty human-readable output")
	help := flag.Bool("h", false, "show help")
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, `dodo-cli - AI-agent CLI client for dodo

Usage:
  dodo-cli [--url <api>] [--token <token>] [--config <path>] [--pretty] <command> [args]

Commands:
  init              Write a minimal config.json
  me                Print the authenticated user profile
  tasks list        List tasks
  tasks get <id>    Show a single task
  tasks create      Create a task
  tasks update <id> Update a task
  tasks complete <id>
  tasks snooze <id> --until <time>
  tasks delete <id>
  completions list
  tokens list
  tokens create --name <name>
  tokens revoke <id>

Flags:
  --url
  --token
  --config path to config.json (default ~/.config/dodo/config.json)
  --pretty      human-readable output (default JSON)
`)
	}
	flag.Parse()
	_ = configPath
	_ = url
	_ = token
	_ = pretty
	if *help {
		flag.Usage()
		return
	}
	fmt.Fprintln(os.Stderr, "not implemented")
	os.Exit(1)
}
