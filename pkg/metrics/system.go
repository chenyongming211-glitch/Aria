package metrics

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	// CPUUsagePercent CPU 使用率（%）
	CPUUsagePercent = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "aria_cpu_usage_percent",
			Help: "CPU usage percentage",
		},
	)

	// CPUCoreUsagePercent Per-Core CPU 使用率
	CPUCoreUsagePercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "aria_cpu_core_usage_percent",
			Help: "Per-core CPU usage percentage",
		},
		[]string{"core", "mode"}, // mode: user, system, softirq, idle
	)

	// CPUBalanceScore CPU 负载均衡度评分 (0-100)
	CPUBalanceScore = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "aria_cpu_balance_score",
			Help: "CPU load balance score (0-100, higher is better)",
		},
	)

	// MemoryBytes 内存使用量（按类型）
	MemoryBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "aria_memory_bytes",
			Help: "Memory usage in bytes",
		},
		[]string{"type"}, // type: alloc, sys, heap_alloc, heap_sys
	)

	// SystemMemoryUsagePercent 系统内存使用率（%）
	SystemMemoryUsagePercent = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "aria_memory_usage_percent",
			Help: "System memory usage percentage",
		},
	)

	// GoGoroutines Goroutine 数量
	GoGoroutines = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "aria_go_goroutines",
			Help: "Number of goroutines",
		},
	)

	// GCPauseSeconds GC 暂停时间（秒）
	GCPauseSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "aria_gc_pause_seconds",
			Help: "Last GC pause duration in seconds",
		},
	)

	// ProcessOpenFDs 打开的文件描述符数量
	ProcessOpenFDs = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "process_open_fds",
			Help: "Number of open file descriptors",
		},
	)

	// ProcessMaxFDs 最大文件描述符数量
	ProcessMaxFDs = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "process_max_fds",
			Help: "Maximum number of file descriptors",
		},
	)

	// NodeNFConntrackEntries 当前 conntrack 条目数
	NodeNFConntrackEntries = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "node_nf_conntrack_entries",
			Help: "Current number of conntrack entries",
		},
	)

	// NodeNFConntrackEntriesLimit conntrack 最大条目数
	NodeNFConntrackEntriesLimit = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "node_nf_conntrack_entries_limit",
			Help: "Maximum number of conntrack entries",
		},
	)

	// BuildInfo Aria 版本和构建信息
	BuildInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "aria_build_info",
			Help: "Aria version and build information",
		},
		[]string{"version", "commit", "build_date", "go_version", "public_ip", "local_ip", "runtime_mode"},
	)

	// DiskUsagePercent 磁盘使用率（%）
	DiskUsagePercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "aria_disk_usage_percent",
			Help: "Disk usage percentage",
		},
		[]string{"path"},
	)

	// DiskFreeBytes 可用磁盘空间（字节）
	DiskFreeBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "aria_disk_free_bytes",
			Help: "Free disk space in bytes",
		},
		[]string{"path"},
	)
)

// SystemCollector 系统资源采集器
type SystemCollector struct {
	lastCPUStat     *cpuStat
	lastCPUCoreStat map[string]*cpuCoreStat
}

// cpuStat CPU 统计信息
type cpuStat struct {
	user   uint64
	nice   uint64
	system uint64
	idle   uint64
	iowait uint64
	irq    uint64
	total  uint64
}

// cpuCoreStat Per-Core CPU 统计信息
type cpuCoreStat struct {
	user    uint64
	nice    uint64
	system  uint64
	idle    uint64
	iowait  uint64
	irq     uint64
	softirq uint64
	total   uint64
}

// NewSystemCollector 创建系统资源采集器
func NewSystemCollector(registerer prometheus.Registerer, version, commit, publicIP, localIP, runtimeMode string) *SystemCollector {
	// 注册指标
	registerer.MustRegister(
		CPUUsagePercent,
		CPUCoreUsagePercent,
		CPUBalanceScore,
		MemoryBytes,
		SystemMemoryUsagePercent,
		GoGoroutines,
		GCPauseSeconds,
		ProcessOpenFDs,
		ProcessMaxFDs,
		NodeNFConntrackEntries,
		NodeNFConntrackEntriesLimit,
		BuildInfo,
		DiskUsagePercent,
		DiskFreeBytes,
	)

	// 设置构建信息（只设置一次）
	buildDate := time.Now().Format("2006-01-02")
	goVersion := runtime.Version()
	if commit == "" {
		commit = "unknown"
	}
	BuildInfo.WithLabelValues(version, commit, buildDate, goVersion, publicIP, localIP, runtimeMode).Set(1)

	return &SystemCollector{}
}

// Name 返回采集器名称
func (sc *SystemCollector) Name() string {
	return "system"
}

// Collect 采集系统资源指标
func (sc *SystemCollector) Collect(ctx context.Context) error {
	// CPU 使用率
	cpuPercent, err := sc.getCPUUsage()
	if err == nil {
		CPUUsagePercent.Set(cpuPercent)
	} else {
		log.Printf("[metrics] Failed to get CPU usage: %v", err)
	}

	// Per-Core CPU 使用率和均衡度
	if err := sc.collectPerCoreCPU(); err != nil {
		log.Printf("[metrics] Failed to collect per-core CPU: %v", err)
	}

	// 内存使用
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	MemoryBytes.WithLabelValues("alloc").Set(float64(mem.Alloc))
	MemoryBytes.WithLabelValues("sys").Set(float64(mem.Sys))
	MemoryBytes.WithLabelValues("heap_alloc").Set(float64(mem.HeapAlloc))
	MemoryBytes.WithLabelValues("heap_sys").Set(float64(mem.HeapSys))

	// 系统内存使用率
	if err := sc.collectSystemMemory(); err != nil {
		log.Printf("[metrics] Failed to collect system memory: %v", err)
	}

	// Goroutine 数量
	GoGoroutines.Set(float64(runtime.NumGoroutine()))

	// GC 暂停时间
	if mem.NumGC > 0 {
		// 获取最近一次 GC 暂停时间
		lastPause := mem.PauseNs[(mem.NumGC+255)%256]
		GCPauseSeconds.Set(float64(lastPause) / 1e9)
	}

	// 文件描述符监控
	if err := sc.collectFDStats(); err != nil {
		log.Printf("[metrics] Failed to collect FD stats: %v", err)
	}

	// Conntrack 监控
	if err := sc.collectConntrackStats(); err != nil {
		log.Printf("[metrics] Failed to collect conntrack stats: %v", err)
	}

	// 磁盘空间监控
	if err := sc.collectDiskStats(); err != nil {
		log.Printf("[metrics] Failed to collect disk stats: %v", err)
	}

	log.Printf("[metrics] System: CPU=%.1f%%, Mem=%dMB, Goroutines=%d",
		cpuPercent, mem.Alloc/1024/1024, runtime.NumGoroutine())

	return nil
}

// getCPUUsage 计算 CPU 使用率（Linux）
func (sc *SystemCollector) getCPUUsage() (float64, error) {
	// 读取 /proc/stat
	current, err := sc.readCPUStat()
	if err != nil {
		return 0, err
	}

	// 首次调用，保存基准值
	if sc.lastCPUStat == nil {
		sc.lastCPUStat = current
		time.Sleep(100 * time.Millisecond) // 短暂等待
		current, err = sc.readCPUStat()
		if err != nil {
			return 0, err
		}
	}

	// 计算差值
	totalDelta := current.total - sc.lastCPUStat.total
	idleDelta := current.idle - sc.lastCPUStat.idle

	if totalDelta == 0 {
		return 0, nil
	}

	cpuPercent := 100.0 * float64(totalDelta-idleDelta) / float64(totalDelta)

	// 更新基准值
	sc.lastCPUStat = current

	return cpuPercent, nil
}

// readCPUStat 读取 /proc/stat
func (sc *SystemCollector) readCPUStat() (*cpuStat, error) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return nil, fmt.Errorf("failed to open /proc/stat: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return nil, fmt.Errorf("empty /proc/stat")
	}

	line := scanner.Text()
	if !strings.HasPrefix(line, "cpu ") {
		return nil, fmt.Errorf("invalid /proc/stat format")
	}

	// 解析 CPU 数据
	fields := strings.Fields(line)
	if len(fields) < 8 {
		return nil, fmt.Errorf("invalid cpu line: %s", line)
	}

	stat := &cpuStat{}
	stat.user, _ = strconv.ParseUint(fields[1], 10, 64)
	stat.nice, _ = strconv.ParseUint(fields[2], 10, 64)
	stat.system, _ = strconv.ParseUint(fields[3], 10, 64)
	stat.idle, _ = strconv.ParseUint(fields[4], 10, 64)
	stat.iowait, _ = strconv.ParseUint(fields[5], 10, 64)
	stat.irq, _ = strconv.ParseUint(fields[6], 10, 64)

	stat.total = stat.user + stat.nice + stat.system + stat.idle + stat.iowait + stat.irq

	return stat, nil
}

// collectFDStats 采集文件描述符统计（仅 Linux）
func (sc *SystemCollector) collectFDStats() error {
	// 仅 Linux 支持
	if runtime.GOOS != "linux" {
		return nil
	}

	// 方法1: 计数 /proc/self/fd 下的文件
	files, err := os.ReadDir("/proc/self/fd")
	if err == nil {
		ProcessOpenFDs.Set(float64(len(files)))
	}

	// 方法2: 读取 /proc/self/limits
	data, err := os.ReadFile("/proc/self/limits")
	if err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.Contains(line, "Max open files") {
				fields := strings.Fields(line)
				if len(fields) >= 5 {
					maxFDs, _ := strconv.ParseFloat(fields[3], 64)
					ProcessMaxFDs.Set(maxFDs)
				}
				break
			}
		}
	}

	return nil
}

// collectConntrackStats 采集 conntrack 统计（仅 Linux）
// 容器环境降级处理：防止权限问题导致整个采集器失败
func (sc *SystemCollector) collectConntrackStats() error {
	if runtime.GOOS != "linux" {
		return nil // Skip on non-Linux
	}

	// 读取当前使用量
	data, err := os.ReadFile("/proc/sys/net/netfilter/nf_conntrack_count")
	if err != nil {
		if os.IsPermission(err) {
			// 容器环境可能无权限访问（/proc 只读挂载）
			log.Printf("[metrics] Conntrack monitoring disabled: %v (hint: run with --privileged or mount /proc/sys)", err)
			return nil // 静默失败，不影响其他指标
		}
		log.Printf("[metrics] Failed to read conntrack count: %v", err)
		return nil
	}

	count, _ := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
	NodeNFConntrackEntries.Set(count)

	// 读取最大值
	data, err = os.ReadFile("/proc/sys/net/netfilter/nf_conntrack_max")
	if err != nil {
		if !os.IsPermission(err) {
			log.Printf("[metrics] Failed to read conntrack max: %v", err)
		}
		return nil
	}

	max, _ := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
	NodeNFConntrackEntriesLimit.Set(max)

	return nil
}

// collectSystemMemory 采集系统内存使用率
func (sc *SystemCollector) collectSystemMemory() error {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return fmt.Errorf("failed to open /proc/meminfo: %w", err)
	}
	defer file.Close()

	var memTotal, memAvailable uint64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			// 跳过 "MemTotal:" 部分，解析数字
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				fmt.Sscanf(parts[1], "%d", &memTotal)
				memTotal = memTotal * 1024 // KB to bytes
			}
		} else if strings.HasPrefix(line, "MemAvailable:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				fmt.Sscanf(parts[1], "%d", &memAvailable)
				memAvailable = memAvailable * 1024 // KB to bytes
			}
		}
	}

	if memTotal > 0 && memAvailable > 0 {
		memUsed := memTotal - memAvailable
		memPercent := float64(memUsed) / float64(memTotal) * 100
		SystemMemoryUsagePercent.Set(memPercent)
	}

	return nil
}

// collectDiskStats 采集磁盘空间统计
func (sc *SystemCollector) collectDiskStats() error {
	paths := []string{"/var/log", "/var/lib/aria"}

	for _, path := range paths {
		var stat syscall.Statfs_t
		err := syscall.Statfs(path, &stat)
		if err != nil {
			// Path may not exist, skip silently
			continue
		}

		total := stat.Blocks * uint64(stat.Bsize)
		free := stat.Bfree * uint64(stat.Bsize)
		used := total - free
		usagePercent := float64(used) / float64(total) * 100

		DiskUsagePercent.WithLabelValues(path).Set(usagePercent)
		DiskFreeBytes.WithLabelValues(path).Set(float64(free))
	}

	return nil
}

// collectPerCoreCPU 采集 Per-Core CPU 使用率和均衡度
func (sc *SystemCollector) collectPerCoreCPU() error {
	// 仅 Linux 支持
	if runtime.GOOS != "linux" {
		return nil
	}

	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return fmt.Errorf("failed to read /proc/stat: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	coreStats := make(map[string]*cpuCoreStat)
	var coreLoads []float64

	for _, line := range lines {
		if !strings.HasPrefix(line, "cpu") || line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 8 {
			continue
		}

		// 跳过总体 CPU（cpu）
		if fields[0] == "cpu" {
			continue
		}

		// 解析 cpu0, cpu1, cpu2, ...
		coreName := fields[0]
		user, _ := strconv.ParseUint(fields[1], 10, 64)
		nice, _ := strconv.ParseUint(fields[2], 10, 64)
		system, _ := strconv.ParseUint(fields[3], 10, 64)
		idle, _ := strconv.ParseUint(fields[4], 10, 64)
		iowait, _ := strconv.ParseUint(fields[5], 10, 64)
		irq, _ := strconv.ParseUint(fields[6], 10, 64)
		softirq, _ := strconv.ParseUint(fields[7], 10, 64)

		total := user + nice + system + idle + iowait + irq + softirq

		currentStat := &cpuCoreStat{
			user:    user,
			nice:    nice,
			system:  system,
			idle:    idle,
			iowait:  iowait,
			irq:     irq,
			softirq: softirq,
			total:   total,
		}

		coreStats[coreName] = currentStat

		// 计算使用率（需要上次的数据）
		if sc.lastCPUCoreStat != nil {
			if lastStat, ok := sc.lastCPUCoreStat[coreName]; ok {
				deltaTotal := currentStat.total - lastStat.total
				if deltaTotal > 0 {
					deltaUser := currentStat.user - lastStat.user
					deltaSystem := currentStat.system - lastStat.system
					deltaSoftirq := currentStat.softirq - lastStat.softirq
					deltaIdle := currentStat.idle - lastStat.idle

					userPercent := float64(deltaUser) / float64(deltaTotal) * 100
					systemPercent := float64(deltaSystem) / float64(deltaTotal) * 100
					softirqPercent := float64(deltaSoftirq) / float64(deltaTotal) * 100
					idlePercent := float64(deltaIdle) / float64(deltaTotal) * 100

					// 更新指标
					CPUCoreUsagePercent.WithLabelValues(coreName, "user").Set(userPercent)
					CPUCoreUsagePercent.WithLabelValues(coreName, "system").Set(systemPercent)
					CPUCoreUsagePercent.WithLabelValues(coreName, "softirq").Set(softirqPercent)
					CPUCoreUsagePercent.WithLabelValues(coreName, "idle").Set(idlePercent)

					// 计算总负载（用于均衡度计算）
					coreLoad := 100.0 - idlePercent
					coreLoads = append(coreLoads, coreLoad)
				}
			}
		}
	}

	// 保存当前状态
	sc.lastCPUCoreStat = coreStats

	// 计算均衡度评分（标准差）
	if len(coreLoads) > 1 {
		balanceScore := calculateBalanceScore(coreLoads)
		CPUBalanceScore.Set(balanceScore)
	}

	return nil
}

// calculateBalanceScore 计算均衡度评分（0-100，越高越均衡）
func calculateBalanceScore(loads []float64) float64 {
	if len(loads) == 0 {
		return 100
	}

	// 计算平均值
	var sum float64
	for _, load := range loads {
		sum += load
	}
	mean := sum / float64(len(loads))

	// 计算标准差
	var variance float64
	for _, load := range loads {
		diff := load - mean
		variance += diff * diff
	}
	variance /= float64(len(loads))
	stdDev := math.Sqrt(variance)

	// 转换为评分（标准差越小，评分越高）
	// 假设标准差 > 30 时评分为 0
	score := 100 - (stdDev * 3.33)
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	return score
}
