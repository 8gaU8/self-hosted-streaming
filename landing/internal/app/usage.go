package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"sync"
	"time"
)

// dockerStats mirrors the subset of the Docker Engine's stats JSON that
// usage math needs. Field names match the raw API response (same JSON the
// Python version decoded via the docker SDK), not Go naming conventions.
type dockerStats struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage  uint64   `json:"total_usage"`
			PercpuUsage []uint64 `json:"percpu_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs     uint64 `json:"online_cpus"`
	} `json:"cpu_stats"`
	PrecpuStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64 `json:"usage"`
		Limit uint64 `json:"limit"`
		Stats struct {
			InactiveFile uint64 `json:"inactive_file"`
			Cache        uint64 `json:"cache"`
		} `json:"stats"`
	} `json:"memory_stats"`
	BlkioStats struct {
		IoServiceBytesRecursive []struct {
			Op    string `json:"op"`
			Value uint64 `json:"value"`
		} `json:"io_service_bytes_recursive"`
	} `json:"blkio_stats"`
}

type usageResult struct {
	CPUPercent float64  `json:"cpu_percent"`
	MemUsed    uint64   `json:"mem_used"`
	MemLimit   uint64   `json:"mem_limit"`
	MemPercent *float64 `json:"mem_percent"`
	DiskRead   uint64   `json:"disk_read"`
	DiskWrite  uint64   `json:"disk_write"`
}

func cpuPercent(s *dockerStats) float64 {
	cpuDelta := int64(s.CPUStats.CPUUsage.TotalUsage) - int64(s.PrecpuStats.CPUUsage.TotalUsage)
	systemDelta := int64(s.CPUStats.SystemCPUUsage) - int64(s.PrecpuStats.SystemCPUUsage)

	onlineCPUs := s.CPUStats.OnlineCPUs
	if onlineCPUs == 0 {
		onlineCPUs = uint64(len(s.CPUStats.CPUUsage.PercpuUsage))
	}
	if onlineCPUs == 0 {
		onlineCPUs = uint64(runtime.NumCPU())
	}
	if onlineCPUs == 0 {
		onlineCPUs = 1
	}

	if systemDelta <= 0 || cpuDelta < 0 {
		return 0.0
	}

	return round1(float64(cpuDelta) / float64(systemDelta) * float64(onlineCPUs) * 100)
}

// memoryUsage matches `docker stats`: it excludes page cache so the result
// reflects actual memory pressure rather than the kernel's reclaimable disk
// cache.
func memoryUsage(s *dockerStats) (used, limit uint64) {
	cache := s.MemoryStats.Stats.InactiveFile
	if cache == 0 {
		cache = s.MemoryStats.Stats.Cache
	}

	used = s.MemoryStats.Usage
	if used > cache {
		used -= cache
	} else {
		used = 0
	}

	return used, s.MemoryStats.Limit
}

func blkioBytes(s *dockerStats) (read, write uint64) {
	for _, e := range s.BlkioStats.IoServiceBytesRecursive {
		switch e.Op {
		case "read", "Read":
			read += e.Value
		case "write", "Write":
			write += e.Value
		}
	}
	return read, write
}

func usageFromStats(s *dockerStats) usageResult {
	memUsed, memLimit := memoryUsage(s)
	diskRead, diskWrite := blkioBytes(s)

	u := usageResult{
		CPUPercent: cpuPercent(s),
		MemUsed:    memUsed,
		MemLimit:   memLimit,
		DiskRead:   diskRead,
		DiskWrite:  diskWrite,
	}
	if memLimit > 0 {
		p := round1(float64(memUsed) / float64(memLimit) * 100)
		u.MemPercent = &p
	}
	return u
}

func round1(v float64) float64 {
	return float64(int64(v*10+0.5)) / 10
}

// DockerCollector talks to the Docker Engine API directly over its unix
// socket to discover the compose project's containers and fetch their
// stats. This deliberately avoids the official docker/docker Go module: at
// the time of writing it's mid-split into github.com/moby/moby, and the
// resulting version skew between its submodules doesn't build cleanly.
// The three calls this needs (list, filter by label, one-shot stats) are a
// thin, stable slice of the API, so talking to it with plain net/http
// avoids that churn and a large transitive dependency tree.
type DockerCollector struct {
	http *http.Client
}

// NewDockerCollector connects to the Docker Engine's unix socket.
func NewDockerCollector() (*DockerCollector, error) {
	sock := os.Getenv("DOCKER_SOCKET")
	if sock == "" {
		sock = "/var/run/docker.sock"
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", sock)
		},
	}
	return &DockerCollector{http: &http.Client{Transport: transport, Timeout: 10 * time.Second}}, nil
}

func (d *DockerCollector) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker"+path, nil)
	if err != nil {
		return err
	}

	resp, err := d.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("docker API %s: status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type containerSummary struct {
	ID     string            `json:"Id"`
	Names  []string          `json:"Names"`
	Labels map[string]string `json:"Labels"`
}

func (d *DockerCollector) listByLabel(ctx context.Context, label string) []containerSummary {
	filterJSON, _ := json.Marshal(map[string][]string{"label": {label}})

	var out []containerSummary
	if err := d.get(ctx, "/containers/json?filters="+url.QueryEscape(string(filterJSON)), &out); err != nil {
		return nil
	}
	return out
}

// listProjectContainers returns all running containers belonging to this
// docker-compose project, discovered via any known monitored service's
// compose project label -- same approach as the Flask version's
// list_project_containers().
func (d *DockerCollector) listProjectContainers(ctx context.Context) []containerSummary {
	for _, svc := range monitoredServices {
		list := d.listByLabel(ctx, fmt.Sprintf("com.docker.compose.service=%s", svc.Key))
		if len(list) == 0 {
			continue
		}
		project := list[0].Labels["com.docker.compose.project"]
		if project == "" {
			continue
		}
		return d.listByLabel(ctx, fmt.Sprintf("com.docker.compose.project=%s", project))
	}
	return nil
}

type containerUsage struct {
	service string
	stats   *dockerStats
}

// fetchAllStats fetches a one-shot stats sample for every container in
// parallel. The Docker Engine API takes roughly 1-2s per container for a
// one-shot sample (it observes two cgroup reads a beat apart to compute
// deltas); fetching sequentially would make this endpoint take
// N * ~1.5s, so goroutines bound it to ~1.5s regardless of container count
// (mirrors the Flask version's ThreadPoolExecutor).
func (d *DockerCollector) fetchAllStats(ctx context.Context, containers []containerSummary) []containerUsage {
	results := make([]containerUsage, len(containers))
	var wg sync.WaitGroup

	for i, c := range containers {
		wg.Add(1)
		go func(i int, c containerSummary) {
			defer wg.Done()

			service := c.Labels["com.docker.compose.service"]
			if service == "" {
				if len(c.Names) > 0 {
					service = c.Names[0]
				} else {
					service = c.ID
				}
			}

			var s dockerStats
			if err := d.get(ctx, "/containers/"+c.ID+"/stats?stream=false", &s); err != nil {
				results[i] = containerUsage{service: service}
				return
			}

			results[i] = containerUsage{service: service, stats: &s}
		}(i, c)
	}

	wg.Wait()
	return results
}

type totalUsage struct {
	CPUPercent     float64  `json:"cpu_percent"`
	MemUsed        uint64   `json:"mem_used"`
	MemLimit       uint64   `json:"mem_limit"`
	MemPercent     *float64 `json:"mem_percent"`
	DiskRead       uint64   `json:"disk_read"`
	DiskWrite      uint64   `json:"disk_write"`
	ContainerCount int      `json:"container_count"`
}

type serviceUsage struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	Available bool   `json:"available"`
	usageResult
}

// MarshalJSON flattens usageResult's fields into the service entry, and
// omits them entirely when the service is unavailable -- matching the
// Flask version's `{**by_service.get(svc["key"], {})}` shape.
func (s serviceUsage) MarshalJSON() ([]byte, error) {
	if !s.Available {
		return json.Marshal(struct {
			Key       string `json:"key"`
			Name      string `json:"name"`
			Available bool   `json:"available"`
		}{s.Key, s.Name, s.Available})
	}
	return json.Marshal(struct {
		Key       string `json:"key"`
		Name      string `json:"name"`
		Available bool   `json:"available"`
		usageResult
	}{s.Key, s.Name, s.Available, s.usageResult})
}

type usageResponse struct {
	UpdatedAt string         `json:"updated_at"`
	Services  []serviceUsage `json:"services"`
	Total     totalUsage     `json:"total"`
}

func (d *DockerCollector) usage(ctx context.Context) usageResponse {
	containers := d.listProjectContainers(ctx)
	results := d.fetchAllStats(ctx, containers)

	byService := map[string]usageResult{}
	total := totalUsage{}

	for _, r := range results {
		if r.stats == nil {
			continue
		}
		u := usageFromStats(r.stats)
		byService[r.service] = u

		total.CPUPercent += u.CPUPercent
		total.MemUsed += u.MemUsed
		if u.MemLimit > total.MemLimit {
			total.MemLimit = u.MemLimit
		}
		total.DiskRead += u.DiskRead
		total.DiskWrite += u.DiskWrite
		total.ContainerCount++
	}

	total.CPUPercent = round1(total.CPUPercent)
	if total.MemLimit > 0 {
		p := round1(float64(total.MemUsed) / float64(total.MemLimit) * 100)
		total.MemPercent = &p
	}

	services := make([]serviceUsage, 0, len(monitoredServices))
	for _, svc := range monitoredServices {
		u, ok := byService[svc.Key]
		services = append(services, serviceUsage{Key: svc.Key, Name: svc.Name, Available: ok, usageResult: u})
	}

	return usageResponse{
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Services:  services,
		Total:     total,
	}
}
