package cog

import (
	"compress/zlib"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"io/ioutil"
	"math"
	"os"

	"github.com/google/tiff"
	"github.com/hhrutter/lzw"
	"golang.org/x/image/ccitt"
)

type Reader struct {
	Data []interface{} // []uint16 |  []uint32 | []uint64 | []int16 |  []int32 | []int64 | []float32 | []float64 | image.Image
	ifds []*IFD
}

func Read(fileName string) *Reader {
	f, err := os.Open(fileName)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	tif, err := tiff.Parse(f, nil, nil)
	if err != nil {
		panic(err)
	}
	tifds := tif.IFDs()

	ifds := make([]*IFD, 0)
	for i := range tifds {
		if err := sanityCheckIFD(tifds[i]); err != nil {
			return nil
		}
		ifd, err := loadIFD(tif.R(), tifds[i])
		if err != nil {
			panic(err)
		}
		ifds = append(ifds, ifd)
	}
	m := &Reader{ifds: ifds}
	for i := range ifds {
		d, _, _ := m.readData(i)
		m.Data = append(m.Data, d)
	}
	return m
}

func ccittFillOrder(tiffFillOrder uint) ccitt.Order {
	if tiffFillOrder == 2 {
		return ccitt.LSB
	}
	return ccitt.MSB
}

func (m Reader) readData(index int) (data interface{}, rect image.Rectangle, err error) {
	compressionType := m.ifds[index].Compression
	SampleFormat := uint16(0)
	if len(m.ifds[index].SampleFormat) > 0 {
		SampleFormat = m.ifds[index].SampleFormat[0]
	}

	width := int(m.ifds[index].ImageWidth)
	height := int(m.ifds[index].ImageLength)

	rect = image.Rect(0, 0, width, height)

	var off int
	bitsPerSample := m.ifds[index].BitsPerSample
	blockPadding := false
	blockWidth := int(width)
	blockHeight := int(height)
	blocksAcross := 1
	blocksDown := 1

	var blockOffsets, blockCounts []uint32

	if m.ifds[index].TileWidth != 0 {
		tileWidth := int(m.ifds[index].TileWidth)
		tileHeight := int(m.ifds[index].TileLength)

		blockPadding = true

		blockWidth = int(tileWidth)
		blockHeight = int(tileHeight)

		blocksAcross = (width + blockWidth - 1) / blockWidth
		blocksDown = (height + blockHeight - 1) / blockHeight

		if len(m.ifds[index].OriginalTileOffsets) > 0 {
			blockOffsets = make([]uint32, len(m.ifds[index].OriginalTileOffsets))
			for i, off := range m.ifds[index].OriginalTileOffsets {
				blockOffsets[i] = uint32(off)
			}
		}

		if len(m.ifds[index].TileByteCounts) > 0 {
			blockCounts = m.ifds[index].TileByteCounts
		}
	} else {
		if len(m.ifds[index].TileByteCounts) > 0 {
			blockHeight = int(m.ifds[index].TileByteCounts[0])
		}

		blocksDown = (height + blockHeight - 1) / blockHeight

		if len(m.ifds[index].StripOffsets) > 0 {
			blockOffsets = m.ifds[index].StripOffsets
		}

		if len(m.ifds[index].StripByteCounts) > 0 {
			blockCounts = m.ifds[index].StripByteCounts
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
				_, err = m.ifds[index].r.ReadAt(buf, offset)
			case CTG3:
				inv := m.ifds[index].PhotometricInterpretation == PI_WhiteIsZero
				order := ccittFillOrder(uint(m.ifds[index].FillOrder))
				r := ccitt.NewReader(io.NewSectionReader(m.ifds[index].r, offset, n), order, ccitt.Group3, blkW, blkH, &ccitt.Options{Invert: inv, Align: false})
				buf, err = ioutil.ReadAll(r)
			case CTG4:
				inv := m.ifds[index].PhotometricInterpretation == PI_WhiteIsZero
				order := ccittFillOrder(uint(m.ifds[index].FillOrder))
				r := ccitt.NewReader(io.NewSectionReader(m.ifds[index].r, offset, n), order, ccitt.Group4, blkW, blkH, &ccitt.Options{Invert: inv, Align: false})
				buf, err = ioutil.ReadAll(r)
			case CTLZW:
				r := lzw.NewReader(io.NewSectionReader(m.ifds[index].r, offset, n), true)
				defer r.Close()
				buf, err = io.ReadAll(r)

				if err != nil {
					println(err)
				}
			case CTDeflate, CTDeflateOld:
				r, err := zlib.NewReader(io.NewSectionReader(m.ifds[index].r, offset, n))
				if err != nil {
					return nil, image.Rectangle{}, err
				}
				buf, err = io.ReadAll(r)
				if err != nil {
					return nil, image.Rectangle{}, err
				}
				r.Close()
			case CTPackBits:
				buf, err = unpackBits(io.NewSectionReader(m.ifds[index].r, offset, n))
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

			if m.ifds[index].Predictor == PredictorHorizontal {
				if bitsPerSample[0] == 16 {
					var off int
					spp := len(bitsPerSample)
					bpp := spp * 2
					for y := ymin; y < ymax; y++ {
						off += spp * 2
						for x := 0; x < (xmax-xmin-1)*bpp; x += 2 {
							v0 := m.ifds[index].r.ByteOrder().Uint16(buf[off-bpp : off-bpp+2])
							v1 := m.ifds[index].r.ByteOrder().Uint16(buf[off : off+2])
							m.ifds[index].r.ByteOrder().PutUint16(buf[off:off+2], v1+v0)
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
			PhotometricInterp := m.ifds[index].PhotometricInterpretation
			var palette color.Palette

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
					es := uint16(0)
					if len(m.ifds[index].ExtraSamples) > 0 {
						es = m.ifds[index].ExtraSamples[0]
					}
					switch es {
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
				if len(m.ifds[index].Colormap) > 0 {
					val := m.ifds[index].Colormap
					numcolors := len(val) / 3
					if len(val)%3 != 0 || numcolors <= 0 || numcolors > 256 {
						return nil, image.Rectangle{}, errors.New("bad ColorMap length")
					}
					palette = make(color.Palette, numcolors)
					for i := 0; i < numcolors; i++ {
						red := uint8(float64(val[i]) / 65535.0 * 255.0)
						green := uint8(float64(val[i+numcolors]) / 65535.0 * 255.0)
						blue := uint8(float64(val[i+2*numcolors]) / 65535.0 * 255.0)
						a := uint8(255)
						palette[i] = color.RGBA{R: red, G: green, B: blue, A: a}
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
						var ddata []uint8
						if data == nil {
							ddata = make([]uint8, width*height)
							data = ddata
						} else {
							ddata = data.([]uint8)
						}
						for y := ymin; y < ymax; y++ {
							for x := xmin; x < xmax; x++ {
								i := y*width + x
								ddata[i] = buf[off]
								off++
							}
						}
					case 16:
						var ddata []uint16
						if data == nil {
							ddata = make([]uint16, width*height)
							data = ddata
						} else {
							ddata = data.([]uint16)
						}
						for y := ymin; y < ymax; y++ {
							for x := xmin; x < xmax; x++ {
								value := m.ifds[index].r.ByteOrder().Uint16(buf[off : off+2])
								i := y*width + x
								ddata[i] = value
								off += 2
							}
						}
					case 32:
						var ddata []uint32
						if data == nil {
							ddata = make([]uint32, width*height)
							data = ddata
						} else {
							ddata = data.([]uint32)
						}
						for y := ymin; y < ymax; y++ {
							for x := xmin; x < xmax; x++ {
								value := m.ifds[index].r.ByteOrder().Uint32(buf[off : off+4])
								i := y*width + x
								ddata[i] = value
								off += 4
							}
						}
					case 64:
						var ddata []uint64
						if data == nil {
							ddata = make([]uint64, width*height)
							data = ddata
						} else {
							ddata = data.([]uint64)
						}
						for y := ymin; y < ymax; y++ {
							for x := xmin; x < xmax; x++ {
								value := m.ifds[index].r.ByteOrder().Uint64(buf[off : off+8])
								i := y*width + x
								ddata[i] = value
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
						var ddata []int8
						if data == nil {
							ddata = make([]int8, width*height)
							data = ddata
						} else {
							ddata = data.([]int8)
						}
						for y := ymin; y < ymax; y++ {
							for x := xmin; x < xmax; x++ {
								i := y*width + x
								ddata[i] = int8(buf[off])
								off++
							}
						}
					case 16:
						var ddata []int16
						if data == nil {
							ddata = make([]int16, width*height)
							data = ddata
						} else {
							ddata = data.([]int16)
						}
						for y := ymin; y < ymax; y++ {
							for x := xmin; x < xmax; x++ {
								value := int16(m.ifds[index].r.ByteOrder().Uint16(buf[off : off+2]))
								i := y*width + x
								ddata[i] = value
								off += 2
							}
						}
					case 32:
						var ddata []int32
						if data == nil {
							ddata = make([]int32, width*height)
							data = ddata
						} else {
							ddata = data.([]int32)
						}
						for y := ymin; y < ymax; y++ {
							for x := xmin; x < xmax; x++ {
								value := int32(m.ifds[index].r.ByteOrder().Uint32(buf[off : off+4]))
								i := y*width + x
								ddata[i] = value
								off += 4
							}
						}
					case 64:
						var ddata []int64
						if data == nil {
							ddata = make([]int64, width*height)
							data = ddata
						} else {
							ddata = data.([]int64)
						}
						for y := ymin; y < ymax; y++ {
							for x := xmin; x < xmax; x++ {
								value := int64(m.ifds[index].r.ByteOrder().Uint64(buf[off : off+8]))
								i := y*width + x
								ddata[i] = value
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
						var ddata []float32
						if data == nil {
							ddata = make([]float32, width*height)
							data = ddata
						} else {
							ddata = data.([]float32)
						}
						for y := ymin; y < ymax; y++ {
							for x := xmin; x < xmax; x++ {
								if off <= len(buf) {
									bits := m.ifds[index].r.ByteOrder().Uint32(buf[off : off+4])
									float := math.Float32frombits(bits)
									i := y*width + x
									ddata[i] = float
									off += 4
								}
							}
						}
					case 64:
						var ddata []float64
						if data == nil {
							ddata = make([]float64, width*height)
							data = ddata
						} else {
							ddata = data.([]float64)
						}
						for y := ymin; y < ymax; y++ {
							for x := xmin; x < xmax; x++ {
								if off <= len(buf) {
									bits := m.ifds[index].r.ByteOrder().Uint64(buf[off : off+8])
									float := math.Float64frombits(bits)
									i := y*width + x
									ddata[i] = float
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
				var ddata *image.Paletted
				if data == nil {
					ddata = image.NewPaletted(image.Rect(0, 0, width, height), palette)
					data = ddata
				} else {
					ddata = data.(*image.Paletted)
				}
				for y := ymin; y < ymax; y++ {
					for x := xmin; x < xmax; x++ {
						val := int(buf[off])
						ddata.Set(x, y, palette[val])
						off++
					}
				}
			case IRGB:
				if bitsPerSample[0] == 8 {
					var ddata *image.RGBA
					if data == nil {
						ddata = image.NewRGBA(image.Rect(0, 0, width, height))
						data = ddata
					} else {
						ddata = data.(*image.RGBA)
					}
					for y := ymin; y < ymax; y++ {
						for x := xmin; x < xmax; x++ {
							red := uint8(buf[off])
							green := uint8(buf[off+1])
							blue := uint8(buf[off+2])
							a := uint8(255)
							off += 3
							ddata.Set(x, y, color.RGBA{R: red, G: green, B: blue, A: a})
						}
					}
				} else if bitsPerSample[0] == 16 {
					var ddata *image.RGBA64
					if data == nil {
						ddata = image.NewRGBA64(image.Rect(0, 0, width, height))
						data = ddata
					} else {
						ddata = data.(*image.RGBA64)
					}
					for y := ymin; y < ymax; y++ {
						for x := xmin; x < xmax; x++ {
							red := m.ifds[index].r.ByteOrder().Uint16(buf[off+0 : off+2])
							green := m.ifds[index].r.ByteOrder().Uint16(buf[off+2 : off+4])
							blue := m.ifds[index].r.ByteOrder().Uint16(buf[off+4 : off+6])
							a := uint16(255)
							off += 6
							ddata.Set(x, y, color.RGBA64{R: red, G: green, B: blue, A: a})
						}
					}
				} else {
					err = errors.New("unsupported data format")
					return
				}
			case INRGBA:
				if bitsPerSample[0] == 8 {
					var ddata *image.NRGBA
					if data == nil {
						ddata = image.NewNRGBA(image.Rect(0, 0, width, height))
						data = ddata
					} else {
						ddata = data.(*image.NRGBA)
					}
					for y := ymin; y < ymax; y++ {
						for x := xmin; x < xmax; x++ {
							red := uint8(buf[off])
							green := uint8(buf[off+1])
							blue := uint8(buf[off+2])
							a := uint8(buf[off+3])
							off += 4
							ddata.Set(x, y, color.NRGBA{R: red, G: green, B: blue, A: a})
						}
					}
				} else if bitsPerSample[0] == 16 {
					var ddata *image.NRGBA64
					if data == nil {
						ddata = image.NewNRGBA64(image.Rect(0, 0, width, height))
						data = ddata
					} else {
						ddata = data.(*image.NRGBA64)
					}
					for y := ymin; y < ymax; y++ {
						for x := xmin; x < xmax; x++ {
							red := m.ifds[index].r.ByteOrder().Uint16(buf[off+0 : off+2])
							green := m.ifds[index].r.ByteOrder().Uint16(buf[off+2 : off+4])
							blue := m.ifds[index].r.ByteOrder().Uint16(buf[off+4 : off+6])
							a := m.ifds[index].r.ByteOrder().Uint16(buf[off+6 : off+8])
							off += 8
							ddata.Set(x, y, color.NRGBA64{R: red, G: green, B: blue, A: a})
						}
					}
				} else {
					err = errors.New("unsupported data format")
					return
				}
			case IRGBA:
				if bitsPerSample[0] == 16 {
					var ddata *image.RGBA64
					if data == nil {
						ddata = image.NewRGBA64(image.Rect(0, 0, width, height))
						data = ddata
					} else {
						ddata = data.(*image.RGBA64)
					}
					for y := ymin; y < ymax; y++ {
						for x := xmin; x < xmax; x++ {
							red := m.ifds[index].r.ByteOrder().Uint16(buf[off+0 : off+2])
							green := m.ifds[index].r.ByteOrder().Uint16(buf[off+2 : off+4])
							blue := m.ifds[index].r.ByteOrder().Uint16(buf[off+4 : off+6])
							a := m.ifds[index].r.ByteOrder().Uint16(buf[off+6 : off+8])
							off += 8
							ddata.Set(x, y, color.RGBA64{R: red, G: green, B: blue, A: a})
						}
					}
				} else {
					var ddata *image.RGBA
					if data == nil {
						ddata = image.NewRGBA(image.Rect(0, 0, width, height))
						data = ddata
					} else {
						ddata = data.(*image.RGBA)
					}
					for y := ymin; y < ymax; y++ {
						for x := xmin; x < xmax; x++ {
							red := uint8(buf[off])
							green := uint8(buf[off+1])
							blue := uint8(buf[off+2])
							a := uint8(buf[off+3])
							off += 4
							ddata.Set(x, y, color.NRGBA{R: red, G: green, B: blue, A: a})
						}
					}
				}
			}
		}
	}

	return
}
