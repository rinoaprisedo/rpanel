package docker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"rpanel/backend/system"
)

type call struct {
	name string
	args []string
}

type fake struct {
	calls  []call
	stdout map[string]string // keyed by first docker arg
	fail   map[string]bool
}

func (f *fake) Run(_ context.Context, _ time.Duration, name string, args ...string) (system.Result, error) {
	f.calls = append(f.calls, call{name, args})
	key := ""
	if len(args) > 0 {
		key = args[0]
	}
	if name == "apt-get" && len(args) > 1 {
		key = "apt-" + args[len(args)-1]
	}
	if f.fail[key] {
		return system.Result{Stderr: "boom\nreason"}, errors.New("exit 1")
	}
	return system.Result{Stdout: f.stdout[key]}, nil
}

func newDocker() (*Docker, *fake) {
	f := &fake{stdout: map[string]string{}, fail: map[string]bool{}}
	return &Docker{Run: f}, f
}

func TestListContainersParsesJSONLines(t *testing.T) {
	d, f := newDocker()
	f.stdout["ps"] = `{"ID":"abc123def456","Names":"web","Image":"nginx:latest","State":"running","Status":"Up 2 hours","Ports":"0.0.0.0:80->80/tcp","CreatedAt":"2024-01-01 10:00:00 +0000 UTC"}
{"ID":"fff000fff000","Names":"db","Image":"postgres:16","State":"exited","Status":"Exited (0) 1 day ago","Ports":"","CreatedAt":"x"}
`
	got, err := d.ListContainers(context.Background())
	if err != nil || len(got) != 2 {
		t.Fatalf("got %v err %v", got, err)
	}
	if got[0].Name != "web" || got[0].State != "running" || got[1].Image != "postgres:16" {
		t.Fatalf("parsed %+v", got)
	}
	f.stdout["ps"] = "not json\n"
	if _, err := d.ListContainers(context.Background()); err == nil {
		t.Fatal("malformed output must error")
	}
}

func TestActionArgvAndValidation(t *testing.T) {
	d, f := newDocker()
	ctx := context.Background()
	for action, want := range map[string]string{
		"start": "start web", "stop": "stop web", "restart": "restart web", "remove": "rm -f web",
	} {
		f.calls = nil
		if err := d.Action(ctx, "web", action); err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(f.calls[0].args, " "); got != want {
			t.Errorf("%s: argv %q want %q", action, got, want)
		}
	}
	for _, id := range []string{"", "-f", "--all", "web; rm", "a b", "../x", "$(id)"} {
		if err := d.Action(ctx, id, "stop"); !errors.Is(err, ErrBadInput) {
			t.Errorf("id %q must be rejected, got %v", id, err)
		}
	}
	if err := d.Action(ctx, "web", "exec"); !errors.Is(err, ErrBadInput) {
		t.Fatalf("unknown action: %v", err)
	}
}

func TestBuildRunArgs(t *testing.T) {
	args, err := BuildRunArgs(RunInput{
		Image: "nginx:1.25", Name: "web", Restart: "unless-stopped",
		Ports: []string{"8080:80", "127.0.0.1:9000:9000/tcp"}, Env: []string{"A=b c", "EMPTY="},
		Volumes: []string{"/www/site:/usr/share/nginx/html:ro", "data:/var/lib/x"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "run -d --name web --restart unless-stopped -p 8080:80 -p 127.0.0.1:9000:9000/tcp -e A=b c -e EMPTY= -v /www/site:/usr/share/nginx/html:ro -v data:/var/lib/x nginx:1.25"
	if got := strings.Join(args, " "); got != want {
		t.Fatalf("argv\n got %q\nwant %q", got, want)
	}
	if args[len(args)-1] != "nginx:1.25" {
		t.Fatal("image must be last")
	}

	bad := []RunInput{
		{Image: "--privileged"},
		{Image: "-v/:/host alpine"},
		{Image: "nginx", Name: "--rm"},
		{Image: "nginx", Name: "a b"},
		{Image: "nginx", Ports: []string{"80:80 --privileged"}},
		{Image: "nginx", Ports: []string{"abc"}},
		{Image: "nginx", Env: []string{"NOEQUALS"}},
		{Image: "nginx", Env: []string{"A B=c"}},
		{Image: "nginx", Env: []string{"A=b\nc"}},
		{Image: "nginx", Volumes: []string{"/a:/b --privileged"}},
		{Image: "nginx", Volumes: []string{"/a/../etc:/b"}},
		{Image: "nginx", Volumes: []string{"relative/path"}},
		{Image: "nginx", Restart: "sometimes"},
	}
	for i, in := range bad {
		if _, err := BuildRunArgs(in); !errors.Is(err, ErrBadInput) {
			t.Errorf("case %d (%+v) must be rejected, got %v", i, in, err)
		}
	}
}

func TestImages(t *testing.T) {
	d, f := newDocker()
	f.stdout["images"] = `{"ID":"sha256:abcdef123456","Repository":"nginx","Tag":"latest","Size":"187MB","CreatedSince":"2 weeks ago"}` + "\n"
	imgs, err := d.ListImages(context.Background())
	if err != nil || len(imgs) != 1 || imgs[0].Repository != "nginx" || imgs[0].Size != "187MB" {
		t.Fatalf("imgs %+v err %v", imgs, err)
	}
	for _, ref := range []string{"", "-q", "nginx;ls", "a b", "$(x)"} {
		if err := d.Pull(context.Background(), ref); !errors.Is(err, ErrBadInput) {
			t.Errorf("pull %q: %v", ref, err)
		}
		if err := d.RemoveImage(context.Background(), ref); !errors.Is(err, ErrBadInput) {
			t.Errorf("rmi %q: %v", ref, err)
		}
	}
	if err := d.Pull(context.Background(), "ghcr.io/org/app:1.0"); err != nil {
		t.Fatal(err)
	}
}

func TestErrorsUseStderrTail(t *testing.T) {
	d, f := newDocker()
	f.fail["stop"] = true
	err := d.Action(context.Background(), "web", "stop")
	if err == nil || !strings.Contains(err.Error(), "reason") {
		t.Fatalf("want stderr in error, got %v", err)
	}
}

func TestLogsClampsTail(t *testing.T) {
	d, f := newDocker()
	d.Logs(context.Background(), "web", 999999)
	if got := strings.Join(f.calls[0].args, " "); got != "logs --tail 200 --timestamps web" {
		t.Fatalf("argv %q", got)
	}
	if _, err := d.Logs(context.Background(), "-f", 10); !errors.Is(err, ErrBadInput) {
		t.Fatal("flag-like container must be rejected")
	}
}

func TestStatus(t *testing.T) {
	d, f := newDocker()
	f.stdout["version"] = "24.0.7\n"
	st := d.Status(context.Background())
	if !st.Installed || !st.Running || st.Version != "24.0.7" || !st.Compose {
		t.Fatalf("status %+v", st)
	}
	f.fail["version"] = true
	st = d.Status(context.Background())
	if !st.Installed || st.Running {
		t.Fatalf("daemon-down status %+v", st)
	}
}

func TestInstallOrder(t *testing.T) {
	d, f := newDocker()
	f.fail["apt-docker-compose-v2"] = true // plugin name differs: fallback is attempted, not fatal
	if err := d.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	var seq []string
	for _, c := range f.calls {
		seq = append(seq, c.name+":"+c.args[len(c.args)-1])
	}
	got := strings.Join(seq, ",")
	want := "apt-get:update,apt-get:docker.io,apt-get:docker-compose-v2,apt-get:docker-compose-plugin,systemctl:docker"
	if got != want {
		t.Fatalf("order\n got %s\nwant %s", got, want)
	}
}

func TestComposeProjects(t *testing.T) {
	d, f := newDocker()
	c := &Compose{D: d, Dir: filepath.Join(t.TempDir(), "docker")}
	ctx := context.Background()

	for _, n := range []string{"", "Bad", "a/b", "..", "-x", "a b", strings.Repeat("a", 64)} {
		if err := c.Save(n, "services: {}", true); !errors.Is(err, ErrBadInput) {
			t.Errorf("name %q must be rejected, got %v", n, err)
		}
	}
	if err := c.Save("shop", "", true); !errors.Is(err, ErrBadInput) {
		t.Fatal("empty content must be rejected")
	}
	if err := c.Save("shop", "services:\n  web:\n    image: nginx\n", true); err != nil {
		t.Fatal(err)
	}
	if err := c.Save("shop", "x: y", true); err == nil {
		t.Fatal("duplicate create must fail")
	}
	if err := c.Save("shop", "services: {}", false); err != nil {
		t.Fatal("update must be allowed")
	}
	if st, _ := os.Stat(filepath.Join(c.Dir, "shop", "docker-compose.yml")); st.Mode().Perm() != 0o600 {
		t.Fatal("compose file must be 0600")
	}
	if list, _ := c.List(); len(list) != 1 || list[0].Name != "shop" {
		t.Fatalf("list %v", list)
	}

	f.calls = nil
	if _, err := c.Up(ctx, "shop"); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(f.calls[0].args, " ")
	wantPrefix := "compose -p shop --project-directory " + filepath.Join(c.Dir, "shop") + " -f " + filepath.Join(c.Dir, "shop", "docker-compose.yml") + " up -d"
	if !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("argv %q", got)
	}
	if _, err := c.Up(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing project: %v", err)
	}
	if err := c.Delete(ctx, "shop"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(c.Dir, "shop")); err == nil {
		t.Fatal("project dir must be removed")
	}
}

func TestOverviewAndStats(t *testing.T) {
	d, f := newDocker()
	f.stdout["system"] = `{"Active":"6","Reclaimable":"0B (0%)","Size":"200.7kB","TotalCount":"6","Type":"Containers"}
{"Active":"5","Reclaimable":"1GB (50%)","Size":"1.857GB","TotalCount":"9","Type":"Images"}
{"Active":"4","Reclaimable":"0B","Size":"2.616GB","TotalCount":"5","Type":"Local Volumes"}
{"Active":"0","Reclaimable":"0B","Size":"0B","TotalCount":"0","Type":"Build Cache"}
`
	f.stdout["network"] = `{"Name":"bridge"}
{"Name":"host"}
{"Name":"none"}
{"Name":"toi_default"}
`
	f.stdout["ps"] = "toi\ntoi\n\nother\n"
	cfg := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(cfg, []byte(`{"auths":{"https://index.docker.io/v1/":{},"ghcr.io":{}}}`), 0o600)
	ov, err := d.Overview(context.Background(), []string{"toi", "panelapp"}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if ov.Containers.Total != 6 || ov.Images.Total != 9 || ov.Images.Size != "1.857GB" || ov.Volumes.Total != 5 || ov.Networks != 4 {
		t.Errorf("overview %+v", ov)
	}
	if ov.Compose != 3 { // toi (both sources), panelapp (panel only), other (label only)
		t.Errorf("compose projects = %d", ov.Compose)
	}
	if ov.Registries != 2 {
		t.Errorf("registries = %d", ov.Registries)
	}

	f.stdout["stats"] = `{"ID":"a1","Name":"toi-postgres","CPUPerc":"0.43%","MemUsage":"333.6MiB / 1.82GiB","MemPerc":"17.90%"}
{"ID":"b2","Name":"toi-backend-1","CPUPerc":"0.00%","MemUsage":"26.27MiB / 1.82GiB","MemPerc":"1.41%"}
`
	st, err := d.Stats(context.Background())
	if err != nil || len(st) != 2 || st[0].Name != "toi-backend-1" {
		t.Fatalf("stats %+v %v", st, err)
	}
	if st[1].CPU != 0.43 || st[1].MemUsed != "333.6MiB" || st[1].MemLimit != "1.82GiB" || st[1].MemPercent != 17.9 {
		t.Errorf("parsed %+v", st[1])
	}
}
