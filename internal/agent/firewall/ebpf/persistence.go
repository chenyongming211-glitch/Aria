package persistence

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/rlimit"
	"golang.org/x/sys/unix"
)

const (
	BasePath    = "/sys/fs/bpf/aria"
	BPFFSMagic  = 0xcafe4a11 // BPF_FS_MAGIC: 0xCAFE 4A11
)

var (
	instance *AriaMaps
	once     sync.Once
)

type MapMetadata struct {
	Name string
	Ref  **ebpf.Map
}

type AriaMaps struct {
	Ingress5TupleMap  *ebpf.Map
	Egress5TupleMap   *ebpf.Map
	IngressIPBlkMap   *ebpf.Map
	IngressPortBlkMap *ebpf.Map
	AppQoSMap         *ebpf.Map
	RuleFlowTable     *ebpf.Map
}

func (m *AriaMaps) getMapList() []MapMetadata {
	return []MapMetadata{
		{"ingress_5tuple_map", &m.Ingress5TupleMap},
		{"egress_5tuple_map", &m.Egress5TupleMap},
		{"ingress_ip_blk_map", &m.IngressIPBlkMap},
		{"ingress_port_blk_map", &m.IngressPortBlkMap},
		{"app_qos_map", &m.AppQoSMap},
		{"rule_flow_table", &m.RuleFlowTable},
	}
}

// LoadOrPinAllMaps 采用单例设计，确保 Agent 运行期间 Map 句柄的唯一性与稳定性
func LoadOrPinAllMaps(spec *ebpf.CollectionSpec) (*AriaMaps, error) {
	var err error
	once.Do(func() {
		if err = rlimit.RemoveMemlock(); err != nil {
			err = fmt.Errorf("memlock rlimit 移除失败: %w", err)
			return
		}

		// 1. 挂载点强检查：杜绝在非 BPF 文件系统下产生脏文件
		if err = checkBPFFSMounted(); err != nil {
			return
		}

		if err = os.MkdirAll(BasePath, 0755); err != nil {
			err = fmt.Errorf("无法创建 BPF 持久化目录: %w", err)
			return
		}

		res := &AriaMaps{}
		var opened []*ebpf.Map

		// ✅ 修正点：直接访问 spec.Maps 字段，而不是调用 Maps() 方法
		maps := spec.Maps
		if maps == nil {
			err = fmt.Errorf("ELF spec 中的 maps 定义为空")
			return
		}

		for _, meta := range res.getMapList() {
			s, ok := maps[meta.Name]
			if !ok {
				rollback(opened)
				err = fmt.Errorf("ELF spec 缺失 map 定义: %s", meta.Name)
				return
			}

			m, e := loadOrPinSingle(meta.Name, s)
			if e != nil {
				rollback(opened)
				err = fmt.Errorf("Map [%s] 初始化失败: %w", meta.Name, e)
				return
			}
			*meta.Ref = m
			opened = append(opened, m)
		}
		instance = res
	})
	return instance, err
}

func loadOrPinSingle(name string, spec *ebpf.MapSpec) (*ebpf.Map, error) {
	path := filepath.Join(BasePath, name)

	// 1. 尝试"继承"内核已固化的 Map 状态
	m, err := ebpf.LoadPinnedMap(path, &ebpf.LoadPinOptions{})
	if err == nil {
		info, iErr := m.Info()
		// 规格强校验：Key/Value 大小及 Map 类型必须完全匹配
		if iErr == nil && info.KeySize == spec.KeySize &&
		   info.ValueSize == spec.ValueSize && info.Type == spec.Type {
			log.Printf("Successfully inherited pinned map: %s", path)
			return m, nil
		}

		log.Printf("⚠️ Map [%s] 规格变更，执行重建...", name)
		m.Close()
	} else if !os.IsNotExist(err) {
		// 采纳建议：非"不存在"类错误（如权限拦截）直接阻断，防止静默失败
		return nil, fmt.Errorf("访问持久化文件异常 [%s]: %w", path, err)
	}

	// 2. 清理旧 Pin 文件（原子化尝试）
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("清理旧持久化文件失败，无法重建 [%s]: %w", path, err)
	}

	// 3. 重新"落户"内核
	m, err = ebpf.NewMap(spec)
	if err != nil {
		return nil, fmt.Errorf("创建新 Map 失败: %w", err)
	}

	if err = m.Pin(path); err != nil {
		m.Close() // 避免产生无法落户的孤儿 FD
		return nil, fmt.Errorf("Map 固化挂载失败 [%s]: %w", path, err)
	}

	log.Printf("Created and pinned new map: %s", path)
	return m, nil
}

// checkBPFFSMounted 验证 /sys/fs/bpf 的 Magic Number
func checkBPFFSMounted() error {
	var st unix.Statfs_t
	if err := unix.Statfs("/sys/fs/bpf", &st); err != nil {
		return fmt.Errorf("无法探测 /sys/fs/bpf 挂载状态: %w", err)
	}
	if uint32(st.Type) != uint32(BPFFSMagic) {
		return fmt.Errorf("/sys/fs/bpf 未挂载 BPF 文件系统 (Magic: 0x%x)", st.Type)
	}
	return nil
}

func rollback(maps []*ebpf.Map) {
	for _, mp := range maps {
		if mp != nil { _ = mp.Close() }
	}
}

// Close closes all maps in the AriaMaps struct
func (m *AriaMaps) Close() error {
	var errs []error

	closers := []struct {
		name string
		mapObj *ebpf.Map
	}{
		{"ingress_5tuple_map", m.Ingress5TupleMap},
		{"egress_5tuple_map", m.Egress5TupleMap},
		{"ingress_ip_blk_map", m.IngressIPBlkMap},
		{"ingress_port_blk_map", m.IngressPortBlkMap},
		{"app_qos_map", m.AppQoSMap},
		{"rule_flow_table", m.RuleFlowTable},
	}

	for _, closer := range closers {
		if closer.mapObj != nil {
			if err := closer.mapObj.Close(); err != nil {
				log.Printf("Error closing map %s: %v", closer.name, err)
				errs = append(errs, fmt.Errorf("failed to close %s: %v", closer.name, err))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors occurred while closing maps: %v", errs)
	}

	return nil
}

// ForceCleanup removes all pinned maps from the filesystem
// This should only be called during product uninstallation
func ForceCleanup() error {
	log.Println("Starting force cleanup of all pinned maps")

	// List of all map names that we pin
	mapNames := []string{
		"ingress_5tuple_map",
		"egress_5tuple_map",
		"ingress_ip_blk_map",
		"ingress_port_blk_map",
		"app_qos_map",
		"rule_flow_table",
	}

	var errs []error

	for _, mapName := range mapNames {
		pinPath := filepath.Join(BasePath, mapName)

		if err := os.Remove(pinPath); err != nil {
			if os.IsNotExist(err) {
				log.Printf("Map %s does not exist, skipping", pinPath)
			} else {
				log.Printf("Failed to remove pinned map %s: %v", pinPath, err)
				errs = append(errs, fmt.Errorf("failed to remove %s: %v", pinPath, err))
			}
		} else {
			log.Printf("Successfully removed pinned map: %s", pinPath)
		}
	}

	// Attempt to remove the base directory as well
	if err := os.Remove(BasePath); err != nil {
		if !os.IsNotExist(err) {
			log.Printf("Failed to remove base directory %s: %v", BasePath, err)
			errs = append(errs, fmt.Errorf("failed to remove base directory: %v", err))
		}
	} else {
		log.Printf("Successfully removed base directory: %s", BasePath)
	}

	if len(errs) > 0 {
		return fmt.Errorf("cleanup completed with %d errors: %v", len(errs), errs)
	}

	log.Println("Force cleanup completed successfully")
	return nil
}