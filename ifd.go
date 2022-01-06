package cog

import (
	"bytes"
	"errors"
	"strings"

	"github.com/google/tiff"
)

type IFD struct {
	SubfileType               uint32   `tiff:"field,tag=254"`
	ImageWidth                uint64   `tiff:"field,tag=256"`
	ImageLength               uint64   `tiff:"field,tag=257"`
	BitsPerSample             []uint16 `tiff:"field,tag=258"`
	Compression               uint16   `tiff:"field,tag=259"`
	PhotometricInterpretation uint16   `tiff:"field,tag=262"`
	DocumentName              string   `tiff:"field,tag=269"`
	StripOffsets              []uint32 `tiff:"field,tag=273"`
	SamplesPerPixel           uint16   `tiff:"field,tag=277"`
	StripByteCounts           []uint32 `tiff:"field,tag=279"`
	PlanarConfiguration       uint16   `tiff:"field,tag=284"`
	Software                  string   `tiff:"field,tag=305"`
	DateTime                  string   `tiff:"field,tag=306"`
	Predictor                 uint16   `tiff:"field,tag=317"`
	Colormap                  []uint16 `tiff:"field,tag=320"`
	TileWidth                 uint16   `tiff:"field,tag=322"`
	TileLength                uint16   `tiff:"field,tag=323"`
	OriginalTileOffsets       []uint64 `tiff:"field,tag=324"`
	NewTileOffsets64          []uint64
	NewTileOffsets32          []uint32
	TempTileByteCounts        []uint64 `tiff:"field,tag=325"`
	TileByteCounts            []uint32
	ExtraSamples              []uint16 `tiff:"field,tag=338"`
	SampleFormat              []uint16 `tiff:"field,tag=339"`
	JPEGTables                []byte   `tiff:"field,tag=347"`

	ModelPixelScaleTag     []float64 `tiff:"field,tag=33550"`
	ModelTiePointTag       []float64 `tiff:"field,tag=33922"`
	ModelTransformationTag []float64 `tiff:"field,tag=34264"`
	GeoKeyDirectoryTag     []uint16  `tiff:"field,tag=34735"`
	GeoDoubleParamsTag     []float64 `tiff:"field,tag=34736"`
	GeoAsciiParamsTag      string    `tiff:"field,tag=34737"`
	GDALMetaData           string    `tiff:"field,tag=42112"`
	NoData                 string    `tiff:"field,tag=42113"`
	LERCParams             []uint32  `tiff:"field,tag=50674"`
	RPCs                   []float64 `tiff:"field,tag=50844"`

	GTModelTypeGeoKey    uint16 `tiff:"field,tag=1024"`
	GTRasterTypeGeoKey   uint16 `tiff:"field,tag=1025"`
	GTCitationGeoKey     uint32 `tiff:"field,tag=1026"`
	GeographicTypeGeoKey uint16 `tiff:"field,tag=2048"`

	ntags            uint64
	ntilesx, ntilesy uint64
	nplanes          uint64 //1 if PlanarConfiguration==1, SamplesPerPixel if PlanarConfiguration==2
	tagsSize         uint64
	strileSize       uint64
	r                tiff.BReader
}

func (ifd *IFD) SetEPSG(epsg uint, rasterPixelIsArea bool) {
	if rasterPixelIsArea {
		ifd.GTRasterTypeGeoKey = uint16(1)
	} else {
		ifd.GTRasterTypeGeoKey = uint16(2)
	}

	if v, ok := geographicTypeMap[epsg]; ok {
		ifd.GTModelTypeGeoKey = uint16(2)
		ifd.GeographicTypeGeoKey = uint16(epsg)
		v += "|"
		v = strings.Replace(v, "_", " ", -1)
		ifd.GTCitationGeoKey = uint32(len(v))
	} else if v, ok := projectedCSMap[epsg]; ok {
		ifd.GTModelTypeGeoKey = uint16(1)
		ifd.GeographicTypeGeoKey = uint16(epsg)
		v += "|"
		v = strings.Replace(v, "_", " ", -1)
		ifd.GTCitationGeoKey = uint32(len(v))
	} else {
		if epsg != 0 {
			panic(errors.New("Unrecognized EPSG code."))
		} else {
			v := "Unknown|"
			ifd.GTCitationGeoKey = uint32(len(v))
		}
	}
}

type GeoTransform [6]float64

func (gt GeoTransform) Origin() (float64, float64) {
	return gt[0], gt[3]
}

func (gt GeoTransform) Scale() (float64, float64) {
	return gt[1], -gt[5]
}

func (ifd *IFD) Geotransform() (GeoTransform, error) {
	gt := GeoTransform{0, 1, 0, 0, 0, 1}
	if len(ifd.ModelPixelScaleTag) >= 2 &&
		ifd.ModelPixelScaleTag[0] != 0 && ifd.ModelPixelScaleTag[1] != 0 {
		gt[1] = ifd.ModelPixelScaleTag[0]
		gt[5] = -ifd.ModelPixelScaleTag[1]
		if gt[5] > 0 {
			return gt, errors.New("negativ y-scale not supported")
		}

		if len(ifd.ModelTiePointTag) >= 6 {
			gt[0] =
				ifd.ModelTiePointTag[3] -
					ifd.ModelTiePointTag[0]*gt[1]
			gt[3] =
				ifd.ModelTiePointTag[4] -
					ifd.ModelTiePointTag[1]*gt[5]
		}
	} else if len(ifd.ModelTransformationTag) == 16 {
		gt[0] = ifd.ModelTransformationTag[3]
		gt[1] = ifd.ModelTransformationTag[0]
		gt[2] = ifd.ModelTransformationTag[1]
		gt[3] = ifd.ModelTransformationTag[7]
		gt[4] = ifd.ModelTransformationTag[4]
		gt[5] = ifd.ModelTransformationTag[5]
	} else {
		return gt, errors.New("no geotiff referencing computed")
	}
	return gt, nil
}

func (ifd *IFD) structure(bigtiff bool) (tagCount, ifdSize, strileSize, planeCount uint64) {
	cnt := uint64(0)
	size := uint64(16) //8 for field count + 8 for next ifd offset
	tagSize := uint64(20)
	planeCount = 1
	if !bigtiff {
		size = 6 // 2 for field count + 4 for next ifd offset
		tagSize = 12
	}
	strileSize = uint64(0)

	if ifd.SubfileType > 0 {
		cnt++
		size += tagSize
	}
	if ifd.ImageWidth > 0 {
		cnt++
		size += tagSize
	}
	if ifd.ImageLength > 0 {
		cnt++
		size += tagSize
	}
	if len(ifd.BitsPerSample) > 0 {
		cnt++
		size += arrayFieldSize(ifd.BitsPerSample, bigtiff)
	}
	if ifd.Compression > 0 {
		cnt++
		size += tagSize
	}

	cnt++ /*PhotometricInterpretation*/
	size += tagSize

	if len(ifd.DocumentName) > 0 {
		cnt++
		size += arrayFieldSize(ifd.DocumentName, bigtiff)
	}
	if ifd.SamplesPerPixel > 0 {
		cnt++
		size += tagSize
	}
	if ifd.PlanarConfiguration > 0 {
		cnt++
		size += tagSize
	}
	if ifd.PlanarConfiguration == 2 {
		planeCount = uint64(ifd.SamplesPerPixel)
	}
	if len(ifd.DateTime) > 0 {
		cnt++
		size += arrayFieldSize(ifd.DateTime, bigtiff)
	}
	if ifd.Predictor > 0 {
		cnt++
		size += tagSize
	}
	if len(ifd.Colormap) > 0 {
		cnt++
		size += arrayFieldSize(ifd.Colormap, bigtiff)
	}
	if ifd.TileWidth > 0 {
		cnt++
		size += tagSize
	}
	if ifd.TileLength > 0 {
		cnt++
		size += tagSize
	}
	if len(ifd.NewTileOffsets32) > 0 {
		cnt++
		size += tagSize
		strileSize += arrayFieldSize(ifd.NewTileOffsets32, bigtiff) - tagSize
	} else if len(ifd.NewTileOffsets64) > 0 {
		cnt++
		size += tagSize
		strileSize += arrayFieldSize(ifd.NewTileOffsets64, bigtiff) - tagSize
	}
	if len(ifd.TileByteCounts) > 0 {
		cnt++
		size += tagSize
		strileSize += arrayFieldSize(ifd.TileByteCounts, bigtiff) - tagSize
	}
	if len(ifd.ExtraSamples) > 0 {
		cnt++
		size += arrayFieldSize(ifd.ExtraSamples, bigtiff)
	}
	if len(ifd.SampleFormat) > 0 {
		cnt++
		size += arrayFieldSize(ifd.SampleFormat, bigtiff)
	}
	if len(ifd.JPEGTables) > 0 {
		cnt++
		size += arrayFieldSize(ifd.JPEGTables, bigtiff)
	}
	if len(ifd.ModelPixelScaleTag) > 0 {
		cnt++
		size += arrayFieldSize(ifd.ModelPixelScaleTag, bigtiff)
	}
	if len(ifd.ModelTiePointTag) > 0 {
		cnt++
		size += arrayFieldSize(ifd.ModelTiePointTag, bigtiff)
	}
	if len(ifd.ModelTransformationTag) > 0 {
		cnt++
		size += arrayFieldSize(ifd.ModelTransformationTag, bigtiff)
	}
	if len(ifd.GeoKeyDirectoryTag) > 0 {
		cnt++
		size += arrayFieldSize(ifd.GeoKeyDirectoryTag, bigtiff)
	}
	if len(ifd.GeoDoubleParamsTag) > 0 {
		cnt++
		size += arrayFieldSize(ifd.GeoDoubleParamsTag, bigtiff)
	}
	if ifd.GeoAsciiParamsTag != "" {
		cnt++
		size += arrayFieldSize(ifd.GeoAsciiParamsTag, bigtiff)
	}
	if ifd.GDALMetaData != "" {
		cnt++
		size += arrayFieldSize(ifd.GDALMetaData, bigtiff)
	}
	if ifd.NoData != "" {
		cnt++
		size += arrayFieldSize(ifd.NoData, bigtiff)
	}
	if len(ifd.LERCParams) > 0 {
		cnt++
		size += arrayFieldSize(ifd.LERCParams, bigtiff)
	}
	if len(ifd.RPCs) > 0 {
		cnt++
		size += arrayFieldSize(ifd.RPCs, bigtiff)
	}
	return cnt, size, strileSize, planeCount
}

type tagData struct {
	bytes.Buffer
	Offset uint64
}

func (t *tagData) NextOffset() uint64 {
	return t.Offset + uint64(t.Buffer.Len())
}
