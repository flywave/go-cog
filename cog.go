package cog

import (
	"fmt"
	"io"
	"os"
	"sort"
)

type CogWriter struct {
	Writer
	tiles []*TileLayer
}

func Write(fileName string, tiles []*TileLayer, bigtiff bool) error {
	w := &CogWriter{tiles: tiles, Writer: Writer{bigtiff: bigtiff, enc: tiffByteOrder}}

	defer w.Close()

	f, err := os.Create(fileName)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	if err := w.writeData(f); err != nil {
		return err
	}

	return nil
}

func (g *CogWriter) Close() error {
	for i := range g.tiles {
		g.tiles[i].Close()
	}
	return nil
}

func (g *CogWriter) writeData(out io.Writer) error {
	sort.Sort(layerSorted(g.tiles))

	err := g.computeImageryOffsets()
	if err != nil {
		return err
	}

	strileData := &tagData{Offset: 16}
	if !g.bigtiff {
		strileData.Offset = 8
	}

	strileData.Offset += uint64(len(ghost))

	for _, t := range g.tiles {
		strileData.Offset += t.ifd.tagsSize
	}

	glen := uint64(len(ghost))
	g.writeHeader(out)

	off := uint64(16 + glen)
	if !g.bigtiff {
		off = 8 + glen
	}
	for i, t := range g.tiles {
		next := i < (len(g.tiles) - 1)
		err := g.writeIFD(out, t.ifd, off, strileData, next)
		if err != nil {
			return fmt.Errorf("write ifd: %w", err)
		}
		off += t.ifd.tagsSize
	}

	_, err = out.Write(strileData.Bytes())
	if err != nil {
		return fmt.Errorf("write strile pointers: %w", err)
	}

	datas := g.tiles
	tiles := getTiles(datas)

	var data []byte
	for tile := range tiles {
		idx := (tile.x + tile.y*uint64(tile.layer.col))
		bc := tile.layer.ifd.TileByteCounts[idx]
		if bc > 0 {
			_, err := tile.layer.GetReader().Seek(int64(tile.layer.ifd.OriginalTileOffsets[idx]), io.SeekStart)
			if err != nil {
				return err
			}
			if uint32(len(data)) < bc+8 {
				data = make([]byte, (bc+8)*2)
			}
			_, err = tile.layer.GetReader().Read(data[0 : 8+bc])
			if err != nil {
				return err
			}
			_, err = out.Write(data[0 : bc+8])
			if err != nil {
				return fmt.Errorf("write %d: %w", bc, err)
			}
		}
	}

	return err
}

func (g *CogWriter) computeStructure() {
	for _, t := range g.tiles {
		ifd := t.ifd
		ifd.ntags, ifd.tagsSize, ifd.strileSize, ifd.nplanes = ifd.structure(g.bigtiff)
	}
}

func (g *CogWriter) computeImageryOffsets() error {
	for _, t := range g.tiles {
		err := t.encode(g.enc, true)
		if err != nil {
			return err
		}
		ifd := t.ifd
		if g.bigtiff {
			ifd.NewTileOffsets64 = make([]uint64, len(ifd.OriginalTileOffsets))
			ifd.NewTileOffsets32 = nil
		} else {
			ifd.NewTileOffsets32 = make([]uint32, len(ifd.OriginalTileOffsets))
			ifd.NewTileOffsets64 = nil
		}
	}
	g.computeStructure()

	dataOffset := uint64(16)
	if !g.bigtiff {
		dataOffset = 8
	}

	dataOffset += uint64(len(ghost)) + 4

	for _, t := range g.tiles {
		dataOffset += t.ifd.strileSize + t.ifd.tagsSize
	}

	datas := g.tiles
	tiles := getTiles(datas)
	for tile := range tiles {
		tileidx := (tile.x + tile.y*uint64(tile.layer.col))
		cnt := uint64(tile.layer.ifd.TileByteCounts[tileidx])
		if cnt > 0 {
			if g.bigtiff {
				tile.layer.ifd.NewTileOffsets64[tileidx] = dataOffset
			} else {
				if dataOffset > uint64(^uint32(0)) {
					for range tiles {
						//skip
					}
					g.bigtiff = true
					return g.computeImageryOffsets()
				}
				tile.layer.ifd.NewTileOffsets32[tileidx] = uint32(dataOffset)
			}
			dataOffset += uint64(tile.layer.ifd.TileByteCounts[tileidx]) + 8
		} else {
			if g.bigtiff {
				tile.layer.ifd.NewTileOffsets64[tileidx] = 0
			} else {
				tile.layer.ifd.NewTileOffsets32[tileidx] = 0
			}
		}
	}

	return nil
}

type tiledTiff struct {
	tile  *Tile
	x, y  uint64
	layer *TileLayer
}

func getTiles(d []*TileLayer) chan tiledTiff {
	ch := make(chan tiledTiff)
	go func() {
		defer close(ch)
		for _, l := range d {
			for _, tile := range l.tiles {
				ch <- tiledTiff{
					tile:  tile,
					x:     uint64(tile.block[0]),
					y:     uint64(tile.block[1]),
					layer: l,
				}
			}
		}
	}()
	return ch
}
