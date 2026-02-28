package schematic_test

import (
	"bytes"
	"testing"

	"github.com/go-mclib/go-schematic"
)

func TestVarintRoundTrip(t *testing.T) {
	values := []int{0, 1, 127, 128, 255, 256, 16383, 16384, 2097151}
	for _, v := range values {
		var buf []byte
		buf = schematic.AppendVarint(buf, v)
		got, n := schematic.DecodeVarint(buf)
		if got != v || n != len(buf) {
			t.Errorf("varint(%d): got %d (read %d bytes), encoded as %d bytes", v, got, n, len(buf))
		}
	}
}

func TestNewSchematic(t *testing.T) {
	s := schematic.New(3, 4, 5)
	if s.Width != 3 || s.Height != 4 || s.Length != 5 {
		t.Fatalf("dimensions = %d,%d,%d, want 3,4,5", s.Width, s.Height, s.Length)
	}

	// everything should be air
	for y := range int(s.Height) {
		for z := range int(s.Length) {
			for x := range int(s.Width) {
				if got := s.BlockAt(x, y, z); got != "minecraft:air" {
					t.Fatalf("BlockAt(%d,%d,%d) = %q, want air", x, y, z, got)
				}
			}
		}
	}
}

func TestSetBlockAndPalette(t *testing.T) {
	s := schematic.New(2, 2, 2)
	s.SetBlock(0, 0, 0, "minecraft:stone")
	s.SetBlock(1, 0, 0, "minecraft:oak_planks")
	s.SetBlock(1, 1, 1, "minecraft:stone")

	if got := s.BlockAt(0, 0, 0); got != "minecraft:stone" {
		t.Errorf("(0,0,0) = %q, want stone", got)
	}
	if got := s.BlockAt(1, 0, 0); got != "minecraft:oak_planks" {
		t.Errorf("(1,0,0) = %q, want oak_planks", got)
	}
	if got := s.BlockAt(1, 1, 1); got != "minecraft:stone" {
		t.Errorf("(1,1,1) = %q, want stone", got)
	}
	if got := s.BlockAt(0, 1, 0); got != "minecraft:air" {
		t.Errorf("(0,1,0) = %q, want air", got)
	}

	// palette should have 3 entries: air, stone, oak_planks
	if len(s.Palette) != 3 {
		t.Errorf("palette size = %d, want 3", len(s.Palette))
	}
}

func TestRoundTrip(t *testing.T) {
	original := schematic.New(4, 3, 5)
	original.DataVersion = 4671
	original.Offset = [3]int32{-2, 0, -2}

	original.SetBlock(0, 0, 0, "minecraft:stone")
	original.SetBlock(1, 0, 0, "minecraft:stone")
	original.SetBlock(2, 0, 0, "minecraft:stone")
	original.SetBlock(0, 1, 0, "minecraft:oak_planks")
	original.SetBlock(3, 2, 4, "minecraft:glass")

	var buf bytes.Buffer
	if err := original.Save(&buf); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	decoded, err := schematic.ReadFrom(&buf)
	if err != nil {
		t.Fatalf("ReadFrom() error = %v", err)
	}

	if decoded.Width != original.Width || decoded.Height != original.Height || decoded.Length != original.Length {
		t.Fatalf("dimensions = %d,%d,%d, want %d,%d,%d",
			decoded.Width, decoded.Height, decoded.Length,
			original.Width, original.Height, original.Length)
	}
	if decoded.Offset != original.Offset {
		t.Errorf("offset = %v, want %v", decoded.Offset, original.Offset)
	}
	if decoded.DataVersion != original.DataVersion {
		t.Errorf("DataVersion = %d, want %d", decoded.DataVersion, original.DataVersion)
	}

	for y := range int(original.Height) {
		for z := range int(original.Length) {
			for x := range int(original.Width) {
				want := original.BlockAt(x, y, z)
				got := decoded.BlockAt(x, y, z)
				if got != want {
					t.Errorf("BlockAt(%d,%d,%d) = %q, want %q", x, y, z, got, want)
				}
			}
		}
	}
}

func TestIndex(t *testing.T) {
	s := schematic.New(4, 3, 5)
	// (y * Length + z) * Width + x
	if got := s.Index(2, 1, 3); got != (1*5+3)*4+2 {
		t.Errorf("Index(2,1,3) = %d, want %d", got, (1*5+3)*4+2)
	}
}
