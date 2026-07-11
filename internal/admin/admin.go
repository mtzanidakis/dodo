package admin

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/mtzanidakis/dodo/internal/auth"
	"github.com/mtzanidakis/dodo/internal/config"
	"github.com/mtzanidakis/dodo/internal/db"
	"github.com/mtzanidakis/dodo/internal/migrations"
	"github.com/mtzanidakis/dodo/internal/models"
	"github.com/mtzanidakis/dodo/internal/store"
)

var (
	version = "dev"
	commit  = "none"
)

func Run(args []string, ver, com string) int {
	version = ver
	commit = com
	if len(args) == 0 {
		usage()
		return 2
	}
	switch args[0] {
	case "version":
		fmt.Printf("dodo admin %s (%s)\n", version, commit)
		return 0
	case "migrate":
		return runMigrate(args[1:])
	case "user":
		return runUser(args[1:])
	case "token":
		return runToken(args[1:])
	case "-h", "--help", "help":
		usage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", args[0])
		usage()
		return 2
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `dodo admin - direct DB admin tool

Usage:
  dodo admin user create      --email --password [--display-name] [--role=user|admin]
  dodo admin user list
  dodo admin user get         --id | --email
  dodo admin user update      --email [--display-name] [--role] [--active]
  dodo admin user delete      --email
  dodo admin user reset-password --email --password
  dodo admin token create     --email --name   (prints full token once)
  dodo admin token list       --email
  dodo admin token revoke     --id | --prefix
  dodo admin migrate
  dodo admin version

Environment: DODO_DATABASE_PATH (default /data/dodo.sqlite)
`)
}

func openStore() (*store.Store, func(), bool) {
	cfg, err := config.LoadAdmin()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return nil, nil, false
	}
	d, err := db.Open(cfg.DatabasePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return nil, nil, false
	}
	slog.SetLogLoggerLevel(slog.LevelInfo)
	return store.New(d), func() { _ = d.Close() }, true
}

func output(v any, pretty bool) {
	if pretty {
		tablePrint(v)
		return
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func tablePrint(v any) {
	switch list := v.(type) {
	case []map[string]any:
		fmt.Printf("%-26s %-30s %-8s %-10s\n", "ID", "EMAIL", "ROLE", "DISPLAY")
		for _, u := range list {
			fmt.Printf("%-26s %-30s %-8s %-10s\n", u["id"], u["email"], u["role"], u["display_name"])
		}
	case []*models.APIToken:
		fmt.Printf("%-26s %-8s %-16s %-20s\n", "ID", "NAME", "PREFIX", "CREATED")
		for _, t := range list {
			fmt.Printf("%-26s %-8s %-16s %-20s\n", t.ID, t.Name, t.TokenPrefix, t.CreatedAt.Format("2006-01-02"))
		}
	default:
		b, _ := json.MarshalIndent(v, "", "  ")
		fmt.Println(string(b))
	}
}

func runMigrate(args []string) int {
	cfg, err := config.LoadAdmin()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	d, err := db.Open(cfg.DatabasePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	defer func() { _ = d.Close() }()
	ctx := context.Background()
	if err := migrations.Apply(ctx, d); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	applied, err := migrations.Applied(ctx, d)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	output(map[string]any{"ok": true, "action": "migrate", "applied": applied}, false)
	return 0
}

func runUser(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: dodo admin user <create|list|get|update|delete|reset-password>")
		return 2
	}
	switch args[0] {
	case "create":
		return userCreate(args[1:])
	case "list":
		return userList(args[1:])
	case "get":
		return userGet(args[1:])
	case "update":
		return userUpdate(args[1:])
	case "delete":
		return userDelete(args[1:])
	case "reset-password":
		return userResetPassword(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown user subcommand %q\n", args[0])
		return 2
	}
}

func userCreate(args []string) int {
	fs := flag.NewFlagSet("user create", flag.ExitOnError)
	email := fs.String("email", "", "email")
	password := fs.String("password", "", "password")
	displayName := fs.String("display-name", "", "display name")
	role := fs.String("role", "user", "role")
	pretty := fs.Bool("pretty", false, "human table output")
	_ = fs.Parse(args)
	if *email == "" || *password == "" {
		fmt.Fprintln(os.Stderr, "error: --email and --password required")
		return 2
	}
	if len(*password) < 8 {
		fmt.Fprintln(os.Stderr, "error: password must be at least 8 characters")
		return 1
	}
	st, done, ok := openStore()
	if !ok {
		return 1
	}
	defer done()
	ctx := context.Background()

	users, _ := st.Users.List(ctx)
	assignedRole := models.Role(*role)
	if assignedRole == "" {
		assignedRole = models.RoleUser
	}
	if len(users) == 0 && *role == "user" {
		assignedRole = models.RoleAdmin
		fmt.Fprintln(os.Stderr, "note: no users exist; first user promoted to admin")
	}
	hash, err := auth.HashPassword(*password)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	u := &models.User{Email: strings.ToLower(*email), PasswordHash: hash, Role: assignedRole, DisplayName: *displayName}
	if err := st.Users.Create(ctx, u); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	_ = st.Audit.Log(ctx, u.ID, "admin.user.create", "user", u.ID, map[string]any{"email": u.Email, "role": u.Role})
	output(toAdminUser(u), *pretty)
	return 0
}

func userList(args []string) int {
	pretty := false
	for _, a := range args {
		if a == "--pretty" {
			pretty = true
		}
	}
	st, done, ok := openStore()
	if !ok {
		return 1
	}
	defer done()
	users, err := st.Users.List(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	out := make([]map[string]any, 0, len(users))
	for _, u := range users {
		out = append(out, toAdminUser(u))
	}
	if pretty {
		fmt.Printf("%-26s %-30s %-8s %-10s\n", "ID", "EMAIL", "ROLE", "DISPLAY")
		for _, u := range out {
			fmt.Printf("%-26s %-30s %-8s %-10s\n", u["id"], u["email"], u["role"], u["display_name"])
		}
		return 0
	}
	output(out, false)
	return 0
}

func userGet(args []string) int {
	fs := flag.NewFlagSet("user get", flag.ExitOnError)
	id := fs.String("id", "", "user id")
	email := fs.String("email", "", "email")
	_ = fs.Parse(args)
	st, done, ok := openStore()
	if !ok {
		return 1
	}
	defer done()
	ctx := context.Background()
	var u *models.User
	var err error
	switch {
	case *id != "":
		u, err = st.Users.GetByID(ctx, *id)
	case *email != "":
		u, err = st.Users.GetByEmail(ctx, strings.ToLower(*email))
	default:
		u, err = st.Users.GetByEmail(ctx, "")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 4
	}
	_ = err
	output(toAdminUser(u), false)
	return 0
}

func userUpdate(args []string) int {
	fs := flag.NewFlagSet("user update", flag.ExitOnError)
	email := fs.String("email", "", "email")
	displayName := fs.String("display-name", "", "display name")
	role := fs.String("role", "", "role")
	active := fs.String("active", "", "active: true|false")
	yes := fs.Bool("yes", false, "skip confirmation")
	_ = fs.Parse(args)
	if *email == "" {
		fmt.Fprintln(os.Stderr, "error: --email required")
		return 2
	}
	st, done, ok := openStore()
	if !ok {
		return 1
	}
	defer done()
	ctx := context.Background()
	u, err := st.Users.GetByEmail(ctx, strings.ToLower(*email))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 4
	}
	if *displayName != "" {
		u.DisplayName = *displayName
	}
	if *role != "" {
		r, err := models.ParseRole(*role)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		u.Role = r
	}
	if *active == "false" && u.DeletedAt == nil {
		if !*yes {
			fmt.Fprintln(os.Stderr, "pass --yes to confirm soft delete")
			return 1
		}
		if err := st.Users.SoftDelete(ctx, u.ID); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
	} else if err := st.Users.Update(ctx, u); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	_ = st.Audit.Log(ctx, u.ID, "admin.user.update", "user", u.ID, nil)
	output(toAdminUser(u), false)
	return 0
}

func userDelete(args []string) int {
	fs := flag.NewFlagSet("user delete", flag.ExitOnError)
	email := fs.String("email", "", "email")
	yes := fs.Bool("yes", false, "skip confirmation")
	_ = fs.Parse(args)
	if *email == "" {
		fmt.Fprintln(os.Stderr, "error: --email required")
		return 2
	}
	if !*yes {
		fmt.Fprintln(os.Stderr, "pass --yes to confirm deletion")
		return 1
	}
	st, done, ok := openStore()
	if !ok {
		return 1
	}
	defer done()
	ctx := context.Background()
	u, err := st.Users.GetByEmail(ctx, strings.ToLower(*email))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 4
	}
	if err := st.Users.SoftDelete(ctx, u.ID); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	_ = st.Audit.Log(ctx, u.ID, "admin.user.delete", "user", u.ID, nil)
	output(map[string]any{"ok": true, "deleted": u.ID}, false)
	return 0
}

func userResetPassword(args []string) int {
	fs := flag.NewFlagSet("user reset-password", flag.ExitOnError)
	email := fs.String("email", "", "email")
	password := fs.String("password", "", "new password")
	_ = fs.Parse(args)
	if *email == "" || *password == "" {
		fmt.Fprintln(os.Stderr, "error: --email and --password required")
		return 2
	}
	if len(*password) < 8 {
		fmt.Fprintln(os.Stderr, "error: password must be at least 8 characters")
		return 1
	}
	st, done, ok := openStore()
	if !ok {
		return 1
	}
	defer done()
	ctx := context.Background()
	u, err := st.Users.GetByEmail(ctx, strings.ToLower(*email))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 4
	}
	hash, err := auth.HashPassword(*password)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if err := st.Users.UpdatePassword(ctx, u.ID, hash); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	_ = st.Audit.Log(ctx, u.ID, "admin.password.reset", "user", u.ID, nil)
	output(map[string]any{"ok": true}, false)
	return 0
}

func runToken(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: dodo admin token <create|list|revoke>")
		return 2
	}
	switch args[0] {
	case "create":
		return tokenCreate(args[1:])
	case "list":
		return tokenList(args[1:])
	case "revoke":
		return tokenRevoke(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown token subcommand %q\n", args[0])
		return 2
	}
}

func tokenCreate(args []string) int {
	fs := flag.NewFlagSet("token create", flag.ExitOnError)
	email := fs.String("email", "", "user email")
	name := fs.String("name", "", "token name")
	_ = fs.Parse(args)
	if *email == "" || *name == "" {
		fmt.Fprintln(os.Stderr, "error: --email and --name required")
		return 2
	}
	st, done, ok := openStore()
	if !ok {
		return 1
	}
	defer done()
	ctx := context.Background()
	u, err := st.Users.GetByEmail(ctx, strings.ToLower(*email))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 4
	}
	gen, err := auth.GenerateAPIToken()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	t, err := st.Tokens.Create(ctx, u.ID, *name, gen.Prefix, gen.Hash)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	_ = st.Audit.Log(ctx, u.ID, "admin.token.create", "api_token", t.ID, map[string]any{"name": *name})
	output(map[string]any{"id": t.ID, "name": t.Name, "prefix": t.TokenPrefix, "token": gen.Full, "created_at": t.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00")}, false)
	return 0
}

func tokenList(args []string) int {
	fs := flag.NewFlagSet("token list", flag.ExitOnError)
	email := fs.String("email", "", "user email")
	pretty := fs.Bool("pretty", false, "human table")
	_ = fs.Parse(args)
	if *email == "" {
		fmt.Fprintln(os.Stderr, "error: --email required")
		return 2
	}
	st, done, ok := openStore()
	if !ok {
		return 1
	}
	defer done()
	u, err := st.Users.GetByEmail(context.Background(), strings.ToLower(*email))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 4
	}
	tokens, err := st.Tokens.List(context.Background(), u.ID)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	out := make([]map[string]any, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, map[string]any{
			"id": t.ID, "name": t.Name, "prefix": t.TokenPrefix, "created_at": t.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	output(out, *pretty)
	return 0
}

func tokenRevoke(args []string) int {
	fs := flag.NewFlagSet("token revoke", flag.ExitOnError)
	id := fs.String("id", "", "token id")
	prefix := fs.String("prefix", "", "token prefix")
	yes := fs.Bool("yes", false, "skip confirmation")
	_ = fs.Parse(args)
	if *id == "" && *prefix == "" {
		fmt.Fprintln(os.Stderr, "error: --id or --prefix required")
		return 2
	}
	if !*yes {
		fmt.Fprintln(os.Stderr, "pass --yes to confirm revocation")
		return 1
	}
	st, done, ok := openStore()
	if !ok {
		return 1
	}
	defer done()
	ctx := context.Background()
	var targetID string
	if *id != "" {
		targetID = *id
	} else {
		tok, err := st.Tokens.GetByPrefix(ctx, *prefix)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error: no active token with that prefix")
			return 1
		}
		targetID = tok.ID
	}
	if err := st.Tokens.Purge(ctx, targetID); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	_ = st.Audit.Log(ctx, "", "admin.token.revoke", "api_token", targetID, nil)
	output(map[string]any{"ok": true}, false)
	return 0
}

func toAdminUser(u *models.User) map[string]any {
	return map[string]any{
		"id":           u.ID,
		"email":        u.Email,
		"role":         u.Role,
		"display_name": u.DisplayName,
		"timezone":     u.Timezone,
		"locale":       u.Locale,
		"theme":        u.Theme,
		"created_at":   u.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		"deleted":      u.Deleted(),
	}
}

var _ = errors.New
var _ io.Reader = strings.NewReader("")
