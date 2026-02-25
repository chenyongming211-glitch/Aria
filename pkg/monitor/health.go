package monitor

import (
	"log"
	"sync"
	"time"

	"aria/pkg/route"
)

const (
	DefaultLossThreshold = 3
	DefaultRTTThreshold  = 200 * time.Millisecond
	DefaultRecoveryDelay = 10 * time.Second
	DefaultCheckInterval = 1 * time.Second
)

type PeerState struct {
	PublicKey     string
	IP            string
	VPCID         string
	IsHealthy     bool
	LastFailure   time.Time
	FailureCount  int
	RecoveryTimer *time.Timer
}

type HealthManager struct {
	prober      *Prober
	routeEngine *route.Engine
	peers       map[string]*PeerState
	mu          sync.RWMutex
	stopChan    chan struct{}

	lossThreshold int
	rttThreshold  time.Duration
	recoveryDelay time.Duration

	eventChan chan HealthEvent
}

type HealthEvent struct {
	Type      string
	PublicKey string
	IP        string
	Timestamp time.Time
}

func NewHealthManager(prober *Prober, routeEngine *route.Engine) *HealthManager {
	return &HealthManager{
		prober:        prober,
		routeEngine:   routeEngine,
		peers:         make(map[string]*PeerState),
		stopChan:      make(chan struct{}),
		lossThreshold: DefaultLossThreshold,
		rttThreshold:  DefaultRTTThreshold,
		recoveryDelay: DefaultRecoveryDelay,
		eventChan:     make(chan HealthEvent, 100),
	}
}

func (h *HealthManager) Start() {
	go h.monitorLoop()
	go h.eventHandler()
	log.Printf("HealthManager: started with thresholds - Loss>=%d, RTT>%v",
		h.lossThreshold, h.rttThreshold)
}

func (h *HealthManager) Stop() {
	// Close stop channel to signal all goroutines to exit
	close(h.stopChan)

	// Safely stop all recovery timers
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, peer := range h.peers {
		if peer.RecoveryTimer != nil {
			if !peer.RecoveryTimer.Stop() {
				// Timer already fired, drain channel to prevent leak
				select {
				case <-peer.RecoveryTimer.C:
				default:
				}
			}
			peer.RecoveryTimer = nil
		}
	}
}

func (h *HealthManager) AddPeer(publicKey, ip, vpcID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.peers[publicKey]; !exists {
		h.peers[publicKey] = &PeerState{
			PublicKey: publicKey,
			IP:        ip,
			VPCID:     vpcID,
			IsHealthy: true,
		}
		h.prober.AddPeer(publicKey, ip)
		log.Printf("HealthManager: added peer %s at %s", publicKey[:8], ip)
	}
}

func (h *HealthManager) RemovePeer(publicKey string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if peer, exists := h.peers[publicKey]; exists {
		if peer.RecoveryTimer != nil {
			peer.RecoveryTimer.Stop()
		}
		delete(h.peers, publicKey)
		h.prober.RemovePeer(publicKey)
		log.Printf("HealthManager: removed peer %s", publicKey[:8])
	}
}

func (h *HealthManager) monitorLoop() {
	ticker := time.NewTicker(DefaultCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-h.stopChan:
			return
		case <-ticker.C:
			h.checkPeers()
		}
	}
}

func (h *HealthManager) checkPeers() {
	// Collect peer public keys under lock
	h.mu.RLock()
	publicKeys := make([]string, 0, len(h.peers))
	for pubKey := range h.peers {
		publicKeys = append(publicKeys, pubKey)
	}
	h.mu.RUnlock()

	// Check each peer's stats without holding the lock
	type peerStats struct {
		publicKey string
		ip        string
		rtt       time.Duration
		loss      float64
		ok        bool
	}
	stats := make([]peerStats, 0, len(publicKeys))
	for _, pubKey := range publicKeys {
		// Get IP from peer state (need to lock briefly)
		h.mu.RLock()
		var ip string
		if peer, ok := h.peers[pubKey]; ok {
			ip = peer.IP
		}
		h.mu.RUnlock()

		rtt, loss, ok := h.prober.GetPeerStats(pubKey)
		stats = append(stats, peerStats{
			publicKey: pubKey,
			ip:        ip,
			rtt:       rtt,
			loss:      loss,
			ok:        ok,
		})
	}

	// Process unhealthy peers first (need to remove route)
	for _, s := range stats {
		if !s.ok || s.ip == "" {
			continue
		}

		if s.loss >= float64(h.lossThreshold) || s.rtt > h.rttThreshold {
			h.mu.Lock()
			peer, exists := h.peers[s.publicKey]
			if exists && peer.IsHealthy {
				peer.IsHealthy = false
				peer.LastFailure = time.Now()
				peer.FailureCount++

				event := HealthEvent{
					Type:      "LinkDown",
					PublicKey: peer.PublicKey,
					IP:        peer.IP,
					Timestamp: time.Now(),
				}
				select {
				case h.eventChan <- event:
				default:
				}

				log.Printf("HealthManager: peer %s marked unhealthy (Failure #%d, RTT=%v, Loss=%.1f%%)",
					peer.PublicKey[:8], peer.FailureCount, s.rtt, s.loss)
				h.mu.Unlock()

				// Route operation outside lock
				if err := h.routeEngine.RemoveVPNRoute(s.ip + "/32"); err != nil {
					log.Printf("HealthManager: failed to remove VPN route: %v", err)
				}
			} else {
				h.mu.Unlock()
			}
		}
	}

	// Process recovery scheduling (outside the above loop to avoid deadlock)
	for _, s := range stats {
		if !s.ok {
			continue
		}

		if s.loss < float64(h.lossThreshold) && s.rtt <= h.rttThreshold {
			h.mu.Lock()
			peer, exists := h.peers[s.publicKey]
			if exists && !peer.IsHealthy {
				log.Printf("HealthManager: scheduling recovery for peer %s (RTT=%v, Loss=%.1f%%)",
					peer.PublicKey[:8], s.rtt, s.loss)
				h.scheduleRecoveryLocked(peer)
			}
			h.mu.Unlock()
		}
	}
}

func (h *HealthManager) markUnhealthy(peer *PeerState, rtt time.Duration, loss float64) {
	h.mu.Lock()
	peer.IsHealthy = false
	peer.LastFailure = time.Now()
	peer.FailureCount++
	h.mu.Unlock()

	event := HealthEvent{
		Type:      "LinkDown",
		PublicKey: peer.PublicKey,
		IP:        peer.IP,
		Timestamp: time.Now(),
	}

	select {
	case h.eventChan <- event:
	default:
	}

	log.Printf("HealthManager: peer %s marked unhealthy (Failure #%d, RTT=%v, Loss=%.1f%%)",
		peer.PublicKey[:8], peer.FailureCount, rtt, loss)

	if err := h.routeEngine.RemoveVPNRoute(peer.IP + "/32"); err != nil {
		log.Printf("HealthManager: failed to remove VPN route: %v", err)
	}
}

func (h *HealthManager) scheduleRecovery(peer *PeerState) {
	h.mu.Lock()

	// 如果已经有恢复定时器在运行，不要重复创建
	if peer.RecoveryTimer != nil {
		h.mu.Unlock()
		return
	}

	log.Printf("HealthManager: recovery timer started for peer %s (will recover in %v)",
		peer.PublicKey[:8], h.recoveryDelay)

	peer.RecoveryTimer = time.AfterFunc(h.recoveryDelay, func() {
		h.mu.Lock()
		peer.IsHealthy = true
		peer.RecoveryTimer = nil  // 清除定时器引用
		h.mu.Unlock()

		event := HealthEvent{
			Type:      "LinkUp",
			PublicKey: peer.PublicKey,
			IP:        peer.IP,
			Timestamp: time.Now(),
		}

		select {
		case h.eventChan <- event:
		default:
		}

		log.Printf("HealthManager: peer %s recovered, restoring VPN route", peer.PublicKey[:8])

		if err := h.routeEngine.AddVPNRoute(peer.IP + "/32"); err != nil {
			log.Printf("HealthManager: failed to restore VPN route: %v", err)
		}
	})

	h.mu.Unlock()
}

// scheduleRecoveryLocked schedules recovery for a peer (caller must hold h.mu)
func (h *HealthManager) scheduleRecoveryLocked(peer *PeerState) {
	// 如果已经有恢复定时器在运行，不要重复创建
	if peer.RecoveryTimer != nil {
		return
	}

	log.Printf("HealthManager: recovery timer started for peer %s (will recover in %v)",
		peer.PublicKey[:8], h.recoveryDelay)

	peer.RecoveryTimer = time.AfterFunc(h.recoveryDelay, func() {
		h.mu.Lock()
		peer.IsHealthy = true
		peer.RecoveryTimer = nil // 清除定时器引用
		h.mu.Unlock()

		event := HealthEvent{
			Type:      "LinkUp",
			PublicKey: peer.PublicKey,
			IP:        peer.IP,
			Timestamp: time.Now(),
		}

		select {
		case h.eventChan <- event:
		default:
		}

		log.Printf("HealthManager: peer %s recovered, restoring VPN route", peer.PublicKey[:8])

		if err := h.routeEngine.AddVPNRoute(peer.IP + "/32"); err != nil {
			log.Printf("HealthManager: failed to restore VPN route: %v", err)
		}
	})
}

func (h *HealthManager) eventHandler() {
	for {
		select {
		case <-h.stopChan:
			return
		case event := <-h.eventChan:
			h.handleEvent(event)
		}
	}
}

func (h *HealthManager) handleEvent(event HealthEvent) {
	log.Printf("HealthManager: received event %s for peer %s", event.Type, event.PublicKey[:8])
}

func (h *HealthManager) GetPeerState(publicKey string) (healthy bool, failureCount int) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if peer, exists := h.peers[publicKey]; exists {
		return peer.IsHealthy, peer.FailureCount
	}
	return true, 0
}

func (h *HealthManager) IsPeerHealthy(publicKey string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if peer, exists := h.peers[publicKey]; exists {
		return peer.IsHealthy
	}
	return true
}

// GetAllPeers 返回所有 peer 的状态（用于 metrics 导出）
func (h *HealthManager) GetAllPeers() map[string]*PeerState {
	h.mu.RLock()
	defer h.mu.RUnlock()

	peers := make(map[string]*PeerState, len(h.peers))
	for k, v := range h.peers {
		peers[k] = v
	}
	return peers
}

var rtt, loss float64

func init() {
	_ = rtt
	_ = loss
}
