package cog

import (
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/flywave/go-cog/lzw"
	"github.com/google/tiff"
)

type GeoTIFF struct {
	Data    [][]float64
	ifds    []*IFD
	bigtiff bool
	enc     binary.ByteOrder
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
	tifds := tif.IFDs()
	ifds := make([]*IFD, 0)
	for i := range tifds {
		ifd, err := loadIFD(tif.R(), tifds[i])
		if err != nil {
			panic(err)
		}
		ifds = append(ifds, ifd)
	}
	m := &GeoTIFF{ifds: ifds}
	for i := range ifds {
		d, _ := m.readData(i)
		m.Data = append(m.Data, d)
	}
	return m
}

func (m GeoTIFF) readData(index int) (data []float64, err error) {
	compressionType := m.ifds[index].Compression
	SampleFormat := uint16(0)
	if len(m.ifds[index].SampleFormat) > 0 {
		SampleFormat = m.ifds[index].SampleFormat[0]
	}

	width := int(m.ifds[index].ImageWidth)
	height := int(m.ifds[index].ImageLength)

	//
	data = make([]float64, width*height)

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

			case CTLZW:
				r := lzw.NewReader(io.NewSectionReader(m.ifds[index].r, offset, n), lzw.MSB, 8)
				defer r.Close()
				buf, err = io.ReadAll(r)

				if err != nil {
					println(err)
				}
			case CTDeflate, CTDeflateOld:
				r, err := zlib.NewReader(io.NewSectionReader(m.ifds[index].r, offset, n))
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
								value := m.ifds[index].r.ByteOrder().Uint16(buf[off : off+2])
								i := y*width + x
								data[i] = float64(value)
								off += 2
							}
						}
					case 32:
						for y := ymin; y < ymax; y++ {
							for x := xmin; x < xmax; x++ {
								value := m.ifds[index].r.ByteOrder().Uint32(buf[off : off+4])
								i := y*width + x
								data[i] = float64(value)
								off += 4
							}
						}
					case 64:
						for y := ymin; y < ymax; y++ {
							for x := xmin; x < xmax; x++ {
								value := m.ifds[index].r.ByteOrder().Uint64(buf[off : off+8])
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
								value := int16(m.ifds[index].r.ByteOrder().Uint16(buf[off : off+2]))
								i := y*width + x
								data[i] = float64(value)
								off += 2
							}
						}
					case 32:
						for y := ymin; y < ymax; y++ {
							for x := xmin; x < xmax; x++ {
								value := int32(m.ifds[index].r.ByteOrder().Uint32(buf[off : off+4]))
								i := y*width + x
								data[i] = float64(value)
								off += 4
							}
						}
					case 64:
						for y := ymin; y < ymax; y++ {
							for x := xmin; x < xmax; x++ {
								value := int64(m.ifds[index].r.ByteOrder().Uint64(buf[off : off+8]))
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
									bits := m.ifds[index].r.ByteOrder().Uint32(buf[off : off+4])
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
									bits := m.ifds[index].r.ByteOrder().Uint64(buf[off : off+8])
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
							red := uint32(float64(m.ifds[index].r.ByteOrder().Uint16(buf[off+0:off+2])) / 65535.0 * 255.0)
							green := uint32(float64(m.ifds[index].r.ByteOrder().Uint16(buf[off+2:off+4])) / 65535.0 * 255.0)
							blue := uint32(float64(m.ifds[index].r.ByteOrder().Uint16(buf[off+4:off+6])) / 65535.0 * 255.0)
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
							red := uint32(float64(m.ifds[index].r.ByteOrder().Uint16(buf[off+0:off+2])) / 65535.0 * 255.0)
							green := uint32(float64(m.ifds[index].r.ByteOrder().Uint16(buf[off+2:off+4])) / 65535.0 * 255.0)
							blue := uint32(float64(m.ifds[index].r.ByteOrder().Uint16(buf[off+4:off+6])) / 65535.0 * 255.0)
							a := uint32(float64(m.ifds[index].r.ByteOrder().Uint16(buf[off+6:off+8])) / 65535.0 * 255.0)
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
							red := uint32(float64(m.ifds[index].r.ByteOrder().Uint16(buf[off+0:off+2])) / 65535.0 * 255.0)
							green := uint32(float64(m.ifds[index].r.ByteOrder().Uint16(buf[off+2:off+4])) / 65535.0 * 255.0)
							blue := uint32(float64(m.ifds[index].r.ByteOrder().Uint16(buf[off+4:off+6])) / 65535.0 * 255.0)
							a := uint32(float64(m.ifds[index].r.ByteOrder().Uint16(buf[off+6:off+8])) / 65535.0 * 255.0)
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
	err := g.computeImageryOffsets()
	if err != nil {
		return err
	}

	strileData := &tagData{Offset: 16}
	if !g.bigtiff {
		strileData.Offset = 8
	}

	strileData.Offset += uint64(len(ghost))

	for _, ifd := range g.ifds {
		strileData.Offset += ifd.tagsSize
	}

	glen := uint64(len(ghost))
	g.writeHeader(out)

	off := uint64(16 + glen)
	if !g.bigtiff {
		off = 8 + glen
	}
	for i, ifd := range g.ifds {
		next := i < len(g.ifds)
		err := g.writeIFD(out, ifd, off, strileData, next)
		if err != nil {
			return fmt.Errorf("write ifd: %w", err)
		}
		off += ifd.tagsSize
	}

	_, err = out.Write(strileData.Bytes())
	if err != nil {
		return fmt.Errorf("write strile pointers: %w", err)
	}

	datas := g.ifds
	tiles := getTiles(datas)
	data := []byte{}
	for tile := range tiles {
		idx := (tile.x+tile.y*tile.ifd.ntilesx)*tile.ifd.nplanes + tile.plane
		bc := tile.ifd.TileByteCounts[idx]
		if tile.ifd.r != nil {
			if bc > 0 {
				_, err := tile.ifd.r.Seek(int64(tile.ifd.OriginalTileOffsets[idx]), io.SeekStart)
				if err != nil {
					return fmt.Errorf("seek to %d: %w", tile.ifd.OriginalTileOffsets[idx], err)
				}
				if uint32(len(data)) < bc+8 {
					data = make([]byte, (bc+8)*2)
				}
				binary.LittleEndian.PutUint32(data, bc)
				_, err = tile.ifd.r.Read(data[4 : 4+bc])
				if err != nil {
					return fmt.Errorf("read %d from %d: %w",
						bc, tile.ifd.OriginalTileOffsets[idx], err)
				}
				copy(data[4+bc:8+bc], data[bc:4+bc])
				_, err = out.Write(data[0 : bc+8])
				if err != nil {
					return fmt.Errorf("write %d: %w", bc, err)
				}
			}
		} else {

		}
	}

	return err
}

func (g *GeoTIFF) computeStructure() {
	for _, ifd := range g.ifds {
		ifd.ntags, ifd.tagsSize, ifd.strileSize, ifd.nplanes = ifd.structure(g.bigtiff)
		ifd.ntilesx = (ifd.ImageWidth + uint64(ifd.TileWidth) - 1) / uint64(ifd.TileWidth)
		ifd.ntilesy = (ifd.ImageLength + uint64(ifd.TileLength) - 1) / uint64(ifd.TileLength)
	}
}

const ghost = `GDAL_STRUCTURAL_METADATA_SIZE=000140 bytes
LAYOUT=IFDS_BEFORE_DATA
BLOCK_ORDER=ROW_MAJOR
BLOCK_LEADER=SIZE_AS_UINT4
BLOCK_TRAILER=LAST_4_BYTES_REPEATED
KNOWN_INCOMPATIBLE_EDITION=NO
  `

func (g *GeoTIFF) computeImageryOffsets() error {
	for _, ifd := range g.ifds {
		if g.bigtiff {
			ifd.NewTileOffsets64 = make([]uint64, len(ifd.OriginalTileOffsets))
			ifd.NewTileOffsets32 = nil
		} else {
			ifd.NewTileOffsets32 = make([]uint32, len(ifd.OriginalTileOffsets))
			ifd.NewTileOffsets64 = nil
		}
	}
	g.computeStructure()

	dataOffset := uint64(16)
	if !g.bigtiff {
		dataOffset = 8
	}

	dataOffset += uint64(len(ghost)) + 4

	for _, ifd := range g.ifds {
		dataOffset += ifd.strileSize + ifd.tagsSize
	}

	datas := g.ifds
	tiles := getTiles(datas)
	for tile := range tiles {
		tileidx := (tile.x+tile.y*tile.ifd.ntilesx)*tile.ifd.nplanes + tile.plane
		cnt := uint64(tile.ifd.TileByteCounts[tileidx])
		if cnt > 0 {
			if g.bigtiff {
				tile.ifd.NewTileOffsets64[tileidx] = dataOffset
			} else {
				if dataOffset > uint64(^uint32(0)) { //^uint32(0) is max uint32
					//rerun with bigtiff support

					//first empty out the tiles channel to avoid a goroutine leak
					for range tiles {
						//skip
					}
					g.bigtiff = true
					return g.computeImageryOffsets()
				}
				tile.ifd.NewTileOffsets32[tileidx] = uint32(dataOffset)
			}
			dataOffset += uint64(tile.ifd.TileByteCounts[tileidx]) + 8
		} else {
			if g.bigtiff {
				tile.ifd.NewTileOffsets64[tileidx] = 0
			} else {
				tile.ifd.NewTileOffsets32[tileidx] = 0
			}
		}
	}

	return nil
}

func (g *GeoTIFF) writeHeader(w io.Writer) error {
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

func (g *GeoTIFF) writeIFD(w io.Writer, ifd *IFD, offset uint64, striledata *tagData, next bool) error {
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

	if ifd.SubfileType > 0 {
		err := g.writeField(w, TagNewSubfileType, ifd.SubfileType)
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

type TiledTiff struct {
	ifd   *IFD
	x, y  uint64
	plane uint64
}

func getTiles(d []*IFD) chan TiledTiff {
	ch := make(chan TiledTiff)
	go func() {
		defer close(ch)

		for _, ovr := range d {
			for y := uint64(0); y < ovr.ntilesy; y++ {
				for x := uint64(0); x < ovr.ntilesx; x++ {
					for p := uint64(0); p < ovr.nplanes; p++ {
						ch <- TiledTiff{
							ifd:   ovr,
							plane: p,
							x:     x,
							y:     y,
						}
					}
				}
			}
		}

	}()
	return ch
}
