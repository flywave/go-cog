package cog

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"io"
	"math"
	"os"
	"testing"
)

func writeTiffHeader(w io.Writer, bigtiff bool, enc binary.ByteOrder) error {
	glen := uint64(len(ghost))
	var err error
	if bigtiff {
		buf := [16]byte{}
		if enc == binary.LittleEndian {
			copy(buf[0:], []byte("II"))
		} else {
			copy(buf[0:], []byte("MM"))
		}
		enc.PutUint16(buf[2:], 43)
		enc.PutUint16(buf[4:], 8)
		enc.PutUint16(buf[6:], 0)
		enc.PutUint64(buf[8:], 16+glen)
		_, err = w.Write(buf[:])
	} else {
		buf := [8]byte{}
		if enc == binary.LittleEndian {
			copy(buf[0:], []byte("II"))
		} else {
			copy(buf[0:], []byte("MM"))
		}
		enc.PutUint16(buf[2:], 42)
		enc.PutUint32(buf[4:], 8+uint32(glen))
		_, err = w.Write(buf[:])
	}
	if err != nil {
		return err
	}

	_, err = w.Write([]byte(ghost))
	return err
}

func writeTiffIFD(w io.Writer, bigtiff bool, enc binary.ByteOrder, ifd *IFD, offset uint64, striledata *tagData, next bool) error {
	nextOff := uint64(0)
	if next {
		nextOff = offset + ifd.tagsSize
	}
	var err error

	overflow := &tagData{
		Offset: offset + 8 + 20*ifd.ntags + 8,
	}
	if !bigtiff {
		overflow.Offset = offset + 2 + 12*ifd.ntags + 4
	}

	if bigtiff {
		err = binary.Write(w, enc, ifd.ntags)
	} else {
		err = binary.Write(w, enc, uint16(ifd.ntags))
	}
	if err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	if ifd.NewSubfileType > 0 {
		err := writeTiffField(w, bigtiff, enc, TagNewSubfileType, ifd.NewSubfileType)
		if err != nil {
			panic(err)
		}
	}
	if ifd.ImageWidth > 0 {
		err := writeTiffField(w, bigtiff, enc, TagImageWidth, uint32(ifd.ImageWidth))
		if err != nil {
			panic(err)
		}
	}
	if ifd.ImageLength > 0 {
		err := writeTiffField(w, bigtiff, enc, TagImageLength, uint32(ifd.ImageLength))
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.BitsPerSample) > 0 {
		err := writeTiffArray(w, bigtiff, enc, TagBitsPerSample, ifd.BitsPerSample, overflow)
		if err != nil {
			panic(err)
		}
	}

	if ifd.Compression > 0 {
		err := writeTiffField(w, bigtiff, enc, TagCompression, ifd.Compression)
		if err != nil {
			panic(err)
		}
	}

	err = writeTiffField(w, bigtiff, enc, TagPhotometricInterpretation, ifd.PhotometricInterpretation)
	if err != nil {
		panic(err)
	}

	if len(ifd.DocumentName) > 0 {
		err := writeTiffArray(w, bigtiff, enc, TagDocumentName, ifd.DocumentName, overflow)
		if err != nil {
			panic(err)
		}
	}

	if ifd.SamplesPerPixel > 0 {
		err := writeTiffField(w, bigtiff, enc, TagSamplesPerPixel, ifd.SamplesPerPixel)
		if err != nil {
			panic(err)
		}
	}

	if ifd.PlanarConfiguration > 0 {
		err := writeTiffField(w, bigtiff, enc, TagPlanarConfiguration, ifd.PlanarConfiguration)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.DateTime) > 0 {
		err := writeTiffArray(w, bigtiff, enc, TagDateTime, ifd.DateTime, overflow)
		if err != nil {
			panic(err)
		}
	}

	if ifd.Predictor > 0 {
		err := writeTiffField(w, bigtiff, enc, TagPredictor, ifd.Predictor)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.Colormap) > 0 {
		err := writeTiffArray(w, bigtiff, enc, TagColorMap, ifd.Colormap, overflow)
		if err != nil {
			panic(err)
		}
	}

	if ifd.TileWidth > 0 {
		err := writeTiffField(w, bigtiff, enc, TagTileWidth, ifd.TileWidth)
		if err != nil {
			panic(err)
		}
	}

	if ifd.TileLength > 0 {
		err := writeTiffField(w, bigtiff, enc, TagTileLength, ifd.TileLength)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.NewTileOffsets32) > 0 {
		err := writeTiffArray(w, bigtiff, enc, TagTileOffsets, ifd.NewTileOffsets32, striledata)
		if err != nil {
			panic(err)
		}
	} else {
		err := writeTiffArray(w, bigtiff, enc, TagTileOffsets, ifd.NewTileOffsets64, striledata)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.TileByteCounts) > 0 {
		err := writeTiffArray(w, bigtiff, enc, TagTileByteCounts, ifd.TileByteCounts, striledata)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.ExtraSamples) > 0 {
		err := writeTiffArray(w, bigtiff, enc, TagExtraSamples, ifd.ExtraSamples, overflow)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.SampleFormat) > 0 {
		err := writeTiffArray(w, bigtiff, enc, TagSampleFormat, ifd.SampleFormat, overflow)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.JPEGTables) > 0 {
		err := writeTiffArray(w, bigtiff, enc, TagJPEGTables, ifd.JPEGTables, overflow)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.ModelPixelScaleTag) > 0 {
		err := writeTiffArray(w, bigtiff, enc, TagModelPixelScaleTag, ifd.ModelPixelScaleTag, overflow)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.ModelTiePointTag) > 0 {
		err := writeTiffArray(w, bigtiff, enc, TagModelTiepointTag, ifd.ModelTiePointTag, overflow)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.ModelTransformationTag) > 0 {
		err := writeTiffArray(w, bigtiff, enc, TagModelTransformationTag, ifd.ModelTransformationTag, overflow)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.GeoKeyDirectoryTag) > 0 {
		err := writeTiffArray(w, bigtiff, enc, TagGeoKeyDirectoryTag, ifd.GeoKeyDirectoryTag, overflow)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.GeoDoubleParamsTag) > 0 {
		err := writeTiffArray(w, bigtiff, enc, TagGeoDoubleParamsTag, ifd.GeoDoubleParamsTag, overflow)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.GeoAsciiParamsTag) > 0 {
		err := writeTiffArray(w, bigtiff, enc, TagGeoAsciiParamsTag, ifd.GeoAsciiParamsTag, overflow)
		if err != nil {
			panic(err)
		}
	}

	if ifd.GDALMetaData != "" {
		err := writeTiffArray(w, bigtiff, enc, TagGDAL_METADATA, ifd.GDALMetaData, overflow)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.NoData) > 0 {
		err := writeTiffArray(w, bigtiff, enc, TagGDAL_NODATA, ifd.NoData, overflow)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.LERCParams) > 0 {
		err := writeTiffArray(w, bigtiff, enc, TagLERCParams, ifd.LERCParams, overflow)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.RPCs) > 0 {
		err := writeTiffArray(w, bigtiff, enc, TagRPCs, ifd.RPCs, overflow)
		if err != nil {
			panic(err)
		}
	}

	if bigtiff {
		err = binary.Write(w, enc, nextOff)
	} else {
		err = binary.Write(w, enc, uint32(nextOff))
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

func writeTiffArray(w io.Writer, bigtiff bool, enc binary.ByteOrder, tag uint16, data interface{}, tags *tagData) error {
	var buf []byte
	if bigtiff {
		buf = make([]byte, 20)
	} else {
		buf = make([]byte, 12)
	}
	enc.PutUint16(buf[0:2], tag)
	switch d := data.(type) {
	case []byte:
		n := len(d)
		enc.PutUint16(buf[2:4], tByte)
		if bigtiff {
			enc.PutUint64(buf[4:12], uint64(n))
			if n <= 8 {
				for i := 0; i < n; i++ {
					buf[12+i] = d[i]
				}
			} else {
				enc.PutUint64(buf[12:], tags.NextOffset())
				tags.Write(d)
			}
		} else {
			enc.PutUint32(buf[4:8], uint32(n))
			if n <= 4 {
				for i := 0; i < n; i++ {
					buf[8+i] = d[i]
				}
			} else {
				enc.PutUint32(buf[8:], uint32(tags.NextOffset()))
				tags.Write(d)
			}
		}
	case []uint16:
		n := len(d)
		enc.PutUint16(buf[2:4], tShort)
		if bigtiff {
			enc.PutUint64(buf[4:12], uint64(n))
			if n <= 4 {
				for i := 0; i < n; i++ {
					enc.PutUint16(buf[12+i*2:], d[i])
				}
			} else {
				enc.PutUint64(buf[12:], tags.NextOffset())
				for i := 0; i < n; i++ {
					binary.Write(tags, enc, d[i])
				}
			}
		} else {
			enc.PutUint32(buf[4:8], uint32(n))
			if n <= 2 {
				for i := 0; i < n; i++ {
					enc.PutUint16(buf[8+i*2:], d[i])
				}
			} else {
				enc.PutUint32(buf[8:], uint32(tags.NextOffset()))
				for i := 0; i < n; i++ {
					binary.Write(tags, enc, d[i])
				}
			}
		}
	case []uint32:
		n := len(d)
		enc.PutUint16(buf[2:4], tLong)
		if bigtiff {
			enc.PutUint64(buf[4:12], uint64(n))
			if n <= 2 {
				for i := 0; i < n; i++ {
					enc.PutUint32(buf[12+i*4:], d[i])
				}
			} else {
				enc.PutUint64(buf[12:], tags.NextOffset())
				for i := 0; i < n; i++ {
					binary.Write(tags, enc, d[i])
				}
			}
		} else {
			enc.PutUint32(buf[4:8], uint32(n))
			if n <= 1 {
				for i := 0; i < n; i++ {
					enc.PutUint32(buf[8:], d[i])
				}
			} else {
				enc.PutUint32(buf[8:], uint32(tags.NextOffset()))
				for i := 0; i < n; i++ {
					binary.Write(tags, enc, d[i])
				}
			}
		}
	case []uint64:
		n := len(d)
		enc.PutUint16(buf[2:4], tLong8)
		if bigtiff {
			enc.PutUint64(buf[4:12], uint64(n))
			if n <= 1 {
				enc.PutUint64(buf[12:], d[0])
			} else {
				enc.PutUint64(buf[12:], tags.NextOffset())
				for i := 0; i < n; i++ {
					binary.Write(tags, enc, d[i])
				}
			}
		} else {
			enc.PutUint32(buf[4:8], uint32(n))
			enc.PutUint32(buf[8:], uint32(tags.NextOffset()))
			for i := 0; i < n; i++ {
				binary.Write(tags, enc, d[i])
			}
		}
	case []float32:
		n := len(d)
		enc.PutUint16(buf[2:4], tFloat)
		if bigtiff {
			enc.PutUint64(buf[4:12], uint64(n))
			if n <= 2 {
				for i := 0; i < n; i++ {
					enc.PutUint32(buf[12+i*4:], math.Float32bits(d[i]))
				}
			} else {
				enc.PutUint64(buf[12:], tags.NextOffset())
				for i := 0; i < n; i++ {
					binary.Write(tags, enc, math.Float32bits(d[i]))
				}
			}
		} else {
			enc.PutUint32(buf[4:8], uint32(n))
			if n <= 1 {
				for i := 0; i < n; i++ {
					enc.PutUint32(buf[8:], math.Float32bits(d[i]))
				}
			} else {
				enc.PutUint32(buf[8:], uint32(tags.NextOffset()))
				for i := 0; i < n; i++ {
					binary.Write(tags, enc, math.Float32bits(d[i]))
				}
			}
		}
	case []float64:
		n := len(d)
		enc.PutUint16(buf[2:4], tDouble)
		if bigtiff {
			enc.PutUint64(buf[4:12], uint64(n))
			if n == 1 {
				for i := 0; i < n; i++ {
					enc.PutUint64(buf[12+i*4:], math.Float64bits(d[0]))
				}
			} else {
				enc.PutUint64(buf[12:], tags.NextOffset())
				for i := 0; i < n; i++ {
					binary.Write(tags, enc, math.Float64bits(d[i]))
				}
			}
		} else {
			enc.PutUint32(buf[4:8], uint32(n))
			enc.PutUint32(buf[8:], uint32(tags.NextOffset()))
			for i := 0; i < n; i++ {
				binary.Write(tags, enc, math.Float64bits(d[i]))
			}
		}
	case string:
		n := len(d) + 1
		enc.PutUint16(buf[2:4], tAscii)
		if bigtiff {
			enc.PutUint64(buf[4:12], uint64(n))
			if n <= 8 {
				for i := 0; i < n-1; i++ {
					buf[12+i] = byte(d[i])
				}
				buf[12+n-1] = 0
			} else {
				enc.PutUint64(buf[12:], tags.NextOffset())
				tags.Write(append([]byte(d), 0))
			}
		} else {
			enc.PutUint32(buf[4:8], uint32(n))
			if n <= 4 {
				for i := 0; i < n-1; i++ {
					buf[8+i] = d[i]
				}
				buf[8+n-1] = 0
			} else {
				enc.PutUint32(buf[8:], uint32(tags.NextOffset()))
				tags.Write(append([]byte(d), 0))
			}
		}
	default:
		return fmt.Errorf("unsupported type %v", d)
	}
	var err error
	if bigtiff {
		_, err = w.Write(buf[0:20])
	} else {
		_, err = w.Write(buf[0:12])
	}
	return err
}

func writeTiffField(w io.Writer, bigtiff bool, enc binary.ByteOrder, tag uint16, data interface{}) error {
	if bigtiff {
		var buf [20]byte
		switch d := data.(type) {
		case byte:
			enc.PutUint16(buf[0:2], tag)
			enc.PutUint16(buf[2:4], tByte)
			enc.PutUint64(buf[4:12], 1)
			buf[12] = d
		case uint16:
			enc.PutUint16(buf[0:2], tag)
			enc.PutUint16(buf[2:4], tShort)
			enc.PutUint64(buf[4:12], 1)
			enc.PutUint16(buf[12:], d)
		case uint32:
			enc.PutUint16(buf[0:2], tag)
			enc.PutUint16(buf[2:4], tLong)
			enc.PutUint64(buf[4:12], 1)
			enc.PutUint32(buf[12:], d)
		case uint64:
			enc.PutUint16(buf[0:2], tag)
			enc.PutUint16(buf[2:4], tLong8)
			enc.PutUint64(buf[4:12], 1)
			enc.PutUint64(buf[12:], d)
		case float32:
			enc.PutUint16(buf[0:2], tag)
			enc.PutUint16(buf[2:4], tFloat)
			enc.PutUint64(buf[4:12], 1)
			enc.PutUint32(buf[12:], math.Float32bits(d))
		case float64:
			enc.PutUint16(buf[0:2], tag)
			enc.PutUint16(buf[2:4], tDouble)
			enc.PutUint64(buf[4:12], 1)
			enc.PutUint64(buf[12:], math.Float64bits(d))
		case int8:
			enc.PutUint16(buf[0:2], tag)
			enc.PutUint16(buf[2:4], tSByte)
			enc.PutUint64(buf[4:12], 1)
			buf[12] = byte(d)
		case int16:
			enc.PutUint16(buf[0:2], tag)
			enc.PutUint16(buf[2:4], tSShort)
			enc.PutUint64(buf[4:12], 1)
			enc.PutUint16(buf[12:], uint16(d))
		case int32:
			enc.PutUint16(buf[0:2], tag)
			enc.PutUint16(buf[2:4], tSLong)
			enc.PutUint64(buf[4:12], 1)
			enc.PutUint32(buf[12:], uint32(d))
		case int64:
			enc.PutUint16(buf[0:2], tag)
			enc.PutUint16(buf[2:4], tSLong8)
			enc.PutUint64(buf[4:12], 1)
			enc.PutUint64(buf[12:], uint64(d))
		default:
			panic("unsupported type")
		}
		_, err := w.Write(buf[0:20])
		return err
	} else {
		var buf [12]byte
		switch d := data.(type) {
		case byte:
			enc.PutUint16(buf[0:2], tag)
			enc.PutUint16(buf[2:4], tByte)
			enc.PutUint32(buf[4:8], 1)
			buf[8] = d
		case uint16:
			enc.PutUint16(buf[0:2], tag)
			enc.PutUint16(buf[2:4], tShort)
			enc.PutUint32(buf[4:8], 1)
			enc.PutUint16(buf[8:], d)
		case uint32:
			enc.PutUint16(buf[0:2], tag)
			enc.PutUint16(buf[2:4], tLong)
			enc.PutUint32(buf[4:8], 1)
			enc.PutUint32(buf[8:], d)
		case float32:
			enc.PutUint16(buf[0:2], tag)
			enc.PutUint16(buf[2:4], tFloat)
			enc.PutUint32(buf[4:8], 1)
			enc.PutUint32(buf[8:], math.Float32bits(d))
		case int8:
			enc.PutUint16(buf[0:2], tag)
			enc.PutUint16(buf[2:4], tSByte)
			enc.PutUint32(buf[4:8], 1)
			buf[8] = byte(d)
		case int16:
			enc.PutUint16(buf[0:2], tag)
			enc.PutUint16(buf[2:4], tSShort)
			enc.PutUint32(buf[4:8], 1)
			enc.PutUint16(buf[8:], uint16(d))
		case int32:
			enc.PutUint16(buf[0:2], tag)
			enc.PutUint16(buf[2:4], tSLong)
			enc.PutUint32(buf[4:8], 1)
			enc.PutUint32(buf[8:], uint32(d))
		default:
			panic("unsupported type")
		}
		_, err := w.Write(buf[0:12])
		return err
	}
}

func TestTiffWrite(t *testing.T) {
	gtiff := Read("./scan_512x512_rgb8_tiled.tif")

	if gtiff == nil {
		t.FailNow()
	}

	data := gtiff.Data[0]

	if data == nil {
		t.FailNow()
	}

	enc := binary.LittleEndian

	src := &RawSource{dataOrImage: data.(*image.Paletted), rect: image.Rect(0, 0, 512, 512), ctype: CTLZW, enc: enc}

	if src == nil {
		t.FailNow()
	}

	buf := &bytes.Buffer{}

	ifd := gtiff.ifds[0]

	byteLen, ifd, err := src.Encode(buf, ifd)

	if err != nil || byteLen == 0 {
		t.FailNow()
	}

	ifd.ImageWidth = 512 * 2
	ifd.ImageLength = 512 * 2
	ifd.NewTileOffsets32 = []uint32{0, 0, 0, 0}
	ifd.TileByteCounts = []uint32{uint32(byteLen), uint32(byteLen), uint32(byteLen), uint32(byteLen)}

	f, err := os.Create("./test.tif")

	if err != nil {
		t.FailNow()
	}

	strileData := &tagData{Offset: 8}

	strileData.Offset += uint64(len(ghost))

	glen := uint64(len(ghost))

	writeTiffHeader(f, false, enc)

	off := uint64(8 + glen)

	ifd.SetEPSG(4326, false)

	ifd.ntags, ifd.tagsSize, ifd.strileSize, ifd.nplanes = ifd.structure(false)

	strileData.Offset += ifd.tagsSize

	dataOffset := uint64(8)

	dataOffset += uint64(len(ghost)) + 4
	dataOffset += ifd.strileSize + ifd.tagsSize

	ifd.NewTileOffsets32 = []uint32{uint32(dataOffset), uint32(dataOffset) + uint32(byteLen+8), uint32(dataOffset) + uint32((byteLen+8)*2), uint32(dataOffset) + uint32((byteLen+8)*3)}

	err = writeTiffIFD(f, false, enc, ifd, off, strileData, false)
	if err != nil {
		t.FailNow()
	}
	off += ifd.tagsSize

	_, err = f.Write(strileData.Bytes())
	if err != nil {
		t.FailNow()
	}

	_, err = f.Write(buf.Bytes())
	if err != nil {
		t.FailNow()
	}

	_, err = f.Write(buf.Bytes())
	if err != nil {
		t.FailNow()
	}

	_, err = f.Write(buf.Bytes())
	if err != nil {
		t.FailNow()
	}

	_, err = f.Write(buf.Bytes())
	if err != nil {
		t.FailNow()
	}

	f.Close()

	gtiff = Read("./test.tif")

	if gtiff == nil {
		t.FailNow()
	}

}
