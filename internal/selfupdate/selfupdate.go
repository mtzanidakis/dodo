// Package selfupdate lets the dodo-cli and dodo-tui binaries upgrade
// themselves in place from the project's GitHub releases: it compares the
// installed version against the latest release, and when a newer one exists it
// downloads the matching archive, verifies its checksum, and atomically
// replaces the running executable.
package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	defaultRepo    = "mtzanidakis/dodo"
	defaultAPIBase = "https://api.github.com"
)

// Options configures an upgrade run. Zero-value fields fall back to sensible
// defaults: the real GitHub API, the running executable, and the host os/arch.
// The overridable fields exist so tests can point the whole flow at a local
// server and a temp file.
type Options struct {
	CurrentVersion string // installed version, e.g. "v0.2.0" or "dev"
	BinaryName     string // "dodo-cli" or "dodo-tui"
	Repo           string // "owner/name"
	APIBase        string // GitHub API base URL
	HTTPClient     *http.Client
	GOOS           string
	GOARCH         string
	ExePath        string // executable to replace
}

type ghAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

// Run checks the latest release and, if it is newer than the installed
// version, downloads the matching archive and replaces the running binary.
// Progress is written to out. It is a no-op (returning nil) when already
// current.
func Run(out io.Writer, opts Options) error {
	opts.applyDefaults()
	if opts.BinaryName == "" {
		return fmt.Errorf("selfupdate: binary name required")
	}

	rel, err := opts.latestRelease()
	if err != nil {
		return err
	}
	latest := strings.TrimSpace(rel.TagName)
	if latest == "" {
		return fmt.Errorf("no released version found")
	}
	_, _ = fmt.Fprintf(out, "installed %s, latest %s\n", DisplayVersion(opts.CurrentVersion), latest)
	if !newer(opts.CurrentVersion, latest) {
		_, _ = fmt.Fprintln(out, "already up to date")
		return nil
	}
	if opts.ExePath == "" {
		return fmt.Errorf("cannot locate the running executable to replace")
	}

	archive, ok := findAsset(rel, opts.BinaryName, opts.GOOS, opts.GOARCH)
	if !ok {
		return fmt.Errorf("no release asset for %s %s/%s", opts.BinaryName, opts.GOOS, opts.GOARCH)
	}
	_, _ = fmt.Fprintf(out, "downloading %s\n", archive.Name)
	data, err := opts.download(archive.URL)
	if err != nil {
		return err
	}
	if sums, ok := findChecksums(rel); ok {
		if err := opts.verifyChecksum(sums.URL, archive.Name, data); err != nil {
			return err
		}
	}
	bin, err := extractBinary(data, opts.BinaryName)
	if err != nil {
		return err
	}
	if err := replaceExecutable(opts.ExePath, bin, opts.GOOS); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "upgraded %s to %s\n", opts.BinaryName, latest)
	return nil
}

func (o *Options) applyDefaults() {
	if o.Repo == "" {
		o.Repo = defaultRepo
	}
	if o.APIBase == "" {
		o.APIBase = defaultAPIBase
	}
	if o.HTTPClient == nil {
		o.HTTPClient = &http.Client{Timeout: 60 * time.Second}
	}
	if o.GOOS == "" {
		o.GOOS = runtime.GOOS
	}
	if o.GOARCH == "" {
		o.GOARCH = runtime.GOARCH
	}
	if o.ExePath == "" {
		if p, err := os.Executable(); err == nil {
			if rp, err := filepath.EvalSymlinks(p); err == nil {
				p = rp
			}
			o.ExePath = p
		}
	}
}

func (o *Options) latestRelease() (*ghRelease, error) {
	url := strings.TrimRight(o.APIBase, "/") + "/repos/" + o.Repo + "/releases/latest"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", o.BinaryName)
	resp, err := o.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("no published release found for %s", o.Repo)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github releases: status %d", resp.StatusCode)
	}
	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decode release: %w", err)
	}
	return &rel, nil
}

func (o *Options) download(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", o.BinaryName)
	resp, err := o.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: status %d", path.Base(url), resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (o *Options) verifyChecksum(sumsURL, archiveName string, data []byte) error {
	raw, err := o.download(sumsURL)
	if err != nil {
		return err
	}
	want := ""
	for _, line := range strings.Split(string(raw), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[1] == archiveName {
			want = f[0]
			break
		}
	}
	if want == "" {
		return nil // no checksum entry for this asset; skip verification
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); !strings.EqualFold(got, want) {
		return fmt.Errorf("checksum mismatch for %s", archiveName)
	}
	return nil
}

// findAsset picks the release archive for the given binary and platform. The
// goreleaser name template embeds the binary, os and arch, so a substring
// match uniquely identifies it (e.g. "dodo-cli" does not match a
// "dodo-dodo-tui_..." asset).
func findAsset(rel *ghRelease, binary, goos, goarch string) (ghAsset, bool) {
	for _, a := range rel.Assets {
		n := a.Name
		if strings.HasSuffix(n, ".tar.gz") &&
			strings.Contains(n, binary) &&
			strings.Contains(n, goos) &&
			strings.Contains(n, goarch) {
			return a, true
		}
	}
	return ghAsset{}, false
}

func findChecksums(rel *ghRelease) (ghAsset, bool) {
	for _, a := range rel.Assets {
		if a.Name == "checksums.txt" {
			return a, true
		}
	}
	return ghAsset{}, false
}

// extractBinary returns the named executable from a .tar.gz archive, tolerating
// a ".exe" suffix on Windows builds.
func extractBinary(archive []byte, binary string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("gunzip: %w", err)
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read archive: %w", err)
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		base := filepath.Base(h.Name)
		if base == binary || base == binary+".exe" {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("binary %q not found in archive", binary)
}

// replaceExecutable writes the new binary next to the current one and renames
// it into place. Writing to the same directory keeps the rename atomic (same
// filesystem); on Windows the running file is moved aside first since it cannot
// be overwritten while executing.
func replaceExecutable(exePath string, newBin []byte, goos string) error {
	dir := filepath.Dir(exePath)
	tmp, err := os.CreateTemp(dir, ".dodo-upgrade-*")
	if err != nil {
		return fmt.Errorf("create temp in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(newBin); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return err
	}
	if goos == "windows" {
		_ = os.Rename(exePath, exePath+".old")
	}
	if err := os.Rename(tmpName, exePath); err != nil {
		return fmt.Errorf("replace %s: %w", exePath, err)
	}
	return nil
}

// DisplayVersion returns v, or "dev" when it is empty (an un-injected build).
func DisplayVersion(v string) string {
	if v = strings.TrimSpace(v); v != "" {
		return v
	}
	return "dev"
}

// newer reports whether latest's numeric core (MAJOR.MINOR.PATCH) is strictly
// greater than current's. An unparseable current (e.g. "dev") is treated as
// upgradeable; an unparseable latest is never an upgrade. Any pre-release or
// git-describe suffix is ignored, so a local "v0.2.0-3-gabc" build is
// considered equal to the v0.2.0 release and will not downgrade.
func newer(current, latest string) bool {
	lc, ok := parseCore(latest)
	if !ok {
		return false
	}
	cc, ok := parseCore(current)
	if !ok {
		return true
	}
	for i := 0; i < 3; i++ {
		if lc[i] != cc[i] {
			return lc[i] > cc[i]
		}
	}
	return false
}

func parseCore(v string) ([3]int, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	var out [3]int
	parts := strings.Split(v, ".")
	if parts[0] == "" {
		return out, false
	}
	for i := 0; i < 3 && i < len(parts); i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
