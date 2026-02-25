package eBPF

import (
	"testing"
)

func TestBucketStateStructure(t *testing.T) {
	// This test validates that our BucketState struct has the expected fields
	// for compatibility with the C structure
	bucket := BucketState{
		Lock:     0,
		Rate:     1000,
		Tokens:   2000,
		LastTime: 3000,
		Capacity: 4000,
	}

	if bucket.Rate != 1000 {
		t.Errorf("Expected Rate to be 1000, got %d", bucket.Rate)
	}

	if bucket.Tokens != 2000 {
		t.Errorf("Expected Tokens to be 2000, got %d", bucket.Tokens)
	}
}

func TestPeerPairStructure(t *testing.T) {
	// This test validates that our PeerPair struct has the expected fields
	peer := PeerPair{
		SrcIP: 1234,
		DstIP: 5678,
	}

	if peer.SrcIP != 1234 {
		t.Errorf("Expected SrcIP to be 1234, got %d", peer.SrcIP)
	}

	if peer.DstIP != 5678 {
		t.Errorf("Expected DstIP to be 5678, got %d", peer.DstIP)
	}
}

func TestACLValueStructure(t *testing.T) {
	// This test validates that our ACLValue struct has the expected fields
	aclVal := ACLValue{
		Action: 1,
		Port:   8080,
	}

	if aclVal.Action != 1 {
		t.Errorf("Expected Action to be 1, got %d", aclVal.Action)
	}

	if aclVal.Port != 8080 {
		t.Errorf("Expected Port to be 8080, got %d", aclVal.Port)
	}
}
