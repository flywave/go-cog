package cog

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"github.com/flywave/go-geo"
)

var epsg4326 geo.Proj
var tiffByteOrder binary.ByteOrder

func init() {
	epsg4326 = geo.NewProj(4326)
	tiffByteOrder = binary.LittleEndian
}

type Writer struct {
	bigtiff bool
	enc     binary.ByteOrder
}

func arrayFieldSize(data interface{}, bigtiff bool) uint64 {
	if bigtiff {
		switch d := data.(type) {
		case []byte:
			if len(d) <= 8 {
				return 20
			}
			return uint64(20 + len(d))
		case []uint16:
			if len(d) <= 4 {
				return 20
			}
			return uint64(20 + 2*len(d))
		case []uint32:
			if len(d) <= 2 {
				return 20
			}
			return uint64(20 + 4*len(d))
		case []uint64:
			if len(d) <= 1 {
				return 20
			}
			return uint64(20 + 8*len(d))
		case []int8:
			if len(d) <= 8 {
				return 20
			}
			return uint64(20 + len(d))
		case []int16:
			if len(d) <= 4 {
				return 20
			}
			return uint64(20 + len(d)*2)
		case []int32:
			if len(d) <= 2 {
				return 20
			}
			return uint64(20 + len(d)*4)
		case []int64:
			if len(d) <= 1 {
				return 20
			}
			return uint64(20 + len(d)*8)
		case []float32:
			if len(d) <= 2 {
				return 20
			}
			return uint64(20 + len(d)*4)
		case []float64:
			if len(d) <= 1 {
				return 20
			}
			return uint64(20 + len(d)*8)
		case string:
			if len(d) <= 7 {
				return 20
			}
			return uint64(20 + len(d) + 1)
		default:
			panic("wrong type")
		}
	} else {
		switch d := data.(type) {
		case []byte:
			if len(d) <= 4 {
				return 12
			}
			return uint64(12 + len(d))
		case []uint16:
			if len(d) <= 2 {
				return 12
			}
			return uint64(12 + 2*len(d))
		case []uint32:
			if len(d) <= 1 {
				return 12
			}
			return uint64(12 + 4*len(d))
		case []int8:
			if len(d) <= 4 {
				return 12
			}
			return uint64(12 + len(d))
		case []int16:
			if len(d) <= 2 {
				return 12
			}
			return uint64(12 + len(d)*2)
		case []int32:
			if len(d) <= 1 {
				return 12
			}
			return uint64(12 + len(d)*4)
		case []float32:
			if len(d) <= 1 {
				return 12
			}
			return uint64(12 + len(d)*4)
		case string:
			if len(d) <= 3 {
				return 12
			}
			return uint64(12 + len(d) + 1)
		case []float64:
			return uint64(12 + len(d)*8)
		case []int64:
			return uint64(12 + len(d)*8)
		case []uint64:
			return uint64(12 + len(d)*8)
		default:
			panic("wrong type")
		}
	}
}

func (g *Writer) writeArray(w io.Writer, tag uint16, data interface{}, tags *tagData) error {
	var buf []byte
	if g.bigtiff {
		buf = make([]byte, 20)
	} else {
		buf = make([]byte, 12)
	}
	g.enc.PutUint16(buf[0:2], tag)
	switch d := data.(type) {
	case []byte:
		n := len(d)
		g.enc.PutUint16(buf[2:4], tByte)
		if g.bigtiff {
			g.enc.PutUint64(buf[4:12], uint64(n))
			if n <= 8 {
				for i := 0; i < n; i++ {
					buf[12+i] = d[i]
				}
			} else {
				g.enc.PutUint64(buf[12:], tags.NextOffset())
				tags.Write(d)
			}
		} else {
			g.enc.PutUint32(buf[4:8], uint32(n))
			if n <= 4 {
				for i := 0; i < n; i++ {
					buf[8+i] = d[i]
				}
			} else {
				g.enc.PutUint32(buf[8:], uint32(tags.NextOffset()))
				tags.Write(d)
			}
		}
	case []uint16:
		n := len(d)
		g.enc.PutUint16(buf[2:4], tShort)
		if g.bigtiff {
			g.enc.PutUint64(buf[4:12], uint64(n))
			if n <= 4 {
				for i := 0; i < n; i++ {
					g.enc.PutUint16(buf[12+i*2:], d[i])
				}
			} else {
				g.enc.PutUint64(buf[12:], tags.NextOffset())
				for i := 0; i < n; i++ {
					binary.Write(tags, g.enc, d[i])
				}
			}
		} else {
			g.enc.PutUint32(buf[4:8], uint32(n))
			if n <= 2 {
				for i := 0; i < n; i++ {
					g.enc.PutUint16(buf[8+i*2:], d[i])
				}
			} else {
				g.enc.PutUint32(buf[8:], uint32(tags.NextOffset()))
				for i := 0; i < n; i++ {
					binary.Write(tags, g.enc, d[i])
				}
			}
		}
	case []uint32:
		n := len(d)
		g.enc.PutUint16(buf[2:4], tLong)
		if g.bigtiff {
			g.enc.PutUint64(buf[4:12], uint64(n))
			if n <= 2 {
				for i := 0; i < n; i++ {
					g.enc.PutUint32(buf[12+i*4:], d[i])
				}
			} else {
				g.enc.PutUint64(buf[12:], tags.NextOffset())
				for i := 0; i < n; i++ {
					binary.Write(tags, g.enc, d[i])
				}
			}
		} else {
			g.enc.PutUint32(buf[4:8], uint32(n))
			if n <= 1 {
				for i := 0; i < n; i++ {
					g.enc.PutUint32(buf[8:], d[i])
				}
			} else {
				g.enc.PutUint32(buf[8:], uint32(tags.NextOffset()))
				for i := 0; i < n; i++ {
					binary.Write(tags, g.enc, d[i])
				}
			}
		}
	case []uint64:
		n := len(d)
		g.enc.PutUint16(buf[2:4], tLong8)
		if g.bigtiff {
			g.enc.PutUint64(buf[4:12], uint64(n))
			if n <= 1 {
				g.enc.PutUint64(buf[12:], d[0])
			} else {
				g.enc.PutUint64(buf[12:], tags.NextOffset())
				for i := 0; i < n; i++ {
					binary.Write(tags, g.enc, d[i])
				}
			}
		} else {
			g.enc.PutUint32(buf[4:8], uint32(n))
			g.enc.PutUint32(buf[8:], uint32(tags.NextOffset()))
			for i := 0; i < n; i++ {
				binary.Write(tags, g.enc, d[i])
			}
		}
	case []float32:
		n := len(d)
		g.enc.PutUint16(buf[2:4], tFloat)
		if g.bigtiff {
			g.enc.PutUint64(buf[4:12], uint64(n))
			if n <= 2 {
				for i := 0; i < n; i++ {
					g.enc.PutUint32(buf[12+i*4:], math.Float32bits(d[i]))
				}
			} else {
				g.enc.PutUint64(buf[12:], tags.NextOffset())
				for i := 0; i < n; i++ {
					binary.Write(tags, g.enc, math.Float32bits(d[i]))
				}
			}
		} else {
			g.enc.PutUint32(buf[4:8], uint32(n))
			if n <= 1 {
				for i := 0; i < n; i++ {
					g.enc.PutUint32(buf[8:], math.Float32bits(d[i]))
				}
			} else {
				g.enc.PutUint32(buf[8:], uint32(tags.NextOffset()))
				for i := 0; i < n; i++ {
					binary.Write(tags, g.enc, math.Float32bits(d[i]))
				}
			}
		}
	case []float64:
		n := len(d)
		g.enc.PutUint16(buf[2:4], tDouble)
		if g.bigtiff {
			g.enc.PutUint64(buf[4:12], uint64(n))
			if n == 1 {
				for i := 0; i < n; i++ {
					g.enc.PutUint64(buf[12+i*4:], math.Float64bits(d[0]))
				}
			} else {
				g.enc.PutUint64(buf[12:], tags.NextOffset())
				for i := 0; i < n; i++ {
					binary.Write(tags, g.enc, math.Float64bits(d[i]))
				}
			}
		} else {
			g.enc.PutUint32(buf[4:8], uint32(n))
			g.enc.PutUint32(buf[8:], uint32(tags.NextOffset()))
			for i := 0; i < n; i++ {
				binary.Write(tags, g.enc, math.Float64bits(d[i]))
			}
		}
	case string:
		n := len(d) + 1
		g.enc.PutUint16(buf[2:4], tAscii)
		if g.bigtiff {
			g.enc.PutUint64(buf[4:12], uint64(n))
			if n <= 8 {
				for i := 0; i < n-1; i++ {
					buf[12+i] = byte(d[i])
				}
				buf[12+n-1] = 0
			} else {
				g.enc.PutUint64(buf[12:], tags.NextOffset())
				tags.Write(append([]byte(d), 0))
			}
		} else {
			g.enc.PutUint32(buf[4:8], uint32(n))
			if n <= 4 {
				for i := 0; i < n-1; i++ {
					buf[8+i] = d[i]
				}
				buf[8+n-1] = 0
			} else {
				g.enc.PutUint32(buf[8:], uint32(tags.NextOffset()))
				tags.Write(append([]byte(d), 0))
			}
		}
	default:
		return fmt.Errorf("unsupported type %v", d)
	}
	var err error
	if g.bigtiff {
		_, err = w.Write(buf[0:20])
	} else {
		_, err = w.Write(buf[0:12])
	}
	return err
}

func (g *Writer) writeField(w io.Writer, tag uint16, data interface{}) error {
	if g.bigtiff {
		var buf [20]byte
		switch d := data.(type) {
		case byte:
			g.enc.PutUint16(buf[0:2], tag)
			g.enc.PutUint16(buf[2:4], tByte)
			g.enc.PutUint64(buf[4:12], 1)
			buf[12] = d
		case uint16:
			g.enc.PutUint16(buf[0:2], tag)
			g.enc.PutUint16(buf[2:4], tShort)
			g.enc.PutUint64(buf[4:12], 1)
			g.enc.PutUint16(buf[12:], d)
		case uint32:
			g.enc.PutUint16(buf[0:2], tag)
			g.enc.PutUint16(buf[2:4], tLong)
			g.enc.PutUint64(buf[4:12], 1)
			g.enc.PutUint32(buf[12:], d)
		case uint64:
			g.enc.PutUint16(buf[0:2], tag)
			g.enc.PutUint16(buf[2:4], tLong8)
			g.enc.PutUint64(buf[4:12], 1)
			g.enc.PutUint64(buf[12:], d)
		case float32:
			g.enc.PutUint16(buf[0:2], tag)
			g.enc.PutUint16(buf[2:4], tFloat)
			g.enc.PutUint64(buf[4:12], 1)
			g.enc.PutUint32(buf[12:], math.Float32bits(d))
		case float64:
			g.enc.PutUint16(buf[0:2], tag)
			g.enc.PutUint16(buf[2:4], tDouble)
			g.enc.PutUint64(buf[4:12], 1)
			g.enc.PutUint64(buf[12:], math.Float64bits(d))
		case int8:
			g.enc.PutUint16(buf[0:2], tag)
			g.enc.PutUint16(buf[2:4], tSByte)
			g.enc.PutUint64(buf[4:12], 1)
			buf[12] = byte(d)
		case int16:
			g.enc.PutUint16(buf[0:2], tag)
			g.enc.PutUint16(buf[2:4], tSShort)
			g.enc.PutUint64(buf[4:12], 1)
			g.enc.PutUint16(buf[12:], uint16(d))
		case int32:
			g.enc.PutUint16(buf[0:2], tag)
			g.enc.PutUint16(buf[2:4], tSLong)
			g.enc.PutUint64(buf[4:12], 1)
			g.enc.PutUint32(buf[12:], uint32(d))
		case int64:
			g.enc.PutUint16(buf[0:2], tag)
			g.enc.PutUint16(buf[2:4], tSLong8)
			g.enc.PutUint64(buf[4:12], 1)
			g.enc.PutUint64(buf[12:], uint64(d))
		default:
			panic("unsupported type")
		}
		_, err := w.Write(buf[0:20])
		return err
	} else {
		var buf [12]byte
		switch d := data.(type) {
		case byte:
			g.enc.PutUint16(buf[0:2], tag)
			g.enc.PutUint16(buf[2:4], tByte)
			g.enc.PutUint32(buf[4:8], 1)
			buf[8] = d
		case uint16:
			g.enc.PutUint16(buf[0:2], tag)
			g.enc.PutUint16(buf[2:4], tShort)
			g.enc.PutUint32(buf[4:8], 1)
			g.enc.PutUint16(buf[8:], d)
		case uint32:
			g.enc.PutUint16(buf[0:2], tag)
			g.enc.PutUint16(buf[2:4], tLong)
			g.enc.PutUint32(buf[4:8], 1)
			g.enc.PutUint32(buf[8:], d)
		case float32:
			g.enc.PutUint16(buf[0:2], tag)
			g.enc.PutUint16(buf[2:4], tFloat)
			g.enc.PutUint32(buf[4:8], 1)
			g.enc.PutUint32(buf[8:], math.Float32bits(d))
		case int8:
			g.enc.PutUint16(buf[0:2], tag)
			g.enc.PutUint16(buf[2:4], tSByte)
			g.enc.PutUint32(buf[4:8], 1)
			buf[8] = byte(d)
		case int16:
			g.enc.PutUint16(buf[0:2], tag)
			g.enc.PutUint16(buf[2:4], tSShort)
			g.enc.PutUint32(buf[4:8], 1)
			g.enc.PutUint16(buf[8:], uint16(d))
		case int32:
			g.enc.PutUint16(buf[0:2], tag)
			g.enc.PutUint16(buf[2:4], tSLong)
			g.enc.PutUint32(buf[4:8], 1)
			g.enc.PutUint32(buf[8:], uint32(d))
		default:
			panic("unsupported type")
		}
		_, err := w.Write(buf[0:12])
		return err
	}
}

const ghost = `GDAL_STRUCTURAL_METADATA_SIZE=000140 bytes
LAYOUT=IFDS_BEFORE_DATA
BLOCK_ORDER=ROW_MAJOR
BLOCK_LEADER=SIZE_AS_UINT4
BLOCK_TRAILER=LAST_4_BYTES_REPEATED
KNOWN_INCOMPATIBLE_EDITION=NO
  `

func (g *Writer) writeHeader(w io.Writer) error {
	glen := uint64(len(ghost))
	var err error
	if g.bigtiff {
		buf := [16]byte{}
		if g.enc == binary.LittleEndian {
			copy(buf[0:], []byte("II"))
		} else {
			copy(buf[0:], []byte("MM"))
		}
		g.enc.PutUint16(buf[2:], 43)
		g.enc.PutUint16(buf[4:], 8)
		g.enc.PutUint16(buf[6:], 0)
		g.enc.PutUint64(buf[8:], 16+glen)
		_, err = w.Write(buf[:])
	} else {
		buf := [8]byte{}
		if g.enc == binary.LittleEndian {
			copy(buf[0:], []byte("II"))
		} else {
			copy(buf[0:], []byte("MM"))
		}
		g.enc.PutUint16(buf[2:], 42)
		g.enc.PutUint32(buf[4:], 8+uint32(glen))
		_, err = w.Write(buf[:])
	}
	if err != nil {
		return err
	}

	_, err = w.Write([]byte(ghost))
	return err
}

func (g *Writer) writeIFD(w io.Writer, ifd *IFD, offset uint64, striledata *tagData, next bool) error {
	nextOff := uint64(0)
	if next {
		nextOff = offset + ifd.tagsSize
	}
	var err error

	overflow := &tagData{
		Offset: offset + 8 + 20*ifd.ntags + 8,
	}
	if !g.bigtiff {
		overflow.Offset = offset + 2 + 12*ifd.ntags + 4
	}

	if g.bigtiff {
		err = binary.Write(w, g.enc, ifd.ntags)
	} else {
		err = binary.Write(w, g.enc, uint16(ifd.ntags))
	}
	if err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	if ifd.NewSubfileType > 0 {
		err := g.writeField(w, TagNewSubfileType, ifd.NewSubfileType)
		if err != nil {
			panic(err)
		}
	}
	if ifd.ImageWidth > 0 {
		err := g.writeField(w, TagImageWidth, uint32(ifd.ImageWidth))
		if err != nil {
			panic(err)
		}
	}
	if ifd.ImageLength > 0 {
		err := g.writeField(w, TagImageLength, uint32(ifd.ImageLength))
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.BitsPerSample) > 0 {
		err := g.writeArray(w, TagBitsPerSample, ifd.BitsPerSample, overflow)
		if err != nil {
			panic(err)
		}
	}

	if ifd.Compression > 0 {
		err := g.writeField(w, TagCompression, ifd.Compression)
		if err != nil {
			panic(err)
		}
	}

	err = g.writeField(w, TagPhotometricInterpretation, ifd.PhotometricInterpretation)
	if err != nil {
		panic(err)
	}

	if len(ifd.DocumentName) > 0 {
		err := g.writeArray(w, TagDocumentName, ifd.DocumentName, overflow)
		if err != nil {
			panic(err)
		}
	}

	if ifd.SamplesPerPixel > 0 {
		err := g.writeField(w, TagSamplesPerPixel, ifd.SamplesPerPixel)
		if err != nil {
			panic(err)
		}
	}

	if ifd.PlanarConfiguration > 0 {
		err := g.writeField(w, TagPlanarConfiguration, ifd.PlanarConfiguration)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.DateTime) > 0 {
		err := g.writeArray(w, TagDateTime, ifd.DateTime, overflow)
		if err != nil {
			panic(err)
		}
	}

	if ifd.Predictor > 0 {
		err := g.writeField(w, TagPredictor, ifd.Predictor)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.Colormap) > 0 {
		err := g.writeArray(w, TagColorMap, ifd.Colormap, overflow)
		if err != nil {
			panic(err)
		}
	}

	if ifd.TileWidth > 0 {
		err := g.writeField(w, TagTileWidth, ifd.TileWidth)
		if err != nil {
			panic(err)
		}
	}

	if ifd.TileLength > 0 {
		err := g.writeField(w, TagTileLength, ifd.TileLength)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.NewTileOffsets32) > 0 {
		err := g.writeArray(w, TagTileOffsets, ifd.NewTileOffsets32, striledata)
		if err != nil {
			panic(err)
		}
	} else {
		err := g.writeArray(w, TagTileOffsets, ifd.NewTileOffsets64, striledata)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.TileByteCounts) > 0 {
		err := g.writeArray(w, TagTileByteCounts, ifd.TileByteCounts, striledata)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.ExtraSamples) > 0 {
		err := g.writeArray(w, TagExtraSamples, ifd.ExtraSamples, overflow)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.SampleFormat) > 0 {
		err := g.writeArray(w, TagSampleFormat, ifd.SampleFormat, overflow)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.JPEGTables) > 0 {
		err := g.writeArray(w, TagJPEGTables, ifd.JPEGTables, overflow)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.ModelPixelScaleTag) > 0 {
		err := g.writeArray(w, TagModelPixelScaleTag, ifd.ModelPixelScaleTag, overflow)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.ModelTiePointTag) > 0 {
		err := g.writeArray(w, TagModelTiepointTag, ifd.ModelTiePointTag, overflow)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.ModelTransformationTag) > 0 {
		err := g.writeArray(w, TagModelTransformationTag, ifd.ModelTransformationTag, overflow)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.GeoKeyDirectoryTag) > 0 {
		err := g.writeArray(w, TagGeoKeyDirectoryTag, ifd.GeoKeyDirectoryTag, overflow)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.GeoDoubleParamsTag) > 0 {
		err := g.writeArray(w, TagGeoDoubleParamsTag, ifd.GeoDoubleParamsTag, overflow)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.GeoAsciiParamsTag) > 0 {
		err := g.writeArray(w, TagGeoAsciiParamsTag, ifd.GeoAsciiParamsTag, overflow)
		if err != nil {
			panic(err)
		}
	}

	if ifd.GDALMetaData != "" {
		err := g.writeArray(w, TagGDAL_METADATA, ifd.GDALMetaData, overflow)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.NoData) > 0 {
		err := g.writeArray(w, TagGDAL_NODATA, ifd.NoData, overflow)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.LERCParams) > 0 {
		err := g.writeArray(w, TagLERCParams, ifd.LERCParams, overflow)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.RPCs) > 0 {
		err := g.writeArray(w, TagRPCs, ifd.RPCs, overflow)
		if err != nil {
			panic(err)
		}
	}

	if g.bigtiff {
		err = binary.Write(w, g.enc, nextOff)
	} else {
		err = binary.Write(w, g.enc, uint32(nextOff))
	}
	if err != nil {
		return fmt.Errorf("write next: %w", err)
	}
	_, err = w.Write(overflow.Bytes())
	if err != nil {
		return fmt.Errorf("write parea: %w", err)
	}
	return nil
}
