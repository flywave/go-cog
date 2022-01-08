package cog

import (
	"testing"

	"github.com/flywave/go-geo"
)

func TestTiffWrite(t *testing.T) {
	gtiff := Read("./test_data/scan_512x512_rgb8_tiled.tif")

	if gtiff == nil {
		t.FailNow()
	}

	data := gtiff.Data[0]

	if data == nil {
		t.FailNow()
	}

	srs900913 := geo.NewProj(900913)

	conf := geo.DefaultTileGridOptions()
	conf[geo.TILEGRID_SRS] = srs900913
	conf[geo.TILEGRID_RES_FACTOR] = 2.0
	conf[geo.TILEGRID_TILE_SIZE] = []uint32{512, 512}
	conf[geo.TILEGRID_ORIGIN] = geo.ORIGIN_UL

	src := NewSource(data, nil, CTLZW)

	if src == nil {
		t.FailNow()
	}

	grid := geo.NewTileGrid(conf)

	bbox := grid.TileBBox([3]int{13733, 6366, 14}, false)

	err := WriteTile("./test_data/tiled.tif", src, bbox, srs900913, [2]uint32{512, 512}, true, nil)

	if err != nil {
		t.FailNow()
	}

	gtiff = Read("./test_data/tiled.tif")
	if gtiff == nil {
		t.FailNow()
	}

	geikeys, _ := gtiff.GetEPSGCode(0)

	bounds := gtiff.GetBounds(0)

	if geikeys == 0 || bounds.Max[0] == 0 {
		t.FailNow()
	}
}

func TestTiffRead(t *testing.T) {
	gtiff := Read("./test_data/test.tif")

	if gtiff == nil {
		t.FailNow()
	}
}
