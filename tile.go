package cog

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/flywave/go-geo"

	vec2d "github.com/flywave/go3d/float64/vec2"
)

type TileWriter struct {
	Writer
	src    TileSource
	grid   *geo.TileGrid
	box    vec2d.Rect
	ifd    *IFD
	level  int
	noData *string
}

func WriteTile(fileName string, src TileSource, box vec2d.Rect, level int, grid *geo.TileGrid, noData *string) error {
	w := NewTileWriter(src, tiffByteOrder, false, box, level, grid, noData)

	f, err := os.Create(fileName)

	if err != nil {
		return err
	}

	err = w.writeData(f)

	if err != nil {
		return err
	}

	defer f.Close()

	return nil
}

func NewTileWriter(src TileSource, enc binary.ByteOrder, bigtiff bool, box vec2d.Rect, level int, grid *geo.TileGrid, noData *string) *TileWriter {
	w := &TileWriter{src: src, grid: grid, box: box, ifd: &IFD{}, level: level, noData: noData, Writer: Writer{enc: enc, bigtiff: bigtiff}}
	return w
}

func (l *TileWriter) GetImageSize() [2]uint64 {
	width := l.box.Max[0] - l.box.Min[0]
	height := l.box.Max[1] - l.box.Min[1]

	res := l.grid.Resolution(l.level)

	return [2]uint64{uint64(math.Ceil(width / res)), uint64(math.Ceil(height / res))}
}

func (l *TileWriter) GetTransform() GeoTransform {
	res := l.grid.Resolution(l.level)
	box := l.grid.Srs.TransformRectTo(epsg4326, l.box, 16)
	return GeoTransform{box.Min[0], res, 0, box.Max[1], 0, -res}
}

func (l *TileWriter) setupIFD() {
	l.ifd.SetEPSG(uint(4326), true)
	si := l.GetImageSize()
	l.ifd.ImageWidth, l.ifd.ImageLength = uint64(si[0]), uint64(si[1])

	if l.ifd.TileWidth != uint16(l.grid.TileSize[0]) {
		l.ifd.TileWidth = uint16(l.grid.TileSize[0])
	}

	if l.ifd.TileLength != uint16(l.grid.TileSize[1]) {
		l.ifd.TileLength = uint16(l.grid.TileSize[1])
	}
	tran := l.GetTransform()
	l.ifd.ModelTiePointTag = tran[:]

	if l.noData != nil {
		l.ifd.NoData = *l.noData
	}
}

func (l *TileWriter) writeData(out io.Writer) error {
	buf := &bytes.Buffer{}

	l.setupIFD()

	ifd := l.ifd

	byteLen, ifd, err := l.src.Encode(buf, ifd)
	if err != nil {
		return err
	}
	ifd.NewTileOffsets32 = []uint32{0}
	ifd.TileByteCounts = []uint32{uint32(byteLen)}

	strileData := &tagData{Offset: 16}
	if !l.bigtiff {
		strileData.Offset = 8
	}

	strileData.Offset += uint64(len(ghost))

	strileData.Offset += l.ifd.tagsSize

	glen := uint64(len(ghost))
	l.writeHeader(out)

	off := uint64(16 + glen)
	if !l.bigtiff {
		off = 8 + glen
	}

	dataOffset := uint64(16)
	if !l.bigtiff {
		dataOffset = 8
	}

	dataOffset += uint64(len(ghost)) + 4

	ifd.ntags, ifd.tagsSize, ifd.strileSize, ifd.nplanes = ifd.structure(false)

	dataOffset += ifd.strileSize + ifd.tagsSize

	ifd.NewTileOffsets32 = []uint32{uint32(dataOffset)}

	err = l.writeIFD(out, l.ifd, off, strileData, false)
	if err != nil {
		return fmt.Errorf("write ifd: %w", err)
	}
	off += l.ifd.tagsSize

	_, err = out.Write(strileData.Bytes())
	if err != nil {
		return fmt.Errorf("write strile pointers: %w", err)
	}

	_, err = out.Write(buf.Bytes())
	if err != nil {
		return fmt.Errorf("write data: %w", err)
	}
	return nil
}
