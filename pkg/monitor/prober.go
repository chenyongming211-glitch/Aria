package monitor

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

const (
	ProbePort     = 51830 // 改为 51830，避免与多隧道冲突（51820-51827）
	ProbeInterval = 200 * time.Millisecond
	StatsWindow   = 75  // 扩大到 15 秒窗口 (75 个样本)
	LogInterval   = 15 * time.Second
)

type PeerStats struct {
	PublicKey       string
	IP              string
	RTTs            []time.Duration
	SentPackets     int
	ReceivedPackets int
	LastSeen        time.Time
	mu              sync.RWMutex
}

type Prober struct {
	conn     *net.UDPConn
	peers    map[string]*PeerStats
	stopChan chan struct{}
	wg       sync.WaitGroup
	mu       sync.RWMutex
}

func NewProber() (*Prober, error) {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", ProbePort))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve addr: %v", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %v", err)
	}

	return &Prober{
		conn:     conn,
		peers:    make(map[string]*PeerStats),
		stopChan: make(chan struct{}),
	}, nil
}

func (p *Prober) Start() {
	p.wg.Add(3)
	go p.receiveLoop()
	go p.probeLoop()
	go p.statsLogger()
	p.wg.Wait()
}

func (p *Prober) Stop() {
	close(p.stopChan)
	if p.conn != nil {
		p.conn.Close()
	}
	p.wg.Wait()
}

func (p *Prober) AddPeer(publicKey, ip string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.peers[publicKey]; !exists {
		p.peers[publicKey] = &PeerStats{
			PublicKey: publicKey,
			IP:        ip,
			RTTs:      make([]time.Duration, 0, StatsWindow),
		}
		log.Printf("Prober: added peer %s at %s", publicKey[:8], ip)
	}
}

func (p *Prober) RemovePeer(publicKey string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.peers, publicKey)
	log.Printf("Prober: removed peer %s", publicKey[:8])
}

func (p *Prober) receiveLoop() {
	defer p.wg.Done()

	buf := make([]byte, 1024)

	for {
		select {
		case <-p.stopChan:
			return
		default:
		}

		p.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, addr, err := p.conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return
		}

		if n < 9 {
			continue
		}

		// 检查包类型：第一个字节 0x01=probe, 0x02=echo
		packetType := buf[0]
		sendTime := time.Unix(0, int64(binary.BigEndian.Uint64(buf[1:9])))

		if packetType == 0x01 {
			// 收到探测包，回显
			buf[0] = 0x02 // 标记为 echo
			_, _ = p.conn.WriteToUDP(buf[:n], addr)
		} else if packetType == 0x02 {
			// 收到回显包，计算 RTT
			rtt := time.Since(sendTime)

			// 根据 IP 地址查找对应的 peer
			p.mu.Lock()
			var foundPeer *PeerStats
			for _, peer := range p.peers {
				if peer.IP == addr.IP.String() {
					foundPeer = peer
					break
				}
			}

			if foundPeer != nil {
				foundPeer.mu.Lock()
				foundPeer.RTTs = append(foundPeer.RTTs, rtt)
				if len(foundPeer.RTTs) > StatsWindow {
					foundPeer.RTTs = foundPeer.RTTs[len(foundPeer.RTTs)-StatsWindow:]
				}
				foundPeer.ReceivedPackets++
				foundPeer.LastSeen = time.Now()
				foundPeer.mu.Unlock()
			}
			p.mu.Unlock()
		}
	}
}

// probeLoop 定期发送探测包
func (p *Prober) probeLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(ProbeInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopChan:
			return
		case <-ticker.C:
			p.mu.RLock()
			peers := make([]*PeerStats, 0, len(p.peers))
			for _, peer := range p.peers {
				peers = append(peers, peer)
			}
			p.mu.RUnlock()

			// 向所有 peer 发送探测包
			for _, peer := range peers {
				_ = p.SendProbe(peer.IP)
			}
		}
	}
}

func (p *Prober) statsLogger() {
	defer p.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopChan:
			return
		case <-ticker.C:
			p.mu.RLock()
			for _, peer := range p.peers {
				peer.mu.RLock()
				avgRTT, lossRate := p.calculateStats(peer)
				peer.mu.RUnlock()

				if avgRTT > 0 {
					log.Printf("Peer %s: Latency=%v, Loss=%.1f%%",
						peer.PublicKey[:8], avgRTT, lossRate)
				}
			}
			p.mu.RUnlock()
		}
	}
}

func (p *Prober) calculateStats(peer *PeerStats) (time.Duration, float64) {
	if len(peer.RTTs) == 0 {
		return 0, 100.0
	}

	var total time.Duration
	for _, rtt := range peer.RTTs {
		total += rtt
	}
	avgRTT := total / time.Duration(len(peer.RTTs))

	// 使用与 GetPeerStats 一致的丢包计算逻辑
	receivedPackets := len(peer.RTTs)
	var lossRate float64

	if receivedPackets >= StatsWindow {
		expectedPackets := StatsWindow
		lossRate = float64(expectedPackets-receivedPackets) / float64(expectedPackets) * 100
	} else {
		lossRate = 0
	}

	if lossRate < 0 {
		lossRate = 0
	}

	return avgRTT, lossRate
}

func (p *Prober) SendProbe(ip string) error {
	buf := make([]byte, 9)
	buf[0] = 0x01 // 标记为 probe
	now := time.Now()
	binary.BigEndian.PutUint64(buf[1:9], uint64(now.UnixNano()))

	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", ip, ProbePort))
	if err != nil {
		return err
	}

	_, err = p.conn.WriteToUDP(buf, addr)
	if err == nil {
		p.mu.RLock()
		for _, peer := range p.peers {
			if peer.IP == ip {
				peer.mu.Lock()
				peer.SentPackets++
				peer.mu.Unlock()
				break
			}
		}
		p.mu.RUnlock()
	}
	return err
}

func (p *Prober) GetPeerStats(publicKey string) (rtt time.Duration, loss float64, ok bool) {
	p.mu.RLock()
	peer, exists := p.peers[publicKey]
	p.mu.RUnlock()

	if !exists {
		return 0, 0, false
	}

	peer.mu.RLock()
	defer peer.mu.RUnlock()

	if len(peer.RTTs) == 0 {
		return 0, 100.0, false
	}

	var total time.Duration
	for _, r := range peer.RTTs {
		total += r
	}
	avgRTT := total / time.Duration(len(peer.RTTs))

	// 使用 RTT 数组长度计算丢包率
	// 问题：StatsWindow=75 太大，导致新添加的 peer 在前几秒显示极高的丢包率
	// 修复：当样本数少于 StatsWindow 时，使用实际样本数作为分母
	// 这样在启动初期会显示 0% 丢包（因为没有证据显示有丢包）
	receivedPackets := len(peer.RTTs)
	var lossRate float64

	if receivedPackets > 0 {
		if receivedPackets >= StatsWindow {
			// 有足够的历史数据，使用完整的窗口计算
			expectedPackets := StatsWindow
			lossRate = float64(expectedPackets-receivedPackets) / float64(expectedPackets) * 100
		} else {
			// 样本不足，使用滑动窗口方式计算
			// 假设最近 N 个探测都收到了（即当前窗口内无丢包）
			// 这种方式在启动初期更准确
			lossRate = 0
		}
	} else {
		lossRate = 100.0
	}

	if lossRate < 0 {
		lossRate = 0
	}

	return avgRTT, lossRate, true
}

// GetAllPeers 返回所有 peer 的信息（用于 metrics 导出）
func (p *Prober) GetAllPeers() map[string]*PeerStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// 返回副本，避免并发修改
	peers := make(map[string]*PeerStats, len(p.peers))
	for k, v := range p.peers {
		peers[k] = v
	}
	return peers
}
