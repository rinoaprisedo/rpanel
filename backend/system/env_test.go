package system

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func envHas(env []string, key string) bool {
	for _, kv := range env {
		if strings.HasPrefix(kv, key+"=") {
			return true
		}
	}
	return false
}

func TestDockerDoesNotInheritThePanelEnvironment(t *testing.T) {
	t.Setenv("DB_PASSWORD", "panel-secret")
	t.Setenv("DB_HOST", "127.0.0.1")
	t.Setenv("JWT_SECRET", "x")
	t.Setenv("DOCKER_HOST", "unix:///custom.sock")
	t.Setenv("https_proxy", "http://proxy:3128")
	t.Setenv("PATH", "/usr/bin")

	d := baseEnv("docker")
	for _, k := range []string{"DB_PASSWORD", "DB_HOST", "JWT_SECRET"} {
		if envHas(d, k) {
			t.Errorf("%s must not reach docker", k)
		}
	}
	for _, k := range []string{"PATH", "DOCKER_HOST", "https_proxy"} {
		if !envHas(d, k) {
			t.Errorf("%s should be kept for docker", k)
		}
	}
	if !envHas(baseEnv("apt-get"), "DB_PASSWORD") {
		t.Error("other commands keep the full environment")
	}
}

func TestDockerProcessReallyStartsClean(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\nenv\n"), 0o755); err != nil {
		t.Skip("cannot write a fake docker")
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	t.Setenv("DB_PASSWORD", "panel-secret")
	res, err := Exec{}.Run(context.Background(), 10*time.Second, "docker", "compose", "up")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Stdout, "DB_PASSWORD") || strings.Contains(res.Stdout, "panel-secret") {
		t.Errorf("the panel's variables reached docker:\n%s", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "PATH=") {
		t.Errorf("PATH is needed to find the docker plugins:\n%s", res.Stdout)
	}
}
