package cog

import (
	"bytes"

	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/flywave/go-cog/lzw"
	"github.com/google/tiff"
	"github.com/google/tiff/bigtiff"
)

type GeoTIFF struct {
	Cog
	Tif  tiff.TIFF
	Data []float64
}

func Read(fileName string) *GeoTIFF {
	f, err := os.Open(fileName)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	tif, err := tiff.Parse(f, nil, nil)
	if err != nil {
		panic(err)
	}
	m := &GeoTIFF{Tif: tif}
	m.Data, _ = m.readData()
	return m
}

func (m GeoTIFF) HasField(tagID uint16) bool {
	ifds := m.Tif.IFDs()
	index := 0
	for index >= 0 {
		if ifds[index].HasField(tagID) {
			return true
		}
		index--
	}
	return false
}

func (m GeoTIFF) GetField(tagID uint16) tiff.Field {
	ifds := m.Tif.IFDs()
	index := 0
	for index >= 0 {
		if ifds[index].HasField(tagID) {
			return ifds[index].GetField(tagID)
		}
		index--
	}
	return nil
}

func (m GeoTIFF) GetFirstInt(tagID uint16) uint {
	field := m.GetField(tagID)
	val := field.Value()
	bs := val.Bytes()
	size := field.Type().Size()
	//
	switch field.Type() {
	case tiff.FTByte:
		return uint(bs[0])
	case tiff.FTShort:
		return uint(val.Order().Uint16(bs[:size]))
	case tiff.FTLong: //
		return uint(val.Order().Uint32(bs[:size]))
	}
	return 0
}

func (m GeoTIFF) GetInt(tagID uint16) []uint {
	field := m.GetField(tagID)
	count := field.Count()
	size := field.Type().Size()
	u := make([]uint, count)
	val := field.Value()
	bs := val.Bytes()

	switch field.Type() {
	case tiff.FTByte:
		for i := uint64(0); i < count; i++ {
			u[i] = uint(bs[i])
		}
	case tiff.FTShort:
		for i := uint64(0); i < count; i++ {
			u[i] = uint(val.Order().Uint16(bs[size*i : size*(i+1)]))
		}
	case tiff.FTLong:
		for i := uint64(0); i < count; i++ {
			u[i] = uint(val.Order().Uint32(bs[size*i : size*(i+1)]))
		}
	case bigtiff.FTLong8:
		for i := uint64(0); i < count; i++ {
			u[i] = uint(val.Order().Uint32(bs[size*i : size*(i+1)]))
		}
	}
	return u
}

func (m GeoTIFF) GetFloat(tagID uint16) []float64 {
	field := m.GetField(tagID)
	count := field.Count()
	u := make([]float64, count)
	val := field.Value()
	bs := val.Bytes()
	switch field.Type() {
	case tiff.FTFloat:
		u2 := make([]float32, count)
		for i := uint64(0); i < count; i++ {
			buf := bytes.NewReader(bs[4*i : 4*(i+1)])
			binary.Read(buf, val.Order(), &u2[i])
		}
		for i := uint64(0); i < count; i++ {
			u[i] = float64(u2[i])
		}
	case tiff.FTDouble:
		for i := uint64(0); i < count; i++ {
			buf := bytes.NewReader(bs[8*i : 8*(i+1)])
			binary.Read(buf, val.Order(), &u[i])
		}

	}
	return u
}

func (m GeoTIFF) Origin() []float64 {
	origin := m.GetFloat(TagModelTiepointTag)
	return origin[3:6]
}

func (m GeoTIFF) Scale() []float64 {
	return m.GetFloat(TagModelPixelScaleTag)
}

func (m GeoTIFF) Transform() []float64 {
	return m.GetFloat(TagModelTransformationTag)
}

func (m GeoTIFF) GetColRow(lon, lat float64) [2]int {
	origin := m.Origin()
	scale := m.Scale()
	col := int(math.Round((lon - origin[0]) / scale[0]))
	row := int(math.Round((origin[1] - lat) / scale[1]))
	width, height := m.Width(), m.Height()
	if row >= height {
		row = height - 1
	}
	if col >= width {
		col = width - 1
	}
	return [2]int{col, row}
}

func (m GeoTIFF) GetLonLat(col, row int) [2]float64 {
	ModelTiepoint := m.GetFloat(TagModelTiepointTag)
	scale := m.Scale()

	lon := (float64(col)-ModelTiepoint[0])*scale[0] + ModelTiepoint[3]
	lat := (float64(row)-ModelTiepoint[1])*scale[1]*(-1) + ModelTiepoint[4]

	return [2]float64{lon, lat}
}

func (m GeoTIFF) BBox() [4]float64 {
	origin := m.Origin()
	scale := m.Scale()
	var maxX = origin[0] + (scale[0] * float64(m.Width()))
	var minY = origin[1] - (scale[1] * float64(m.Height()))
	return [4]float64{
		origin[0], origin[1], maxX, minY,
	}
}

func (m GeoTIFF) Width() int {
	return int(m.GetFirstInt(TagImageWidth))
}

func (m GeoTIFF) Height() int {
	return int(m.GetFirstInt(TagImageLength))
}

func (m GeoTIFF) GetAlt(column, row int) float64 {
	return m.Data[column+row*m.Width()]
}

func (m GeoTIFF) GetAltByLonLat(lon, lat float64) float64 {
	colrow := m.GetColRow(lon, lat)
	col, row := colrow[0], colrow[1]
	fmt.Println(col, row)
	return m.GetAlt(col, row)
}

func (m GeoTIFF) readData() (data []float64, err error) {
	tif := m.Tif

	compressionType := m.GetFirstInt(TagCompression)
	SampleFormat := m.GetFirstInt(TagSampleFormat)

	width := m.Width()
	height := m.Height()

	//
	data = make([]float64, width*height)

	var off int
	bitsPerSample := m.GetInt(TagBitsPerSample)
	blockPadding := false
	blockWidth := int(width)
	blockHeight := int(height)
	blocksAcross := 1
	blocksDown := 1

	var blockOffsets, blockCounts []uint

	if m.HasField(TagTileWidth) {
		tileWidth := int(m.GetFirstInt(TagTileWidth))
		tileHeight := int(m.GetFirstInt(TagTileLength))

		blockPadding = true

		blockWidth = int(tileWidth)
		blockHeight = int(tileHeight)

		blocksAcross = (width + blockWidth - 1) / blockWidth
		blocksDown = (height + blockHeight - 1) / blockHeight

		if ok := m.HasField(TagTileOffsets); ok {
			blockOffsets = m.GetInt(TagTileOffsets)
		}

		if ok := m.HasField(TagTileByteCounts); ok {
			blockCounts = m.GetInt(TagTileByteCounts)
		}

	} else {
		if m.HasField(TagRowsPerStrip) {
			blockHeight = int(m.GetFirstInt(TagRowsPerStrip))
		}

		blocksDown = (height + blockHeight - 1) / blockHeight

		if ok := m.HasField(TagStripOffsets); ok {
			blockOffsets = m.GetInt(TagStripOffsets)
		}

		if ok := m.HasField(TagStripByteCounts); ok {
			blockCounts = m.GetInt(TagStripByteCounts)
		}
	}
	var buf []byte

	for i := 0; i < blocksAcross; i++ {
		blkW := blockWidth
		if !blockPadding && i == blocksAcross-1 && width%blockWidth != 0 {
			blkW = width % blockWidth
		}
		for j := 0; j < blocksDown; j++ {
			blkH := blockHeight
			if !blockPadding && j == blocksDown-1 && height%blockHeight != 0 {
				blkH = height % blockHeight
			}
			offset := int64(blockOffsets[j*blocksAcross+i])
			n := int64(blockCounts[j*blocksAcross+i])
			switch compressionType {
			case CTNone:
				buf = make([]byte, n)
				_, err = tif.R().ReadAt(buf, offset)

			case CTLZW:
				r := lzw.NewReader(io.NewSectionReader(tif.R(), offset, n), lzw.MSB, 8)
				defer r.Close()
				buf, err = io.ReadAll(r)

				if err != nil {
					println(err)
				}
			case CTDeflate, CTDeflateOld:
				r, err := zlib.NewReader(io.NewSectionReader(tif.R(), offset, n))
				if err != nil {
					return nil, err
				}
				buf, err = io.ReadAll(r)
				if err != nil {
					return nil, err
				}
				r.Close()
			case CTPackBits:

			default:
				err = fmt.Errorf("unsupported compression value %d", compressionType)
			}

			xmin := i * blockWidth
			ymin := j * blockHeight
			xmax := xmin + blkW
			ymax := ymin + blkH

			xmax = minInt(xmax, width)
			ymax = minInt(ymax, height)

			off = 0

			if m.HasField(TagPredictor) && m.GetFirstInt(TagPredictor) == PredictorHorizontal {
				if bitsPerSample[0] == 16 {
					var off int
					spp := len(bitsPerSample)
					bpp := spp * 2
					for y := ymin; y < ymax; y++ {
						off += spp * 2
						for x := 0; x < (xmax-xmin-1)*bpp; x += 2 {
							v0 := tif.R().ByteOrder().Uint16(buf[off-bpp : off-bpp+2])
							v1 := tif.R().ByteOrder().Uint16(buf[off : off+2])
							tif.R().ByteOrder().PutUint16(buf[off:off+2], v1+v0)
							off += 2
						}
					}
				} else if bitsPerSample[0] == 8 {
					var off int
					spp := len(bitsPerSample)
					for y := ymin; y < ymax; y++ {
						off += spp
						for x := 0; x < (xmax-xmin-1)*spp; x++ {
							buf[off] += buf[off-spp]
							off++
						}
					}
				}
			}
			var mode ImageMode
			PhotometricInterp := m.GetFirstInt(TagPhotometricInterpretation)
			var palette []uint32

			switch PhotometricInterp {
			case PI_RGB:
				if bitsPerSample[0] == 16 {
					for _, b := range bitsPerSample {
						if b != 16 {
							err = errors.New("wrong number of samples for 16bit RGB")
							return
						}
					}
				} else {
					for _, b := range bitsPerSample {
						if b != 8 {
							err = errors.New("wrong number of samples for 8bit RGB")
							return
						}
					}
				}

				switch len(bitsPerSample) {
				case 3:
					mode = IRGB
				case 4:
					switch m.GetFirstInt(TagExtraSamples) {
					case 1:
						mode = IRGBA
					case 2:
						mode = INRGBA
					default:
						err = errors.New("wrong number of samples for RGB")
						return
					}
				default:
					err = errors.New("wrong number of samples for RGB")
					return
				}
			case PI_Paletted:
				mode = IPaletted
				if ok := m.HasField(TagColorMap); ok {
					val := m.GetInt(TagColorMap)
					numcolors := len(val) / 3
					if len(val)%3 != 0 || numcolors <= 0 || numcolors > 256 {
						return nil, errors.New("bad ColorMap length")
					}
					palette = make([]uint32, numcolors)
					for i := 0; i < numcolors; i++ {
						red := uint32(float64(val[i]) / 65535.0 * 255.0)
						green := uint32(float64(val[i+numcolors]) / 65535.0 * 255.0)
						blue := uint32(float64(val[i+2*numcolors]) / 65535.0 * 255.0)
						a := uint32(255)
						val := uint32((a << 24) | (red << 16) | (green << 8) | blue)
						palette[i] = val
					}
				} else {
					err = errors.New("could not locate the colour map tag")
					return
				}
			case PI_WhiteIsZero:
				mode = IGrayInvert
			case PI_BlackIsZero:
				mode = IGray
			default:
				err = errors.New("unsupported image format")
				return
			}

			switch mode {
			case IGray, IGrayInvert:
				switch SampleFormat {
				case 1:
					switch bitsPerSample[0] {
					case 8:
						for y := ymin; y < ymax; y++ {
							for x := xmin; x < xmax; x++ {
								i := y*width + x
								data[i] = float64(buf[off])
								off++
							}
						}
					case 16:
						for y := ymin; y < ymax; y++ {
							for x := xmin; x < xmax; x++ {
								value := tif.R().ByteOrder().Uint16(buf[off : off+2])
								i := y*width + x
								data[i] = float64(value)
								off += 2
							}
						}
					case 32:
						for y := ymin; y < ymax; y++ {
							for x := xmin; x < xmax; x++ {
								value := tif.R().ByteOrder().Uint32(buf[off : off+4])
								i := y*width + x
								data[i] = float64(value)
								off += 4
							}
						}
					case 64:
						for y := ymin; y < ymax; y++ {
							for x := xmin; x < xmax; x++ {
								value := tif.R().ByteOrder().Uint64(buf[off : off+8])
								i := y*width + x
								data[i] = float64(value)
								off += 8
							}
						}
					default:
						err = errors.New("unsupported data format")
						return
					}
				case 2:
					switch bitsPerSample[0] {
					case 8:
						for y := ymin; y < ymax; y++ {
							for x := xmin; x < xmax; x++ {
								i := y*width + x
								data[i] = float64(int8(buf[off]))
								off++
							}
						}
					case 16:
						for y := ymin; y < ymax; y++ {
							for x := xmin; x < xmax; x++ {
								value := int16(tif.R().ByteOrder().Uint16(buf[off : off+2]))
								i := y*width + x
								data[i] = float64(value)
								off += 2
							}
						}
					case 32:
						for y := ymin; y < ymax; y++ {
							for x := xmin; x < xmax; x++ {
								value := int32(tif.R().ByteOrder().Uint32(buf[off : off+4]))
								i := y*width + x
								data[i] = float64(value)
								off += 4
							}
						}
					case 64:
						for y := ymin; y < ymax; y++ {
							for x := xmin; x < xmax; x++ {
								value := int64(tif.R().ByteOrder().Uint64(buf[off : off+8]))
								i := y*width + x
								data[i] = float64(value)
								off += 8
							}
						}
					default:
						err = errors.New("unsupported data format")
						return
					}
				case 3:
					switch bitsPerSample[0] {
					case 32:
						for y := ymin; y < ymax; y++ {
							for x := xmin; x < xmax; x++ {
								if off <= len(buf) {
									bits := tif.R().ByteOrder().Uint32(buf[off : off+4])
									float := math.Float32frombits(bits)
									i := y*width + x
									data[i] = float64(float)
									off += 4
								}
							}
						}
					case 64:
						for y := ymin; y < ymax; y++ {
							for x := xmin; x < xmax; x++ {
								if off <= len(buf) {
									bits := tif.R().ByteOrder().Uint64(buf[off : off+8])
									float := math.Float64frombits(bits)
									i := y*width + x
									data[i] = float
									off += 8
								}
							}
						}
					default:
						err = errors.New("unsupported data format")
						return
					}
				default:
					err = errors.New("unsupported sample format")
					return
				}
			case IPaletted:
				for y := ymin; y < ymax; y++ {
					for x := xmin; x < xmax; x++ {
						i := y*width + x
						val := int(buf[off])
						data[i] = float64(palette[val])
						off++
					}
				}

			case IRGB:
				if bitsPerSample[0] == 8 {
					for y := ymin; y < ymax; y++ {
						for x := xmin; x < xmax; x++ {
							red := uint32(buf[off])
							green := uint32(buf[off+1])
							blue := uint32(buf[off+2])
							a := uint32(255)
							off += 3
							i := y*width + x
							val := uint32((a << 24) | (red << 16) | (green << 8) | blue)
							data[i] = float64(val)
						}
					}
				} else if bitsPerSample[0] == 16 {
					for y := ymin; y < ymax; y++ {
						for x := xmin; x < xmax; x++ {
							red := uint32(float64(tif.R().ByteOrder().Uint16(buf[off+0:off+2])) / 65535.0 * 255.0)
							green := uint32(float64(tif.R().ByteOrder().Uint16(buf[off+2:off+4])) / 65535.0 * 255.0)
							blue := uint32(float64(tif.R().ByteOrder().Uint16(buf[off+4:off+6])) / 65535.0 * 255.0)
							a := uint32(255)
							off += 6
							i := y*width + x
							val := uint32((a << 24) | (red << 16) | (green << 8) | blue)
							data[i] = float64(val)
						}
					}
				} else {
					err = errors.New("unsupported data format")
					return
				}
			case INRGBA:
				if bitsPerSample[0] == 8 {
					for y := ymin; y < ymax; y++ {
						for x := xmin; x < xmax; x++ {
							red := uint32(buf[off])
							green := uint32(buf[off+1])
							blue := uint32(buf[off+2])
							a := uint32(buf[off+3])
							off += 4
							i := y*width + x
							val := uint32((a << 24) | (red << 16) | (green << 8) | blue)
							data[i] = float64(val)
						}
					}
				} else if bitsPerSample[0] == 16 {
					for y := ymin; y < ymax; y++ {
						for x := xmin; x < xmax; x++ {
							red := uint32(float64(tif.R().ByteOrder().Uint16(buf[off+0:off+2])) / 65535.0 * 255.0)
							green := uint32(float64(tif.R().ByteOrder().Uint16(buf[off+2:off+4])) / 65535.0 * 255.0)
							blue := uint32(float64(tif.R().ByteOrder().Uint16(buf[off+4:off+6])) / 65535.0 * 255.0)
							a := uint32(float64(tif.R().ByteOrder().Uint16(buf[off+6:off+8])) / 65535.0 * 255.0)
							off += 8
							i := y*width + x
							val := uint32((a << 24) | (red << 16) | (green << 8) | blue)
							data[i] = float64(val)
						}
					}
				} else {
					err = errors.New("unsupported data format")
					return
				}
			case IRGBA:
				if bitsPerSample[0] == 16 {
					for y := ymin; y < ymax; y++ {
						for x := xmin; x < xmax; x++ {
							red := uint32(float64(tif.R().ByteOrder().Uint16(buf[off+0:off+2])) / 65535.0 * 255.0)
							green := uint32(float64(tif.R().ByteOrder().Uint16(buf[off+2:off+4])) / 65535.0 * 255.0)
							blue := uint32(float64(tif.R().ByteOrder().Uint16(buf[off+4:off+6])) / 65535.0 * 255.0)
							a := uint32(float64(tif.R().ByteOrder().Uint16(buf[off+6:off+8])) / 65535.0 * 255.0)
							off += 8
							i := y*width + x
							val := uint32((a << 24) | (red << 16) | (green << 8) | blue)
							data[i] = float64(val)
						}
					}
				} else {
					for y := ymin; y < ymax; y++ {
						for x := xmin; x < xmax; x++ {
							red := uint32(buf[off])
							green := uint32(buf[off+1])
							blue := uint32(buf[off+2])
							a := uint32(buf[off+3])
							off += 4
							i := y*width + x
							val := uint32((a << 24) | (red << 16) | (green << 8) | blue)
							data[i] = float64(val)
						}
					}
				}
			}
		}
	}

	return
}

func (g *GeoTIFF) writeData(out io.Writer) error {
	return nil
}
