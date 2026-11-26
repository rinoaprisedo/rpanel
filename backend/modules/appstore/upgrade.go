package appstore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"rpanel/backend/system"
)

// backupDir holds the nginx configuration snapshots taken before every upgrade.
var backupDir = "/var/backups/rpanel"

const keepBackups = 5

var aptEnv = []string{"NEEDRESTART_MODE=l", "NEEDRESTART_SUSPEND=1"}

// settle is how long nginx gets to come up after a restart before it is judged.
var settle = 2 * time.Second

// policyVersions returns the Installed and Candidate fields of apt-cache policy, untouched
// (a security fix often changes only the packaging revision, so cleanVersion would hide it).
func (s *Service) policyVersions(ctx context.Context, pkg string, fresh bool) (installed, cand string) {
	c := s.candidateInfo(ctx, pkg, fresh)
	for _, l := range strings.Split(c.raw, "\n") {
		t := strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(t, "Installed:"):
			installed = strings.TrimSpace(strings.TrimPrefix(t, "Installed:"))
		case strings.HasPrefix(t, "Candidate:"):
			cand = strings.TrimSpace(strings.TrimPrefix(t, "Candidate:"))
		}
	}
	return
}

// newerBuild returns the version apt offers when it differs from the installed one, else "".
func (s *Service) newerBuild(ctx context.Context, a *App) string {
	inst, cand := s.policyVersions(ctx, a.Packages[0], false)
	if cand == "" || cand == "(none)" || inst == "" || inst == "(none)" || cand == inst {
		return ""
	}
	return cand
}

// Upgrade updates an installed app in the background. nginx is backed up and verified first
// and rolled back if the new build does not come up with the existing configuration.
func (s *Service) Upgrade(id string) (*App, error) {
	a := find(id)
	if a == nil {
		return nil, ErrNotFound
	}
	if !a.Upgrade {
		return nil, fmt.Errorf("%w: %s cannot be updated from here", ErrBadInput, a.Name)
	}
	if !s.isInstalled(a, "") {
		return nil, fmt.Errorf("%w: %s is not installed", ErrBadState, a.Name)
	}
	return s.start(a, StateUpdating, func(ctx context.Context) error { return s.doUpgrade(ctx, a) })
}

func (s *Service) nginxTest(ctx context.Context) error {
	res, err := s.run.Run(ctx, 30*time.Second, "nginx", "-t")
	if err != nil {
		return fmt.Errorf("%s", tail(res.Stderr+res.Stdout, 4))
	}
	return nil
}

func (s *Service) backupNginx(ctx context.Context) (string, error) {
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return "", fmt.Errorf("cannot create backup folder: %w", err)
	}
	file := filepath.Join(backupDir, "nginx-"+time.Now().Format("20060102-150405")+".tar.gz")
	if res, err := s.run.Run(ctx, 2*time.Minute, "tar", "-czf", file, "-C", "/etc", "nginx"); err != nil {
		_ = os.Remove(file)
		return "", fmt.Errorf("configuration backup failed: %s", tail(res.Stderr, 3))
	}
	if st, err := os.Stat(file); err != nil || st.Size() == 0 {
		return "", fmt.Errorf("configuration backup is empty")
	}
	pruneBackups()
	return file, nil
}

func pruneBackups() {
	files, _ := filepath.Glob(filepath.Join(backupDir, "nginx-*.tar.gz"))
	sort.Strings(files)
	for len(files) > keepBackups {
		_ = os.Remove(files[0])
		files = files[1:]
	}
}

func (s *Service) restoreNginx(ctx context.Context, file string) error {
	if res, err := s.run.Run(ctx, 2*time.Minute, "tar", "-xzf", file, "-C", "/etc"); err != nil {
		return fmt.Errorf("restoring %s failed: %s", file, tail(res.Stderr, 3))
	}
	return nil
}

func (s *Service) doUpgrade(ctx context.Context, a *App) error {
	pkg := a.Packages[0]
	// 1. Never start from a broken configuration: the restart at the end would take every site down.
	if err := s.nginxTest(ctx); err != nil {
		return fmt.Errorf("nginx configuration is invalid, fix it before updating:\n%s", err)
	}
	backup, err := s.backupNginx(ctx)
	if err != nil {
		return err
	}
	if err := s.aptUpdate(ctx); err != nil {
		return err
	}
	before, cand := s.policyVersions(ctx, pkg, true)
	if cand == "" || cand == "(none)" || cand == before {
		return nil // already the newest build apt offers
	}

	// 2. Upgrade only what is installed, keep the administrator's config files.
	args := []string{"install", "--only-upgrade", "-y", "-o", "DPkg::Lock::Timeout=300",
		"-o", "Dpkg::Options::=--force-confold", "-o", "Dpkg::Options::=--force-confdef", pkg}
	if res, err := system.RunLines(ctx, s.run, 20*time.Minute, aptEnv, func(string) {}, "apt-get", args...); err != nil {
		_ = s.restoreNginx(ctx, backup)
		s.restartNginx(ctx)
		return fmt.Errorf("update failed, configuration restored:\n%s", tail(res.Stderr+res.Stdout, 6))
	}

	// 3. The new binary must accept the existing configuration, else restore the backup.
	if err := s.nginxTest(ctx); err != nil {
		if rerr := s.restoreNginx(ctx, backup); rerr != nil {
			return fmt.Errorf("new nginx rejects the configuration (%s) and the backup could not be restored: %v", err, rerr)
		}
		if err2 := s.nginxTest(ctx); err2 != nil {
			return fmt.Errorf("new nginx rejects the configuration even after restoring %s:\n%s", backup, err2)
		}
		s.restartNginx(ctx)
		return fmt.Errorf("new nginx rejected the configuration, the backup was restored:\n%s", err)
	}

	// 4. A restart (not a reload) so the new binary is the one that runs.
	if err := s.restartAndVerify(ctx, a.Unit); err != nil {
		_ = s.restoreNginx(ctx, backup)
		if err2 := s.restartAndVerify(ctx, a.Unit); err2 != nil {
			return fmt.Errorf("nginx did not start after the update (%v) and still fails after restoring the configuration: %v", err, err2)
		}
		return fmt.Errorf("nginx did not start after the update, the configuration was restored and nginx is running again: %v", err)
	}
	return nil
}

func (s *Service) restartNginx(ctx context.Context) {
	_, _ = s.run.Run(ctx, time.Minute, "systemctl", "restart", "nginx")
}

func (s *Service) restartAndVerify(ctx context.Context, unit string) error {
	if res, err := s.run.Run(ctx, time.Minute, "systemctl", "restart", unit); err != nil {
		return fmt.Errorf("restart failed: %s", tail(res.Stderr, 3))
	}
	time.Sleep(settle)
	if !s.active(ctx, unit) {
		return fmt.Errorf("%s is not active after restart", unit)
	}
	return nil
}
