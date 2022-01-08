package cog

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
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

func (l *TileWriter) GetTransform() GeoTransform {
	box := l.grid.Srs.TransformRectTo(epsg4326, l.box, 16)
	res := caclulatePixelSize(int(l.grid.TileSize[0]), int(l.grid.TileSize[1]), box)

	if l.grid.FlippedYAxis {
		return GeoTransform{box.Min[0], res[0], 0, box.Max[1], 0, -res[1]}
	}
	return GeoTransform{box.Min[0], res[0], 0, box.Max[1], 0, res[1]}
}

func (l *TileWriter) setupIFD() {
	l.ifd.SetEPSG(uint(4326), true)
	l.ifd.ImageWidth, l.ifd.ImageLength = uint64(l.grid.TileSize[0]), uint64(l.grid.TileSize[1])

	if l.ifd.TileWidth != uint16(l.grid.TileSize[0]) {
		l.ifd.TileWidth = uint16(l.grid.TileSize[0])
	}

	if l.ifd.TileLength != uint16(l.grid.TileSize[1]) {
		l.ifd.TileLength = uint16(l.grid.TileSize[1])
	}
	tran := l.GetTransform()

	var north, south, east, west float64
	if tran[5] < 0 {
		north = tran[3]
		south = tran[3] + tran[5]*float64(l.grid.TileSize[1])
	} else {
		south = tran[3]
		north = tran[3] + tran[5]*float64(l.grid.TileSize[1])
	}
	if tran[1] < 0 {
		east = tran[0]
		west = tran[0] + tran[1]*float64(l.grid.TileSize[0])
	} else {
		west = tran[0]
		east = tran[0] + tran[1]*float64(l.grid.TileSize[0])
	}

	cellSizeX := (east - west) / float64(l.grid.TileSize[0])
	cellSizeY := (north - south) / float64(l.grid.TileSize[0])

	l.ifd.ModelTiePointTag = []float64{0, 0, 0, west, north, 0}
	l.ifd.ModelPixelScaleTag = []float64{cellSizeX, cellSizeY, 0}

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
