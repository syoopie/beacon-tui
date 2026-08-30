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

func TestCheckPortReportsRivalSpecs(t *testing.T) {
	specs := []server.Spec{
		{ID: "survival", Port: 25565},
		{ID: "creative", Port: 25565},
		{ID: "skyblock", Port: 25570},
	}
	block := CheckPort(25565, "survival", specs)
	if block.OSListener {
		t.Errorf("unexpected OS listener on 25565 during test")
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
