package dashboard

import (
	"bufio"
	"context"
	stdnet "net"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

const (
	sampleInterval = 3 * time.Second
	historySize    = 100 // ~5 minutes
)

// Point is one time-series sample.
type Point struct {
	Time       int64   `json:"t"`
	CPU        float64 `json:"cpu"`
	MemPercent float64 `json:"mem"`
	NetRxBps   float64 `json:"net_rx"`
	NetTxBps   float64 `json:"net_tx"`
	Load1      float64 `json:"load1"`
	Disk       float64 `json:"disk"`       // usage of the root file system, percent
	DiskRead   float64 `json:"disk_read"`  // bytes/s over all physical disks
	DiskWrite  float64 `json:"disk_write"` // bytes/s over all physical disks
}

type DiskInfo struct {
	Mount   string  `json:"mount"`
	FSType  string  `json:"fstype"`
	Total   uint64  `json:"total"`
	Used    uint64  `json:"used"`
	Percent float64 `json:"percent"`
	Inodes  float64 `json:"inodes_percent"`
}

// NetIface is one network interface with its current rate and lifetime totals.
type NetIface struct {
	Name    string  `json:"name"`
	IP      string  `json:"ip"`
	RxBps   float64 `json:"rx_bps"`
	TxBps   float64 `json:"tx_bps"`
	RxTotal uint64  `json:"rx_total"`
	TxTotal uint64  `json:"tx_total"`
}

// TCPStates counts the machine's TCP sockets by state.
type TCPStates struct {
	Established int `json:"established"`
	Listen      int `json:"listen"`
	TimeWait    int `json:"time_wait"`
	CloseWait   int `json:"close_wait"`
	SynRecv     int `json:"syn_recv"`
	Other       int `json:"other"`
	Total       int `json:"total"`
}

type Overview struct {
	Hostname  string     `json:"hostname"`
	OS        string     `json:"os"`
	Kernel    string     `json:"kernel"`
	Arch      string     `json:"arch"`
	Uptime    uint64     `json:"uptime"`
	CPUModel  string     `json:"cpu_model"`
	CPUCores  int        `json:"cpu_cores"`
	CPU       float64    `json:"cpu_percent"`
	MemTotal  uint64     `json:"mem_total"`
	MemUsed   uint64     `json:"mem_used"`
	MemPct    float64    `json:"mem_percent"`
	SwapTotal uint64     `json:"swap_total"`
	SwapUsed  uint64     `json:"swap_used"`
	Load1     float64    `json:"load1"`
	Load5     float64    `json:"load5"`
	Load15    float64    `json:"load15"`
	NetRxBps  float64    `json:"net_rx_bps"`
	NetTxBps  float64    `json:"net_tx_bps"`
	NetRx     uint64     `json:"net_rx_total"`
	NetTx     uint64     `json:"net_tx_total"`
	Disks     []DiskInfo `json:"disks"`

	MemAvailable uint64     `json:"mem_available"`
	MemCached    uint64     `json:"mem_cached"`
	MemBuffers   uint64     `json:"mem_buffers"`
	PerCPU       []float64  `json:"per_cpu"`
	DiskReadBps  float64    `json:"disk_read_bps"`
	DiskWriteBps float64    `json:"disk_write_bps"`
	Interfaces   []NetIface `json:"interfaces"`
	TCP          TCPStates  `json:"tcp"`
	Processes    int        `json:"processes"`
	BootTime     uint64     `json:"boot_time"`
}

type ProcessInfo struct {
	PID    int32   `json:"pid"`
	Name   string  `json:"name"`
	User   string  `json:"user"`
	CPU    float64 `json:"cpu"`
	MemPct float32 `json:"mem"`
}

// Collector samples host metrics in the background and keeps a short history.
type Collector struct {
	mu      sync.RWMutex
	current Overview
	history []Point
	lastRx  uint64
	lastTx  uint64
	lastAt  time.Time
	lastRd  uint64
	lastWr  uint64
	lastNIC map[string][2]uint64

	// long keeps one averaged point per minute for up to 7 days (in memory only).
	long []Point
	acc  minuteAcc
}

func NewCollector() *Collector { return &Collector{} }

// Start samples immediately, then every sampleInterval until ctx is done.
func (c *Collector) Start(ctx context.Context) {
	c.sample(ctx)
	go func() {
		t := time.NewTicker(sampleInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				c.sample(ctx)
			}
		}
	}()
}

func (c *Collector) sample(ctx context.Context) {
	o := Overview{}
	if h, err := host.InfoWithContext(ctx); err == nil {
		o.Hostname, o.OS, o.Kernel, o.Arch, o.Uptime = h.Hostname, h.Platform+" "+h.PlatformVersion, h.KernelVersion, h.KernelArch, h.Uptime
	}
	if infos, err := cpu.InfoWithContext(ctx); err == nil && len(infos) > 0 {
		o.CPUModel = infos[0].ModelName
	}
	o.CPUCores, _ = cpu.CountsWithContext(ctx, true)
	// interval 0 = since previous call; first call returns 0 which is fine.
	if p, err := cpu.PercentWithContext(ctx, 0, false); err == nil && len(p) > 0 {
		o.CPU = p[0]
	}
	if pc, err := cpu.PercentWithContext(ctx, 0, true); err == nil {
		o.PerCPU = pc
	}
	if m, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		o.MemTotal, o.MemUsed, o.MemPct = m.Total, m.Used, m.UsedPercent
		o.MemAvailable, o.MemCached, o.MemBuffers = m.Available, m.Cached, m.Buffers
	}
	if s, err := mem.SwapMemoryWithContext(ctx); err == nil {
		o.SwapTotal, o.SwapUsed = s.Total, s.Used
	}
	if l, err := load.AvgWithContext(ctx); err == nil {
		o.Load1, o.Load5, o.Load15 = l.Load1, l.Load5, l.Load15
	}
	o.Disks = collectDisks(ctx)
	if pids, err := process.PidsWithContext(ctx); err == nil {
		o.Processes = len(pids)
	}
	if bt, err := host.BootTimeWithContext(ctx); err == nil {
		o.BootTime = bt
	}
	o.TCP = readTCPStates("/proc/net/tcp", "/proc/net/tcp6")
	var rd, wr uint64
	if io, err := disk.IOCountersWithContext(ctx); err == nil {
		for name, st := range io {
			if physicalDisk(name) {
				rd += st.ReadBytes
				wr += st.WriteBytes
			}
		}
	}
	nics := map[string][2]uint64{}
	if io, err := net.IOCountersWithContext(ctx, true); err == nil {
		addrs := ifaceIPs(ctx)
		for _, n := range io {
			if skipIface(n.Name) {
				continue
			}
			nics[n.Name] = [2]uint64{n.BytesRecv, n.BytesSent}
			o.Interfaces = append(o.Interfaces, NetIface{Name: n.Name, IP: addrs[n.Name], RxTotal: n.BytesRecv, TxTotal: n.BytesSent})
		}
		sort.Slice(o.Interfaces, func(i, j int) bool { return o.Interfaces[i].Name < o.Interfaces[j].Name })
	}

	now := time.Now()
	if io, err := net.IOCountersWithContext(ctx, false); err == nil && len(io) > 0 {
		o.NetRx, o.NetTx = io[0].BytesRecv, io[0].BytesSent
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.lastAt.IsZero() {
		if dt := now.Sub(c.lastAt).Seconds(); dt > 0 {
			if o.NetRx >= c.lastRx {
				o.NetRxBps = float64(o.NetRx-c.lastRx) / dt
			}
			if o.NetTx >= c.lastTx {
				o.NetTxBps = float64(o.NetTx-c.lastTx) / dt
			}
			if rd >= c.lastRd {
				o.DiskReadBps = float64(rd-c.lastRd) / dt
			}
			if wr >= c.lastWr {
				o.DiskWriteBps = float64(wr-c.lastWr) / dt
			}
			for i := range o.Interfaces {
				if prev, ok := c.lastNIC[o.Interfaces[i].Name]; ok {
					if o.Interfaces[i].RxTotal >= prev[0] {
						o.Interfaces[i].RxBps = float64(o.Interfaces[i].RxTotal-prev[0]) / dt
					}
					if o.Interfaces[i].TxTotal >= prev[1] {
						o.Interfaces[i].TxBps = float64(o.Interfaces[i].TxTotal-prev[1]) / dt
					}
				}
			}
		}
	}
	c.lastRd, c.lastWr, c.lastNIC = rd, wr, nics
	c.lastRx, c.lastTx, c.lastAt = o.NetRx, o.NetTx, now
	c.current = o
	pt := Point{
		Time: now.Unix(), CPU: o.CPU, MemPercent: o.MemPct,
		NetRxBps: o.NetRxBps, NetTxBps: o.NetTxBps, Load1: o.Load1, Disk: rootDisk(o.Disks),
		DiskRead: o.DiskReadBps, DiskWrite: o.DiskWriteBps,
	}
	c.history = append(c.history, pt)
	if len(c.history) > historySize {
		c.history = c.history[len(c.history)-historySize:]
	}
	c.addLong(pt)
}

// rootDisk picks the file system the panel and sites live on: "/" if present, else the first.
func rootDisk(ds []DiskInfo) float64 {
	for _, d := range ds {
		if d.Mount == "/" {
			return d.Percent
		}
	}
	if len(ds) > 0 {
		return ds[0].Percent
	}
	return 0
}

var readOnlyFS = map[string]bool{"squashfs": true, "iso9660": true, "udf": true}

func collectDisks(ctx context.Context) []DiskInfo {
	parts, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	out := []DiskInfo{}
	for _, p := range parts {
		if seen[p.Device] || seen[p.Mountpoint] || readOnlyFS[p.Fstype] {
			continue // snap images and ISOs are always 100% full and never actionable
		}
		u, err := disk.UsageWithContext(ctx, p.Mountpoint)
		if err != nil || u.Total == 0 {
			continue
		}
		seen[p.Device], seen[p.Mountpoint] = true, true
		out = append(out, DiskInfo{Mount: p.Mountpoint, FSType: p.Fstype, Total: u.Total, Used: u.Used, Percent: u.UsedPercent, Inodes: u.InodesUsedPercent})
	}
	return out
}

func (c *Collector) Overview() Overview {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.current
}

func (c *Collector) History() []Point {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Point, len(c.history))
	copy(out, c.history)
	return out
}

// TopProcesses returns the top n processes ordered by "cpu" or "mem".
func TopProcesses(ctx context.Context, by string, n int) []ProcessInfo {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return []ProcessInfo{}
	}
	list := make([]ProcessInfo, 0, len(procs))
	for _, p := range procs {
		name, err := p.NameWithContext(ctx)
		if err != nil {
			continue
		}
		cpuP, _ := p.CPUPercentWithContext(ctx) // lifetime average; cheap, no sleep
		memP, _ := p.MemoryPercentWithContext(ctx)
		user, _ := p.UsernameWithContext(ctx)
		list = append(list, ProcessInfo{PID: p.Pid, Name: name, User: user, CPU: cpuP, MemPct: memP})
	}
	sort.Slice(list, func(i, j int) bool {
		if by == "mem" {
			return list[i].MemPct > list[j].MemPct
		}
		return list[i].CPU > list[j].CPU
	})
	if len(list) > n {
		list = list[:n]
	}
	return list
}

var diskNameRe = regexp.MustCompile(`^(sd[a-z]+|vd[a-z]+|xvd[a-z]+|hd[a-z]+|nvme[0-9]+n[0-9]+|mmcblk[0-9]+)$`)

// physicalDisk keeps whole disks only: partitions, loop, RAM and device-mapper devices would count the same bytes twice.
func physicalDisk(name string) bool { return diskNameRe.MatchString(name) }

// skipIface hides loopback and the per-container virtual interfaces.
func skipIface(name string) bool {
	return name == "lo" || strings.HasPrefix(name, "veth") || strings.HasPrefix(name, "br-") && len(name) > 15
}

func ifaceIPs(ctx context.Context) map[string]string {
	out := map[string]string{}
	ifs, err := net.InterfacesWithContext(ctx)
	if err != nil {
		return out
	}
	for _, i := range ifs {
		for _, a := range i.Addrs {
			ip, _, err := stdnet.ParseCIDR(a.Addr)
			if err == nil && ip.To4() != nil {
				out[i.Name] = ip.String()
				break
			}
		}
	}
	return out
}

// readTCPStates counts sockets per state from /proc/net/tcp{,6} (cheap, no per-process scan). Missing files give zeros.
func readTCPStates(paths ...string) TCPStates {
	var t TCPStates
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		first := true
		for sc.Scan() {
			if first {
				first = false
				continue
			}
			fields := strings.Fields(sc.Text())
			if len(fields) < 4 {
				continue
			}
			t.Total++
			switch fields[3] {
			case "01":
				t.Established++
			case "0A":
				t.Listen++
			case "06":
				t.TimeWait++
			case "08":
				t.CloseWait++
			case "03":
				t.SynRecv++
			default:
				t.Other++
			}
		}
		f.Close()
	}
	return t
}
