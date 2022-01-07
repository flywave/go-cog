package cog

import (
	"encoding/binary"
	"errors"
	"io"
	"io/ioutil"
	"math"
	"os"
	"sort"

	"github.com/flywave/go-geo"

	vec2d "github.com/flywave/go3d/float64/vec2"
)

type Tile struct {
	block [2]int
	Id    [3]int
	Src   TileSource
}

type tiledSortedByXY []*Tile

func (a tiledSortedByXY) Len() int      { return len(a) }
func (a tiledSortedByXY) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a tiledSortedByXY) Less(i, j int) bool {
	if a[i].Id[0] == a[j].Id[0] {
		return a[i].Id[1] < a[j].Id[1]
	}
	return a[i].Id[0] < a[j].Id[0]
}

type tiledSortedByXYup []*Tile

func (a tiledSortedByXYup) Len() int      { return len(a) }
func (a tiledSortedByXYup) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a tiledSortedByXYup) Less(i, j int) bool {
	if a[i].Id[0] == a[j].Id[0] {
		return a[i].Id[1] > a[j].Id[1]
	}
	return a[i].Id[0] < a[j].Id[0]
}

type TileLayer struct {
	row      int
	col      int
	level    int
	tiles    []*Tile
	tilemap  map[[3]int]*Tile
	box      vec2d.Rect
	grid     geo.TileGrid
	ifd      *IFD
	tempFile *os.File
	noData   *string
}

func NewTileLayer(box vec2d.Rect, level int, grid geo.TileGrid) *TileLayer {
	rect, si, iter, err := grid.GetAffectedLevelTiles(box, level)

	if err != nil {
		return nil
	}

	tiles := []*Tile{}
	tilemap := make(map[[3]int]*Tile)

	x, y, zoom, done := iter.Next()
	for !done {
		t := &Tile{Id: [3]int{x, y, zoom}}
		tiles = append(tiles, t)
		tilemap[[3]int{x, y, zoom}] = t
		x, y, zoom, done = iter.Next()
	}
	t := &Tile{Id: [3]int{x, y, zoom}}
	tiles = append(tiles, t)
	tilemap[[3]int{x, y, zoom}] = t

	if grid.FlippedYAxis {
		sort.Sort(tiledSortedByXY(tiles))
	} else {
		sort.Sort(tiledSortedByXYup(tiles))
	}

	for i := range tiles {
		tiles[i].block = [2]int{int(i % si[0]), int(i / si[0])}
	}

	p, err := ioutil.TempFile(os.TempDir(), "tile-")

	if err != nil {
		return nil
	}

	return &TileLayer{row: si[1], col: si[0], level: level, box: rect, grid: grid, tilemap: tilemap, tiles: tiles, tempFile: p}
}

func (l *TileLayer) GetTile(t [3]int) *Tile {
	if t[2] != l.level {
		return nil
	}
	if t, ok := l.tilemap[t]; ok {
		return t
	}
	return nil
}

func (l *TileLayer) GetReader() io.ReadSeeker {
	return l.tempFile
}

func (l *TileLayer) SetSource(t [3]int, src TileSource) error {
	if t[2] != l.level {
		return errors.New("tile not found")
	}
	if t, ok := l.tilemap[t]; ok {
		t.Src = src
		return nil
	}
	return errors.New("tile not found")
}

func (l *TileLayer) GetTileSize() [2]uint32 {
	return [2]uint32{l.grid.TileSize[0], l.grid.TileSize[1]}
}

func (l *TileLayer) GetImageSize() [2]uint64 {
	width := l.box.Max[0] - l.box.Min[0]
	height := l.box.Max[1] - l.box.Min[1]

	res := l.grid.Resolution(l.level)

	return [2]uint64{uint64(math.Ceil(width / res)), uint64(math.Ceil(height / res))}
}

func (l *TileLayer) GetTransform() GeoTransform {
	res := l.grid.Resolution(l.level)
	box := l.grid.Srs.TransformRectTo(epsg4326, l.box, 16)
	return GeoTransform{box.Min[0], res, 0, box.Max[1], 0, -res}
}

func (l *TileLayer) setupIFD() {
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

func (l *TileLayer) Close() error {
	l.tempFile.Close()
	os.Remove(l.tempFile.Name())
	return nil
}

func (l *TileLayer) encode(enc binary.ByteOrder, clearOnSave bool) error {
	offset := uint64(0)

	for i := range l.tiles {
		var imageLen uint32
		if l.ifd == nil {
			l.ifd = &IFD{
				OriginalTileOffsets: make([]uint64, len(l.tiles)),
				TileByteCounts:      make([]uint32, len(l.tiles)),
			}
			n, _, err := l.tiles[i].Src.Encode(l.tempFile, l.ifd)
			if err != nil {
				return err
			}
			imageLen = n
		} else {
			n, _, err := l.tiles[i].Src.Encode(l.tempFile, nil)
			if err != nil {
				return err
			}
			imageLen = n
		}

		l.ifd.TileByteCounts[i] = imageLen
		l.ifd.OriginalTileOffsets[i] = offset
		offset += uint64(imageLen + 8)
	}

	l.tempFile.Sync()

	l.setupIFD()

	if clearOnSave {
		for i := range l.tiles {
			l.tiles[i].Src.Reset()
		}
	}

	return nil
}

type layerSorted []*TileLayer

func (a layerSorted) Len() int      { return len(a) }
func (a layerSorted) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a layerSorted) Less(i, j int) bool {
	return a[i].level > a[j].level
}

func BuildTileLayers(box vec2d.Rect, levels []int, grid geo.TileGrid) []*TileLayer {
	layers := make([]*TileLayer, len(levels))
	for i, level := range levels {
		layers[i] = NewTileLayer(box, level, grid)
	}
	sort.Sort(layerSorted(layers))
	return layers
}