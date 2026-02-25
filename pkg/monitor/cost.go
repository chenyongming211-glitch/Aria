package monitor

import (
	"log"
	"sync"

	"aria/pkg/route"
)

type CostOptimizer struct {
	routeEngine   *route.Engine
	selfVPCID     string
	selfRegion    string
	selfInterface string
	peerRoutes    map[string]string
	mu            sync.RWMutex
}

func NewCostOptimizer(routeEngine *route.Engine, selfVPCID, selfRegion, selfInterface string) *CostOptimizer {
	return &CostOptimizer{
		routeEngine:   routeEngine,
		selfVPCID:     selfVPCID,
		selfRegion:    selfRegion,
		selfInterface: selfInterface,
		peerRoutes:    make(map[string]string),
	}
}

func (c *CostOptimizer) OptimizePeer(peer *PeerInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if peer.VPCID == "" || peer.PrivateIP == "" {
		return
	}

	if peer.VPCID == c.selfVPCID && peer.Region == c.selfRegion {
		c.addDirectRoute(peer.PublicKey, peer.PrivateIP)
		log.Printf("CostOptimizer: peer %s in same VPC %s, using direct route",
			peer.PublicKey[:8], peer.VPCID)
	}
}

func (c *CostOptimizer) addDirectRoute(publicKey, privateIP string) {
	if _, exists := c.peerRoutes[publicKey]; !exists {
		if err := c.routeEngine.AddDirectRoute(privateIP+"/32", c.selfInterface); err != nil {
			log.Printf("CostOptimizer: failed to add direct route for %s: %v", publicKey[:8], err)
			return
		}
		c.peerRoutes[publicKey] = privateIP
	}
}

func (c *CostOptimizer) RemovePeer(publicKey string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ip, exists := c.peerRoutes[publicKey]; exists {
		delete(c.peerRoutes, publicKey)
		log.Printf("CostOptimizer: removed direct route for peer %s", publicKey[:8])
		_ = ip
	}
}

func (c *CostOptimizer) IsUsingDirectRoute(publicKey string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, exists := c.peerRoutes[publicKey]
	return exists
}

type PeerInfo struct {
	PublicKey string
	PrivateIP string
	VPCID     string
	Region    string
}
