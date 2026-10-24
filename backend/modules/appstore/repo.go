package appstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/armor"
)

// RepoDef describes a vendor apt repository that carries versions the
// distribution does not. The signing key is downloaded over HTTPS and only
// accepted when its fingerprint matches one pinned here.
type RepoDef struct {
	Name         string // shown in the confirmation dialog
	File         string // stem of the key and sources file RPanel writes
	Fingerprints []string
	Key          func(osID string) string
	// Source returns "<uri> <suite> <component>" or false when the OS is unsupported.
	Source func(osID, codename, label string) (string, bool)
}

var phpRepo = &RepoDef{
	Name: "Ondřej Surý PHP repository", File: "php",
	Fingerprints: []string{
		"B8DC7E53946656EFBCE4C1DD71DAEAAB4AD4CAB6", // Launchpad PPA ondrej/php
		"15058500A0235D97F5D10063B188E2B695BD4743", // packages.sury.org (Debian)
	},
	Key: func(osID string) string {
		if osID == "debian" {
			return "https://packages.sury.org/php/apt.gpg"
		}
		return "https://keyserver.ubuntu.com/pks/lookup?op=get&search=0x71DAEAAB4AD4CAB6"
	},
	Source: func(osID, codename, _ string) (string, bool) {
		switch osID {
		case "ubuntu":
			return "https://ppa.launchpadcontent.net/ondrej/php/ubuntu " + codename + " main", true
		case "debian":
			return "https://packages.sury.org/php/ " + codename + " main", true
		}
		return "", false
	},
}

var nodeRepo = &RepoDef{
	Name: "NodeSource", File: "nodejs",
	Fingerprints: []string{"6F71F525282841EEDAF851B42F59B5F99B1BE0B4"},
	Key:          func(string) string { return "https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key" },
	Source: func(_, _, label string) (string, bool) {
		return "https://deb.nodesource.com/node_" + label + ".x nodistro main", true
	},
}

var mariadbRepo = &RepoDef{
	Name: "MariaDB Foundation", File: "mariadb",
	Fingerprints: []string{"177F4010FE56CA3336300305F1656F24C74CD1D8"},
	Key:          func(string) string { return "https://mariadb.org/mariadb_release_signing_key.pgp" },
	Source: func(osID, codename, label string) (string, bool) {
		if osID != "ubuntu" && osID != "debian" {
			return "", false
		}
		return "https://deb.mariadb.org/" + label + "/" + osID + " " + codename + " main", true
	},
}

func phpPackages(v string, _ bool) []string {
	p := "php" + v + "-"
	out := make([]string, 0, 11)
	for _, x := range []string{"fpm", "cli", "mysql", "pgsql", "curl", "mbstring", "xml", "zip", "gd", "intl", "bcmath"} {
		out = append(out, p+x)
	}
	return out
}

// osRelease returns the distribution ID and codename used to pick repository suites.
func osRelease() (id, codename string) {
	b, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "", ""
	}
	kv := map[string]string{}
	for _, l := range strings.Split(string(b), "\n") {
		if k, v, ok := strings.Cut(l, "="); ok {
			kv[k] = strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	codename = kv["VERSION_CODENAME"]
	if kv["ID"] == "ubuntu" && kv["UBUNTU_CODENAME"] != "" {
		codename = kv["UBUNTU_CODENAME"]
	}
	return kv["ID"], codename
}

const maxKeyBytes = 1 << 20

func httpFetch(ctx context.Context, rawURL string) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" {
		return nil, fmt.Errorf("refusing non-https key URL")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxKeyBytes))
}

var errKeyMismatch = errors.New("signing key fingerprint does not match the pinned one")

// checkKey accepts the key only if one of its primary keys is pinned and
// reports whether it is ASCII-armored (apt needs a matching file extension).
func checkKey(data []byte, pinned []string) (armored bool, err error) {
	var ring openpgp.EntityList
	if blk, aerr := armor.Decode(bytes.NewReader(data)); aerr == nil {
		armored = true
		ring, err = openpgp.ReadKeyRing(blk.Body)
	} else {
		ring, err = openpgp.ReadKeyRing(bytes.NewReader(data))
	}
	if err != nil {
		return false, fmt.Errorf("unreadable signing key: %w", err)
	}
	for _, e := range ring {
		got := fmt.Sprintf("%X", e.PrimaryKey.Fingerprint)
		for _, want := range pinned {
			if got == want {
				return armored, nil
			}
		}
	}
	return false, errKeyMismatch
}

// setupRepo downloads and verifies the repository key, then writes the key
// and a sources file. It replaces any earlier file of the same app so only
// one version stream is active at a time.
func (s *Service) setupRepo(ctx context.Context, a *App, label string) error {
	osID, codename := s.osInfo()
	src, ok := a.Repo.Source(osID, codename, label)
	if !ok || codename == "" {
		return fmt.Errorf("%s does not support %s %s", a.Repo.Name, osID, codename)
	}
	data, err := s.fetch(ctx, a.Repo.Key(osID))
	if err != nil {
		return fmt.Errorf("cannot download the %s signing key: %w", a.Repo.Name, err)
	}
	armored, err := checkKey(data, a.Repo.Fingerprints)
	if err != nil {
		return fmt.Errorf("%s: %w", a.Repo.Name, err)
	}
	ext := ".gpg"
	if armored {
		ext = ".asc"
	}
	keyDir := filepath.Join(s.aptDir, "keyrings")
	listDir := filepath.Join(s.aptDir, "sources.list.d")
	for _, d := range []string{keyDir, listDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	for _, old := range []string{".gpg", ".asc"} {
		_ = os.Remove(filepath.Join(keyDir, "rpanel-"+a.Repo.File+old))
	}
	keyPath := filepath.Join(keyDir, "rpanel-"+a.Repo.File+ext)
	if err := os.WriteFile(keyPath, data, 0o644); err != nil {
		return err
	}
	line := fmt.Sprintf("# Managed by RPanel App Store\ndeb [signed-by=%s] %s\n", keyPath, src)
	return os.WriteFile(filepath.Join(listDir, "rpanel-"+a.Repo.File+".list"), []byte(line), 0o644)
}
