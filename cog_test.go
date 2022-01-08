package cog

import (
	"fmt"
	"image"
	"testing"

	"github.com/flywave/go-geo"
	vec2d "github.com/flywave/go3d/float64/vec2"
)

func TestCases(t *testing.T) {
	tiles := [][3]int{
		{13733, 6366, 14},
		{13733, 6367, 14},
		{13734, 6366, 14},
		{13734, 6367, 14},
	}
	tiffs := make(map[[3]int]TileSource)
	bbox := vec2d.Rect{Min: vec2d.MaxVal, Max: vec2d.MinVal}

	srs900913 := geo.NewProj(900913)

	conf := geo.DefaultTileGridOptions()
	conf[geo.TILEGRID_SRS] = srs900913
	conf[geo.TILEGRID_RES_FACTOR] = 2.0
	conf[geo.TILEGRID_TILE_SIZE] = []uint32{512, 512}
	conf[geo.TILEGRID_ORIGIN] = geo.ORIGIN_UL

	grid := geo.NewTileGrid(conf)

	rect := image.Rect(0, 0, 512, 512)

	for i := range tiles {
		gtiffname := fmt.Sprintf("./test_data/%d_%d_%d.tif", tiles[i][2], tiles[i][0], tiles[i][1])
		gtiff := Read(gtiffname)

		if gtiff == nil {
			t.FailNow()
		}

		src := NewSource(gtiff.Data[0], &rect, CTLZW)

		tiffs[[3]int{tiles[i][0], tiles[i][1], tiles[i][2]}] = src

		bb := grid.TileBBox([3]int{tiles[i][0], tiles[i][1], tiles[i][2]}, false)

		bbox.Join(&bb)
	}

	layer := NewTileLayer(bbox, 14, grid)

	for id, k := range tiffs {
		layer.SetSource(id, k)
	}

	err := Write("./test_data/cog.tif", []*TileLayer{layer}, false)

	if err != nil {
		t.FailNow()
	}

	gtiff := Read("./test_data/cog.tif")

	if gtiff == nil {
		t.FailNow()
	}

}
