// Package schematic implements reading and writing of Sponge Schematic v2/v3 files.
//
// These are the .schem files used by WorldEdit for Minecraft 1.13+.
// The format stores blocks as palette-indexed varint arrays inside
// a gzip-compressed NBT structure.
package schematic

import (
	"compress/gzip"
	"fmt"
	"io"
	"maps"
	"slices"

	"github.com/go-mclib/protocol/nbt"
)

type Schematic struct {
	Version       int32
	DataVersion   int32
	Width         int16
	Height        int16
	Length        int16
	Offset        [3]int32
	Palette       []string // index → block state string
	Blocks        []int32  // flat array of palette indices, len = Width*Height*Length
	BlockEntities []BlockEntity
	Entities      []Entity
}

type BlockEntity struct {
	Pos  [3]int32
	ID   string
	Data nbt.Compound
}

type Entity struct {
	Pos  [3]float64
	ID   string
	Data nbt.Compound
}

// New creates an empty schematic filled with air.
func New(width, height, length int16) *Schematic {
	s := &Schematic{
		Version:     3,
		DataVersion: 4671, // 1.21.11
		Width:       width,
		Height:      height,
		Length:      length,
		Palette:     []string{"minecraft:air"},
		Blocks:      make([]int32, int(width)*int(height)*int(length)),
	}
	return s
}

// Index returns the flat array index for the given (x, y, z) position.
func (s *Schematic) Index(x, y, z int) int {
	return (y*int(s.Length)+z)*int(s.Width) + x
}

// BlockAt returns the block state string at (x, y, z).
func (s *Schematic) BlockAt(x, y, z int) string {
	idx := s.Index(x, y, z)
	return s.Palette[s.Blocks[idx]]
}

// SetBlock sets the block at (x, y, z), adding to palette if needed.
func (s *Schematic) SetBlock(x, y, z int, state string) {
	palIdx := slices.Index(s.Palette, state)
	if palIdx == -1 {
		palIdx = len(s.Palette)
		s.Palette = append(s.Palette, state)
	}
	s.Blocks[s.Index(x, y, z)] = int32(palIdx)
}

// ReadFrom reads a gzipped .schem file, auto-detecting v2 or v3.
func ReadFrom(r io.Reader) (*Schematic, error) {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("gzip: %w", err)
	}
	defer gr.Close()

	data, err := io.ReadAll(gr)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	tag, _, err := nbt.DecodeFile(data, nbt.WithMaxBytes(0))
	if err != nil {
		return nil, fmt.Errorf("nbt decode: %w", err)
	}

	root, ok := tag.(nbt.Compound)
	if !ok {
		return nil, fmt.Errorf("expected root compound, got %T", tag)
	}

	// v3 wraps everything under a "Schematic" sub-compound
	if inner := root.GetCompound("Schematic"); inner != nil {
		root = inner
	}

	version := root.GetInt("Version")
	if version < 2 || version > 3 {
		return nil, fmt.Errorf("unsupported schematic version %d (expected 2 or 3)", version)
	}

	s := &Schematic{
		Version:     version,
		DataVersion: root.GetInt("DataVersion"),
		Width:       root.GetShort("Width"),
		Height:      root.GetShort("Height"),
		Length:      root.GetShort("Length"),
	}

	if off := root.GetIntArray("Offset"); len(off) >= 3 {
		s.Offset = [3]int32{off[0], off[1], off[2]}
	}

	// blocks: v2 stores at root, v3 under "Blocks" sub-compound
	var blocksCompound nbt.Compound
	if version == 3 {
		blocksCompound = root.GetCompound("Blocks")
	} else {
		blocksCompound = root
	}
	if blocksCompound == nil {
		return nil, fmt.Errorf("missing block data")
	}

	if err := s.readPalette(blocksCompound); err != nil {
		return nil, err
	}
	if err := s.readBlockData(blocksCompound); err != nil {
		return nil, err
	}
	s.readBlockEntities(blocksCompound)
	s.readEntities(root)

	return s, nil
}

func (s *Schematic) readPalette(c nbt.Compound) error {
	// v2: "Palette", v3: "Palette"
	paletteTag := c.GetCompound("Palette")
	if paletteTag == nil {
		return fmt.Errorf("missing Palette")
	}

	maxID := int32(0)
	for _, tag := range paletteTag {
		if id, ok := tag.(nbt.Int); ok && int32(id) > maxID {
			maxID = int32(id)
		}
	}

	s.Palette = make([]string, maxID+1)
	for state, tag := range paletteTag {
		if id, ok := tag.(nbt.Int); ok {
			s.Palette[int32(id)] = state
		}
	}
	return nil
}

func (s *Schematic) readBlockData(c nbt.Compound) error {
	// v2: "BlockData", v3: "Data"
	var raw []byte
	if data := c.GetByteArray("BlockData"); data != nil {
		raw = data
	} else if data := c.GetByteArray("Data"); data != nil {
		raw = data
	} else {
		return fmt.Errorf("missing BlockData/Data")
	}

	total := int(s.Width) * int(s.Height) * int(s.Length)
	s.Blocks = make([]int32, total)

	offset := 0
	for i := range total {
		val, n := DecodeVarint(raw[offset:])
		offset += n
		s.Blocks[i] = int32(val)
	}
	return nil
}

func (s *Schematic) readBlockEntities(c nbt.Compound) {
	list := c.GetList("BlockEntities")
	for _, elem := range list.Elements {
		comp, ok := elem.(nbt.Compound)
		if !ok {
			continue
		}

		be := BlockEntity{
			ID:   comp.GetString("Id"),
			Data: make(nbt.Compound),
		}
		if pos := comp.GetIntArray("Pos"); len(pos) >= 3 {
			be.Pos = [3]int32{pos[0], pos[1], pos[2]}
		}

		for k, v := range comp {
			if k != "Id" && k != "Pos" {
				be.Data[k] = v
			}
		}
		s.BlockEntities = append(s.BlockEntities, be)
	}
}

func (s *Schematic) readEntities(root nbt.Compound) {
	// v3: entities under "Entities" compound with "Entities" list inside
	// v2: "Entities" list at root
	var list nbt.List
	if entComp := root.GetCompound("Entities"); entComp != nil {
		list = entComp.GetList("Entities")
	} else {
		list = root.GetList("Entities")
	}

	for _, elem := range list.Elements {
		comp, ok := elem.(nbt.Compound)
		if !ok {
			continue
		}

		e := Entity{
			ID:   comp.GetString("Id"),
			Data: make(nbt.Compound),
		}
		if posList := comp.GetList("Pos"); posList.Len() >= 3 {
			if d, ok := posList.Get(0).(nbt.Double); ok {
				e.Pos[0] = float64(d)
			}
			if d, ok := posList.Get(1).(nbt.Double); ok {
				e.Pos[1] = float64(d)
			}
			if d, ok := posList.Get(2).(nbt.Double); ok {
				e.Pos[2] = float64(d)
			}
		}

		for k, v := range comp {
			if k != "Id" && k != "Pos" {
				e.Data[k] = v
			}
		}
		s.Entities = append(s.Entities, e)
	}
}

// Save writes the schematic as a gzipped Sponge Schematic v3 file.
func (s *Schematic) Save(w io.Writer) error {
	// build palette compound (state → id)
	paletteCompound := make(nbt.Compound, len(s.Palette))
	for i, state := range s.Palette {
		paletteCompound[state] = nbt.Int(i)
	}

	// encode block data as varints
	blockData := s.encodeBlockData()

	// build block entities list
	beList := make([]nbt.Tag, len(s.BlockEntities))
	for i, be := range s.BlockEntities {
		comp := nbt.Compound{
			"Id":  nbt.String(be.ID),
			"Pos": nbt.IntArray{be.Pos[0], be.Pos[1], be.Pos[2]},
		}
		maps.Copy(comp, be.Data)
		beList[i] = comp
	}

	blocks := nbt.Compound{
		"Palette": paletteCompound,
		"Data":    nbt.ByteArray(blockData),
	}
	if len(beList) > 0 {
		blocks["BlockEntities"] = nbt.List{ElementType: nbt.TagCompound, Elements: beList}
	}

	// build entities list
	root := nbt.Compound{
		"Version":     nbt.Int(3),
		"DataVersion": nbt.Int(s.DataVersion),
		"Width":       nbt.Short(s.Width),
		"Height":      nbt.Short(s.Height),
		"Length":      nbt.Short(s.Length),
		"Offset":      nbt.IntArray{s.Offset[0], s.Offset[1], s.Offset[2]},
		"Blocks":      blocks,
	}

	if len(s.Entities) > 0 {
		entList := make([]nbt.Tag, len(s.Entities))
		for i, e := range s.Entities {
			comp := nbt.Compound{
				"Id": nbt.String(e.ID),
				"Pos": nbt.List{
					ElementType: nbt.TagDouble,
					Elements:    []nbt.Tag{nbt.Double(e.Pos[0]), nbt.Double(e.Pos[1]), nbt.Double(e.Pos[2])},
				},
			}
			maps.Copy(comp, e.Data)
			entList[i] = comp
		}
		root["Entities"] = nbt.Compound{
			"Entities": nbt.List{ElementType: nbt.TagCompound, Elements: entList},
		}
	}

	data, err := nbt.EncodeFile(root, "Schematic")
	if err != nil {
		return fmt.Errorf("nbt encode: %w", err)
	}

	gw := gzip.NewWriter(w)
	if _, err := gw.Write(data); err != nil {
		return fmt.Errorf("gzip write: %w", err)
	}
	return gw.Close()
}

func (s *Schematic) encodeBlockData() []byte {
	var buf []byte
	for _, id := range s.Blocks {
		buf = AppendVarint(buf, int(id))
	}
	return buf
}

// DecodeVarint reads a variable-length integer from the byte slice.
func DecodeVarint(data []byte) (value int, bytesRead int) {
	shift := 0
	for _, b := range data {
		bytesRead++
		value |= int(b&0x7F) << shift
		if b&0x80 == 0 {
			return
		}
		shift += 7
	}
	return
}

// AppendVarint appends a variable-length integer to the byte slice.
func AppendVarint(buf []byte, value int) []byte {
	for {
		b := byte(value & 0x7F)
		value >>= 7
		if value != 0 {
			b |= 0x80
		}
		buf = append(buf, b)
		if value == 0 {
			return buf
		}
	}
}
