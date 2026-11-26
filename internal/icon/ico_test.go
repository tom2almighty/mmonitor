package icon

import (
	"encoding/binary"
	"os"
	"testing"
)

func TestGenerateICOFromSVG(t *testing.T) {
	svg, err := os.ReadFile("../../assets/logo.svg")
	if err != nil {
		t.Fatalf("read logo.svg: %v", err)
	}
	ico, err := GenerateICOFromSVG(svg)
	if err != nil {
		t.Fatalf("GenerateICOFromSVG: %v", err)
	}
	if len(ico) < 100 {
		t.Fatalf("expected non-trivial ico bytes, got %d", len(ico))
	}
	// Verify ICONDIR header: reserved=0, type=1, count=4
	if binary.LittleEndian.Uint16(ico[0:2]) != 0 ||
		binary.LittleEndian.Uint16(ico[2:4]) != 1 ||
		binary.LittleEndian.Uint16(ico[4:6]) != 4 {
		t.Fatalf("invalid ICO header: %x", ico[:6])
	}
}
