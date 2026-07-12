package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"v0.2.0", "v0.2.1", true},
		{"0.2.0", "0.3.0", true},
		{"v1.0.0", "v1.0.0", false},
		{"v0.2.1", "v0.2.0", false},
		{"dev", "v0.2.0", true},               // unparseable current -> upgrade
		{"v0.2.0-3-gabc123", "v0.2.0", false}, // local git build == release
		{"v0.2.0-3-gabc123", "v0.2.1", true},  // still upgrades to a newer release
		{"v0.2.0", "not-a-version", false},    // unparseable latest -> no upgrade
	}
	for _, c := range cases {
		if got := newer(c.current, c.latest); got != c.want {
			t.Errorf("newer(%q, %q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}

// tarGz builds a .tar.gz archive containing a single regular file.
func tarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// releaseServer serves a GitHub-like release plus its archive and checksums.
func releaseServer(t *testing.T, tag, archiveName string, archive []byte) *httptest.Server {
	t.Helper()
	sum := sha256.Sum256(archive)
	checksums := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), archiveName)

	mux := http.NewServeMux()
	var base string
	mux.HandleFunc("/repos/owner/repo/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		rel := ghRelease{
			TagName: tag,
			Assets: []ghAsset{
				{Name: archiveName, URL: base + "/dl/" + archiveName},
				{Name: "checksums.txt", URL: base + "/dl/checksums.txt"},
			},
		}
		_ = json.NewEncoder(w).Encode(rel)
	})
	mux.HandleFunc("/dl/"+archiveName, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(archive) })
	mux.HandleFunc("/dl/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(checksums))
	})
	srv := httptest.NewServer(mux)
	base = srv.URL
	t.Cleanup(srv.Close)
	return srv
}

func TestRunUpgradesInPlace(t *testing.T) {
	archiveName := "dodo-dodo-cli_9.9.9_linux_amd64.tar.gz"
	newContent := []byte("#!/bin/sh\necho new-binary\n")
	archive := tarGz(t, "dodo-cli", newContent)
	srv := releaseServer(t, "v9.9.9", archiveName, archive)

	// A stand-in "installed" binary that should get replaced.
	exe := filepath.Join(t.TempDir(), "dodo-cli")
	if err := os.WriteFile(exe, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err := Run(&out, Options{
		CurrentVersion: "v0.2.0",
		BinaryName:     "dodo-cli",
		Repo:           "owner/repo",
		APIBase:        srv.URL,
		GOOS:           "linux",
		GOARCH:         "amd64",
		ExePath:        exe,
	})
	if err != nil {
		t.Fatalf("Run: %v (out: %s)", err, out.String())
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, newContent) {
		t.Fatalf("binary not replaced: got %q", got)
	}
	info, err := os.Stat(exe)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("replaced binary not executable: %v", info.Mode())
	}
}

func TestRunAlreadyLatest(t *testing.T) {
	archiveName := "dodo-dodo-cli_9.9.9_linux_amd64.tar.gz"
	archive := tarGz(t, "dodo-cli", []byte("new"))
	srv := releaseServer(t, "v0.2.0", archiveName, archive)

	exe := filepath.Join(t.TempDir(), "dodo-cli")
	orig := []byte("current-binary")
	if err := os.WriteFile(exe, orig, 0o755); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err := Run(&out, Options{
		CurrentVersion: "v0.2.0",
		BinaryName:     "dodo-cli",
		Repo:           "owner/repo",
		APIBase:        srv.URL,
		GOOS:           "linux",
		GOARCH:         "amd64",
		ExePath:        exe,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, _ := os.ReadFile(exe); !bytes.Equal(got, orig) {
		t.Fatalf("binary changed when already current: %q", got)
	}
	if !bytes.Contains(out.Bytes(), []byte("already up to date")) {
		t.Fatalf("expected up-to-date message, got: %s", out.String())
	}
}

func TestRunChecksumMismatch(t *testing.T) {
	archiveName := "dodo-dodo-cli_9.9.9_linux_amd64.tar.gz"
	archive := tarGz(t, "dodo-cli", []byte("payload"))

	// A release whose checksums.txt carries a deliberately wrong hash.
	mux := http.NewServeMux()
	var base string
	mux.HandleFunc("/repos/owner/repo/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(ghRelease{
			TagName: "v9.9.9",
			Assets: []ghAsset{
				{Name: archiveName, URL: base + "/dl/" + archiveName},
				{Name: "checksums.txt", URL: base + "/dl/checksums.txt"},
			},
		})
	})
	mux.HandleFunc("/dl/"+archiveName, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(archive) })
	mux.HandleFunc("/dl/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("deadbeef  " + archiveName + "\n"))
	})
	srv := httptest.NewServer(mux)
	base = srv.URL
	t.Cleanup(srv.Close)

	exe := filepath.Join(t.TempDir(), "dodo-cli")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err := Run(&out, Options{
		CurrentVersion: "v0.2.0",
		BinaryName:     "dodo-cli",
		Repo:           "owner/repo",
		APIBase:        srv.URL,
		GOOS:           "linux",
		GOARCH:         "amd64",
		ExePath:        exe,
	})
	if err == nil {
		t.Fatal("expected checksum mismatch error, got nil")
	}
	if got, _ := os.ReadFile(exe); string(got) != "old" {
		t.Fatalf("binary should be untouched on checksum failure: %q", got)
	}
}
