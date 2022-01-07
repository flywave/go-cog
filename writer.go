package cog

import (
	"encoding/binary"
	"fmt"
	"io"
)

type Writer struct {
	tiles   []*TileLayer
	bigtiff bool
	enc     binary.ByteOrder
}

func (g *Writer) Close() error {
	for i := range g.tiles {
		g.tiles[i].Close()
	}
	return nil
}

func (g *Writer) writeData(out io.Writer) error {
	err := g.computeImageryOffsets()
	if err != nil {
		return err
	}

	strileData := &tagData{Offset: 16}
	if !g.bigtiff {
		strileData.Offset = 8
	}

	strileData.Offset += uint64(len(ghost))

	for _, t := range g.tiles {
		strileData.Offset += t.ifd.tagsSize
	}

	glen := uint64(len(ghost))
	g.writeHeader(out)

	off := uint64(16 + glen)
	if !g.bigtiff {
		off = 8 + glen
	}
	for i, t := range g.tiles {
		next := i < len(g.tiles)
		err := g.writeIFD(out, t.ifd, off, strileData, next)
		if err != nil {
			return fmt.Errorf("write ifd: %w", err)
		}
		off += t.ifd.tagsSize
	}

	_, err = out.Write(strileData.Bytes())
	if err != nil {
		return fmt.Errorf("write strile pointers: %w", err)
	}

	datas := g.tiles
	tiles := getTiles(datas)

	var data []byte
	for tile := range tiles {
		idx := (tile.x + tile.y*uint64(tile.layer.col))
		bc := tile.layer.ifd.TileByteCounts[idx]
		if bc > 0 {
			_, err := tile.layer.tempFile.Seek(int64(tile.layer.ifd.OriginalTileOffsets[idx]), io.SeekStart)
			if err != nil {
				return err
			}
			if uint32(len(data)) < bc+8 {
				data = make([]byte, (bc+8)*2)
			}
			binary.LittleEndian.PutUint32(data, bc)
			_, err = tile.layer.tempFile.Read(data[4 : 4+bc])
			if err != nil {
				return err
			}
			copy(data[4+bc:8+bc], data[bc:4+bc])
			_, err = out.Write(data[0 : bc+8])
			if err != nil {
				return fmt.Errorf("write %d: %w", bc, err)
			}
		}
	}

	return err
}

func (g *Writer) computeStructure() {
	for _, t := range g.tiles {
		ifd := t.ifd
		ifd.ntags, ifd.tagsSize, ifd.strileSize, ifd.nplanes = ifd.structure(g.bigtiff)
	}
}

const ghost = `GDAL_STRUCTURAL_METADATA_SIZE=000140 bytes
LAYOUT=IFDS_BEFORE_DATA
BLOCK_ORDER=ROW_MAJOR
BLOCK_LEADER=SIZE_AS_UINT4
BLOCK_TRAILER=LAST_4_BYTES_REPEATED
KNOWN_INCOMPATIBLE_EDITION=NO
  `

func (g *Writer) computeImageryOffsets() error {
	for _, t := range g.tiles {
		err := t.computeStructure(g.enc)
		if err != nil {
			return err
		}
		ifd := t.ifd
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

	for _, t := range g.tiles {
		dataOffset += t.ifd.strileSize + t.ifd.tagsSize
	}

	datas := g.tiles
	tiles := getTiles(datas)
	for tile := range tiles {
		tileidx := (tile.x + tile.y*uint64(tile.layer.col))
		cnt := uint64(tile.layer.ifd.TileByteCounts[tileidx])
		if cnt > 0 {
			if g.bigtiff {
				tile.layer.ifd.NewTileOffsets64[tileidx] = dataOffset
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
				tile.layer.ifd.NewTileOffsets32[tileidx] = uint32(dataOffset)
			}
			dataOffset += uint64(tile.layer.ifd.TileByteCounts[tileidx]) + 8
		} else {
			if g.bigtiff {
				tile.layer.ifd.NewTileOffsets64[tileidx] = 0
			} else {
				tile.layer.ifd.NewTileOffsets32[tileidx] = 0
			}
		}
	}

	return nil
}

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

type TiledTiff struct {
	tile  *Tile
	x, y  uint64
	layer *TileLayer
}

func getTiles(d []*TileLayer) chan TiledTiff {
	ch := make(chan TiledTiff)
	go func() {
		defer close(ch)
		for _, l := range d {
			for _, tile := range l.tiles {
				ch <- TiledTiff{
					tile:  tile,
					x:     uint64(tile.block[0]),
					y:     uint64(tile.block[1]),
					layer: l,
				}
			}
		}
	}()
	return ch
}
