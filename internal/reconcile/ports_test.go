package reconcile

import (
	"net"
	"testing"

	"github.com/syoopie/beacon-tui/internal/server"
)

func TestCheckPortDetectsOSListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	block := CheckPort(port, "survival", nil)
	if !block.OSListener || !block.Blocked() {
		t.Fatalf("CheckPort(%d) = %+v, want OSListener true", port, block)
	}
}

func TestCheckPortNamesRivalSpecsButDoesNotBlock(t *testing.T) {
	// A port nothing listens on, not 25565: a server running on this machine
	// would hold that one.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	specs := []server.Spec{
		{ID: "survival", Port: port},
		{ID: "creative", Port: port},
		{ID: "skyblock", Port: port + 1},
	}
	block := CheckPort(port, "survival", specs)
	if block.OSListener {
		t.Errorf("unexpected OS listener on %d during test", port)
	}
	if block.Blocked() {
		t.Errorf("a stopped rival spec must not block the start: %+v", block)
	}
	if len(block.Specs) != 1 || block.Specs[0] != "creative" {
		t.Fatalf("rival specs = %v, want [creative]", block.Specs)
	}
}

func TestCheckPortFreeWhenNobodyClaims(t *testing.T) {
	block := CheckPort(1, "survival", nil)
	if block.Blocked() {
		t.Fatalf("port 0 reported blocked: %+v", block)
	}
}
