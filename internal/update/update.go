// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package update

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Repository is where the releases are published.
const Repository = "otnc/Volm"

// apiBase is a variable so that tests can point it at a local server.
var apiBase = "https://api.github.com"

// Release is a published version with its installer.
type Release struct {
	Version    Version
	Tag        string
	Notes      string
	Prerelease bool
	// InstallerURL and ChecksumsURL are the download links of Volm-Setup-<version>.exe and SHA256SUMS.
	InstallerURL, ChecksumsURL string
	installerName              string
}

// ErrNoRelease reports that GitHub has no release to offer: none published yet, or the repository is not public.
var ErrNoRelease = errors.New("update: no release found")

// ErrChecksum reports a download whose SHA-256 differs from SHA256SUMS.
var ErrChecksum = errors.New("update: the download does not match SHA256SUMS")

var client = &http.Client{Timeout: 30 * time.Second}

type ghRelease struct {
	TagName    string `json:"tag_name"`
	Body       string `json:"body"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// Latest returns the newest release: the latest stable one, or with prerelease the newest of all, prereleases included.
//
//	rel, err := update.Latest(ctx, false)
//	if err == nil && rel.Version.Compare(current) > 0 {
//		// offer rel
//	}
func Latest(ctx context.Context, prerelease bool) (Release, error) {
	var list []ghRelease
	if prerelease {
		if err := getJSON(ctx, "/repos/"+Repository+"/releases?per_page=20", &list); err != nil {
			return Release{}, err
		}
	} else {
		var one ghRelease
		if err := getJSON(ctx, "/repos/"+Repository+"/releases/latest", &one); err != nil {
			return Release{}, err
		}
		list = []ghRelease{one}
	}
	var best Release
	found := false
	for _, r := range list {
		if r.Draft {
			continue
		}
		rel, err := fromGitHub(r)
		if err != nil {
			continue
		}
		if !found || rel.Version.Compare(best.Version) > 0 {
			best, found = rel, true
		}
	}
	if !found {
		return Release{}, ErrNoRelease
	}
	return best, nil
}

func fromGitHub(r ghRelease) (Release, error) {
	v, err := ParseVersion(r.TagName)
	if err != nil {
		return Release{}, err
	}
	rel := Release{Version: v, Tag: r.TagName, Notes: r.Body, Prerelease: r.Prerelease, installerName: "Volm-Setup-" + v.String() + ".exe"}
	for _, a := range r.Assets {
		switch a.Name {
		case rel.installerName:
			rel.InstallerURL = a.URL
		case "SHA256SUMS":
			rel.ChecksumsURL = a.URL
		}
	}
	if rel.InstallerURL == "" || rel.ChecksumsURL == "" {
		return Release{}, fmt.Errorf("update: %s has no installer or SHA256SUMS", r.TagName)
	}
	return rel, nil
}

func getJSON(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+path, http.NoBody)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Volm-update-check")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return ErrNoRelease
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("update: GitHub answered %s", resp.Status)
	}
	return json.UnmarshalRead(resp.Body, v)
}

// Download fetches the release's installer and SHA256SUMS into a new temporary directory and checks the installer against its line in SHA256SUMS. The check catches a broken download; it cannot catch a release that was tampered with, as both files come from the same place.
func Download(ctx context.Context, rel Release) (installer string, err error) {
	dir, err := os.MkdirTemp("", "Volm-update-")
	if err != nil {
		return "", err
	}
	installer = filepath.Join(dir, rel.installerName)
	sums := filepath.Join(dir, "SHA256SUMS")
	if err := download(ctx, rel.InstallerURL, installer); err != nil {
		return "", err
	}
	if err := download(ctx, rel.ChecksumsURL, sums); err != nil {
		return "", err
	}
	want, err := checksumFor(sums, rel.installerName)
	if err != nil {
		return "", err
	}
	got, err := fileSHA256(installer)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(got, want) {
		return "", ErrChecksum
	}
	return installer, nil
}

func download(ctx context.Context, url, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Volm-update-check")
	resp, err := (&http.Client{Timeout: 10 * time.Minute}).Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("update: downloading %s: %s", url, resp.Status)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		return errors.Join(err, f.Close())
	}
	return f.Close()
}

// checksumFor finds the hash of name in a file of "<sha256>  <name>" lines, as sha256sum writes.
func checksumFor(sumsPath, name string) (string, error) {
	f, err := os.Open(sumsPath)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	s := bufio.NewScanner(f)
	for s.Scan() {
		fields := strings.Fields(s.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			return fields[0], nil
		}
	}
	if err := s.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("update: SHA256SUMS lists no %s", name)
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// RunInstaller starts the installer silently and returns without waiting. The installer asks the running Volm to quit, replaces it, and starts the new one.
func RunInstaller(path string) error {
	cmd := exec.Command(path, "/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART")
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// DueForCheck reports whether the automatic check at start should run: at most once a day.
func DueForCheck(last, now time.Time) bool {
	return last.IsZero() || now.Sub(last) >= 24*time.Hour
}
