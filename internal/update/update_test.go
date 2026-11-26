// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package update

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestCompare(t *testing.T) {
	ordered := []string{"0.9.9", "1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-beta", "1.0.0-rc.1", "1.0.0-rc.2", "1.0.0-rc.10", "1.0.0", "1.0.1", "1.1.0", "2.0.0"}
	for i := range len(ordered) - 1 {
		a, _ := ParseVersion(ordered[i])
		b, _ := ParseVersion(ordered[i+1])
		if a.Compare(b) >= 0 || b.Compare(a) <= 0 {
			t.Errorf("%s should come before %s", ordered[i], ordered[i+1])
		}
	}
	a, _ := ParseVersion("v1.2.3")
	b, _ := ParseVersion("1.2.3")
	if a.Compare(b) != 0 || a.String() != "1.2.3" {
		t.Errorf("v1.2.3 = %v, want equal to 1.2.3", a)
	}
}

func TestParseVersionRejects(t *testing.T) {
	for _, s := range []string{"", "1.2", "1.2.3.4", "1.x.3", "1.2.3-", "-1.0.0", "dev"} {
		if v, err := ParseVersion(s); err == nil {
			t.Errorf("ParseVersion(%q) = %v, want an error", s, v)
		}
	}
}

func TestDueForCheck(t *testing.T) {
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	if !DueForCheck(time.Time{}, now) {
		t.Error("never checked: want due")
	}
	if DueForCheck(now.Add(-23*time.Hour), now) {
		t.Error("checked 23h ago: want not due")
	}
	if !DueForCheck(now.Add(-24*time.Hour), now) {
		t.Error("checked 24h ago: want due")
	}
}

// fakeGitHub serves a release list and the files of one release.
func fakeGitHub(t *testing.T, installer []byte, sums string) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		release := func(tag string, pre bool) string {
			v := tag[1:]
			return fmt.Sprintf(`{"tag_name":%q,"body":"notes of %s","prerelease":%v,"assets":[{"name":"Volm-Setup-%s.exe","browser_download_url":"%s/dl/setup"},{"name":"SHA256SUMS","browser_download_url":"%s/dl/sums"}]}`, tag, tag, pre, v, srv.URL, srv.URL)
		}
		switch r.URL.Path {
		case "/repos/otnc/Volm/releases/latest":
			_, _ = fmt.Fprint(w, release("v1.2.0", false))
		case "/repos/otnc/Volm/releases":
			_, _ = fmt.Fprintf(w, "[%s,%s,%s]", release("v1.2.0", false), release("v1.3.0-rc.1", true), `{"tag_name":"v9.0.0","draft":true,"assets":[]}`)
		case "/dl/setup":
			_, _ = w.Write(installer)
		case "/dl/sums":
			_, _ = fmt.Fprint(w, sums)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	apiBase = srv.URL
	t.Cleanup(func() { apiBase = "https://api.github.com" })
	return srv
}

func TestLatestAndDownload(t *testing.T) {
	installer := []byte("installer bytes")
	sum := sha256.Sum256(installer)
	fakeGitHub(t, installer, hex.EncodeToString(sum[:])+"  Volm-Setup-1.2.0.exe\n")

	rel, err := Latest(t.Context(), false)
	if err != nil || rel.Version.String() != "1.2.0" || rel.Notes != "notes of v1.2.0" {
		t.Fatalf("Latest stable = %+v, %v", rel, err)
	}
	pre, err := Latest(t.Context(), true)
	if err != nil || pre.Version.String() != "1.3.0-rc.1" {
		t.Fatalf("Latest with prereleases = %+v, %v; want 1.3.0-rc.1 and no draft", pre, err)
	}

	path, err := Download(t.Context(), rel)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(installer) {
		t.Fatalf("downloaded %q, %v", got, err)
	}
}

func TestDownloadRejectsChecksumMismatch(t *testing.T) {
	fakeGitHub(t, []byte("installer bytes"), "0000000000000000000000000000000000000000000000000000000000000000  Volm-Setup-1.2.0.exe\n")
	rel, err := Latest(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Download(t.Context(), rel); !errors.Is(err, ErrChecksum) {
		t.Fatalf("err = %v, want ErrChecksum", err)
	}
}

func TestLatestWithoutReleases(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	apiBase = srv.URL
	defer func() { apiBase = "https://api.github.com" }()
	if _, err := Latest(t.Context(), false); !errors.Is(err, ErrNoRelease) {
		t.Fatalf("err = %v, want ErrNoRelease", err)
	}
}
