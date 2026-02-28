# go-schematic

Go library for reading and writing [Sponge Schematic](https://github.com/SpongePowered/Schematic-Specification) v2/v3 files (`.schem`), as used by WorldEdit for Minecraft 1.13+.

Blocks are stored as palette-indexed varint arrays inside a gzip-compressed NBT structure.

## Install

```sh
go get github.com/go-mclib/go-schematic
```

## Usage

### Create a new schematic

```go
s := schematic.New(16, 8, 16) // width, height, length

s.SetBlock(0, 0, 0, "minecraft:stone")
s.SetBlock(1, 0, 0, "minecraft:oak_planks")

block := s.BlockAt(0, 0, 0) // "minecraft:stone"
```

### Save to file

```go
f, _ := os.Create("build.schem")
defer f.Close()

if err := s.Save(f); err != nil {
    log.Fatal(err)
}
```

### Load from file

`ReadFrom` auto-detects v2 and v3 schematics:

```go
f, _ := os.Open("build.schem")
defer f.Close()

s, err := schematic.ReadFrom(f)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("size: %dx%dx%d\n", s.Width, s.Height, s.Length)
```

### Block states

Block state strings follow Minecraft's format: `namespace:block[property=value,...]`. Properties are optional and describe variants like orientation, open/closed state, etc.

```go
// simple block
s.SetBlock(0, 0, 0, "minecraft:stone")

// block with state properties
s.SetBlock(1, 0, 0, "minecraft:oak_stairs[facing=east,half=bottom,shape=straight]")
s.SetBlock(2, 0, 0, "minecraft:oak_door[facing=north,half=lower,open=false]")
s.SetBlock(3, 0, 0, "minecraft:redstone_wire[power=15]")

// reading back includes the full state string
fmt.Println(s.BlockAt(1, 0, 0)) // "minecraft:oak_stairs[facing=east,half=bottom,shape=straight]"
```

The palette is managed automatically -- new states are added on first use, and `BlockAt` returns the exact string that was set.

### Block entities and entities

Block entities (chests, signs, etc.) and entities (mobs, items, etc.) are preserved during read/write:

```go
for _, be := range s.BlockEntities {
    fmt.Printf("%s at %v\n", be.ID, be.Pos)
}

for _, e := range s.Entities {
    fmt.Printf("%s at %v\n", e.ID, e.Pos)
}
```

## Types

| Type | Description |
| ---- | ----------- |
| `Schematic` | Main struct holding dimensions, palette, block data, and entities |
| `BlockEntity` | Tile entity with position, ID, and NBT data |
| `Entity` | Entity with float position, ID, and NBT data |

## License

MIT
