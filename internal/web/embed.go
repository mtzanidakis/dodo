package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

//go:embed all:templates
var templatesFS embed.FS

//go:embed dist/version.txt
var versionFile embed.FS

func AssetsFS() (fs.FS, string) {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil, ""
	}
	v := versionString()
	return sub, v
}

func versionString() string {
	b, err := versionFile.ReadFile("dist/version.txt")
	if err != nil {
		return "dev"
	}
	if len(b) == 0 {
		return "dev"
	}
	v := string(b)
	if len(v) > 0 && v[len(v)-1] == '\n' {
		v = v[:len(v)-1]
	}
	return v
}

// TemplatesFS returns the embedded templates filesystem.
func TemplatesFS() (fs.FS, error) {
	return fs.Sub(templatesFS, "templates")
}
