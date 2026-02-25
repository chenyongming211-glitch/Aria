package tunnel

import (
	"testing"
)

func TestNewManager(t *testing.T) {
	mgr, err := NewManager("wg0")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	if mgr.GetInterface() != "wg0" {
		t.Errorf("Expected interface name 'wg0', got '%s'", mgr.GetInterface())
	}
}

func TestPeerConfig(t *testing.T) {
	peer := PeerConfig{
		PublicKey:           "AbCdEf1234567890AbCdEf1234567890AbCdEf12=",
		AllowedIPs:          []string{"10.0.0.2/32"},
		Endpoint:            "192.168.1.100:51820",
		PersistentKeepalive: 25,
	}

	if peer.PersistentKeepalive != 25 {
		t.Errorf("Expected PersistentKeepalive 25, got %d", peer.PersistentKeepalive)
	}

	if len(peer.AllowedIPs) != 1 {
		t.Errorf("Expected 1 allowed IP, got %d", len(peer.AllowedIPs))
	}
}

func TestSetKeys(t *testing.T) {
	mgr, _ := NewManager("wg0")
	mgr.SetKeys("privatekey123", "publickey456")

	if mgr.privateKey != "privatekey123" {
		t.Errorf("Expected private key 'privatekey123', got '%s'", mgr.privateKey)
	}

	if mgr.publicKey != "publickey456" {
		t.Errorf("Expected public key 'publickey456', got '%s'", mgr.publicKey)
	}
}
