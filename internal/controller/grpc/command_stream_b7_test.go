package grpc

import (
	"testing"
	"time"
)

func TestCommandStreamIdlePollBackoffIncreasesAndCaps(t *testing.T) {
	base := 10 * time.Millisecond
	maximum := 80 * time.Millisecond

	if got := commandStreamIdlePollInterval(base, maximum, 0); got != base {
		t.Fatalf("first idle poll interval = %v, want %v", got, base)
	}
	second := commandStreamIdlePollInterval(base, maximum, 1)
	third := commandStreamIdlePollInterval(base, maximum, 2)
	if !(base < second && second < third) {
		t.Fatalf("expected idle intervals to increase, got base=%v second=%v third=%v", base, second, third)
	}
	if got := commandStreamIdlePollInterval(base, maximum, 20); got != maximum {
		t.Fatalf("expected idle poll interval to cap at %v, got %v", maximum, got)
	}
}
