package cog

import (
	"testing"
)

func TestCases(t *testing.T) {
	gtiff := Read("./test_data/scan_512x512_rgb8_tiled.tif")

	if gtiff == nil {
		t.FailNow()
	}
}
