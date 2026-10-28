package docker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Usage struct {
	Total  int    `json:"total"`
	Active int    `json:"active"`
	Size   string `json:"size"` // as printed by docker, e.g. "1.857GB"
}

// Overview is the "Resource Overview" of the Docker page.
type Overview struct {
	Containers Usage `json:"containers"`
	Images     Usage `json:"images"`
	Volumes    Usage `json:"volumes"`
	Networks   int   `json:"networks"`
	Compose    int   `json:"compose"`
	Registries int   `json:"registries"`
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

// registryCount counts the registries `docker login` stored credentials for.
func registryCount(configPath string) int {
	b, err := os.ReadFile(configPath)
	if err != nil {
		return 0
	}
	var cfg struct {
		Auths map[string]json.RawMessage `json:"auths"`
	}
	if json.Unmarshal(b, &cfg) != nil {
		return 0
	}
	return len(cfg.Auths)
}

func defaultDockerConfig() string {
	if d := os.Getenv("DOCKER_CONFIG"); d != "" {
		return filepath.Join(d, "config.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".docker", "config.json")
}

// Overview summarises disk usage and object counts. panelProjects are the compose projects the panel manages;
// projects started elsewhere are found through the compose label on containers.
func (d *Docker) Overview(ctx context.Context, panelProjects []string, configPath string) (*Overview, error) {
	ov := &Overview{}
	res, err := d.cli(ctx, 60*time.Second, "system", "df", "--format", "{{json .}}")
	if err != nil {
		return nil, err
	}
	_ = jsonLines(res.Stdout, func(raw []byte) error {
		var r struct{ Type, TotalCount, Active, Size string }
		if json.Unmarshal(raw, &r) != nil {
			return nil
		}
		u := Usage{Total: atoi(r.TotalCount), Active: atoi(r.Active), Size: r.Size}
		switch r.Type {
		case "Containers":
			ov.Containers = u
		case "Images":
			ov.Images = u
		case "Local Volumes":
			ov.Volumes = u
		}
		return nil
	})
	if res, err := d.cli(ctx, 20*time.Second, "network", "ls", "--format", "{{json .}}"); err == nil {
		_ = jsonLines(res.Stdout, func([]byte) error { ov.Networks++; return nil })
	}
	projects := map[string]bool{}
	for _, p := range panelProjects {
		projects[p] = true
	}
	if res, err := d.cli(ctx, 20*time.Second, "ps", "-a", "--format", `{{.Label "com.docker.compose.project"}}`); err == nil {
		for _, l := range strings.Split(res.Stdout, "\n") {
			if l = strings.TrimSpace(l); l != "" {
				projects[l] = true
			}
		}
	}
	ov.Compose = len(projects)
	if configPath != "" {
		ov.Registries = registryCount(configPath)
	}
	return ov, nil
}

// ContainerStat is a live CPU/memory sample of a running container.
type ContainerStat struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	CPU        float64 `json:"cpu"`
	MemUsed    string  `json:"mem_used"`
	MemLimit   string  `json:"mem_limit"`
	MemPercent float64 `json:"mem_percent"`
}

func pct(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(s), "%"), 64)
	return f
}

// Stats samples every running container once.
func (d *Docker) Stats(ctx context.Context) ([]ContainerStat, error) {
	res, err := d.cli(ctx, 40*time.Second, "stats", "--no-stream", "--format", "{{json .}}")
	if err != nil {
		return nil, err
	}
	out := []ContainerStat{}
	err = jsonLines(res.Stdout, func(raw []byte) error {
		var r struct{ ID, Name, CPUPerc, MemUsage, MemPerc string }
		if err := json.Unmarshal(raw, &r); err != nil {
			return err
		}
		used, limit, _ := strings.Cut(r.MemUsage, " / ")
		out = append(out, ContainerStat{ID: r.ID, Name: r.Name, CPU: pct(r.CPUPerc), MemUsed: strings.TrimSpace(used), MemLimit: strings.TrimSpace(limit), MemPercent: pct(r.MemPerc)})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, err
}
