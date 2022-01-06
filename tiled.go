package cog

import (
	"math"

	"github.com/flywave/go-geo"

	vec2d "github.com/flywave/go3d/float64/vec2"
)

type Tile struct {
	block [2]int
	Id    [3]int
}

type TileLayer struct {
	row     int
	col     int
	level   int
	tiles   []*Tile
	tilemap map[[3]int]*Tile
	box     vec2d.Rect
	grid    geo.TileGrid
	ifd     *IFD
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

	for i := range tiles {
		tiles[i].block = [2]int{int(i % si[0]), int(i / si[0])}
	}

	return &TileLayer{row: si[1], col: si[0], level: level, box: rect, grid: grid, tilemap: tilemap, tiles: tiles}
}

func (l *TileLayer) GetTile(x, y int) *Tile {
	id := [3]int{x, y, l.level}
	if t, ok := l.tilemap[id]; ok {
		return t
	}
	return nil
}

func (l *TileLayer) GetTileSize() [2]uint32 {
	return [2]uint32{l.grid.TileSize[0], l.grid.TileSize[1]}
}

func (l *TileLayer) GetImageSize() [2]uint32 {
	width := l.box.Max[0] - l.box.Min[0]
	height := l.box.Max[1] - l.box.Min[1]

	res := l.grid.Resolution(l.level)

	return [2]uint32{uint32(math.Floor(width / res)), uint32(math.Floor(height / res))}
}

func (l *TileLayer) GetTransform() GeoTransform {
	res := l.grid.Resolution(l.level)
	return GeoTransform{l.box.Min[0], res, 0, l.box.Max[1], 0, -res}
}

func (l *TileLayer) GetEPSG() uint32 {
	return uint32(geo.GetEpsgNum(l.grid.Srs.GetSrsCode()))
}

func (l *TileLayer) GetBounds() vec2d.Rect {
	return l.box
}
