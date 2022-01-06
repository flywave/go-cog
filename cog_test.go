package cog

import (
	"fmt"
	"os"
	"testing"

	vec2d "github.com/flywave/go3d/float64/vec2"

	"github.com/flywave/go-geo"
	"github.com/google/tiff"
)

func TestCases(t *testing.T) {
	gtiff := Read("./scan_512x512_rgb8_tiled.tif")

	if gtiff == nil {
		t.FailNow()
	}
}

func TestTile(t *testing.T) {
	tiles := [][3]int{
		{13733, 6366, 14},
		{13733, 6367, 14},
		{13734, 6366, 14},
		{13734, 6367, 14},
	}

	srs900913 := geo.NewProj(900913)

	conf := geo.DefaultTileGridOptions()
	conf[geo.TILEGRID_SRS] = srs900913
	conf[geo.TILEGRID_RES_FACTOR] = 2.0
	conf[geo.TILEGRID_TILE_SIZE] = []uint32{512, 512}
	conf[geo.TILEGRID_ORIGIN] = geo.ORIGIN_UL

	grid := geo.NewTileGrid(conf)

	r := vec2d.Rect{Min: vec2d.MaxVal, Max: vec2d.MinVal}

	for i := range tiles {
		bbox := grid.TileBBox([3]int{tiles[i][0], tiles[i][1], tiles[i][2]}, false)

		r.Join(&bbox)
	}

	layer := NewTileLayer(r, 14, *grid)

	for i := range tiles {
		f, err := os.Open(fmt.Sprintf("./%d_%d_%d.tif", tiles[i][2], tiles[i][0], tiles[i][1]))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()

		tif, err := tiff.Parse(f, nil, nil)
		if err != nil {
			t.Fatal(err)
		}

		//tie := layer.GetTile(tiles[i][0], tiles[i][1])

		ifd, err := loadIFD(tif.R(), tif.IFDs()[0])
		if err != nil {
			t.Fatal(err)
		}

		ifd.r = tif.R()
	}

	if layer.GetTile(13733, 6367) == nil {
		t.FailNow()
	}
	si := layer.GetImageSize()
	if si[0] != 512*2 {
		t.FailNow()
	}

	code := layer.GetEPSG()

	if code == 0 {
		t.FailNow()
	}

	tram := layer.GetTransform()

	if tram[0] == 0 {
		t.FailNow()
	}
}
