package cog

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/google/tiff"

	_ "github.com/google/tiff/bigtiff"
	_ "github.com/google/tiff/geotiff"
)

type IFD struct {
	SubfileType               uint32   `tiff:"field,tag=254"`
	ImageWidth                uint64   `tiff:"field,tag=256"`
	ImageLength               uint64   `tiff:"field,tag=257"`
	BitsPerSample             []uint16 `tiff:"field,tag=258"`
	Compression               uint16   `tiff:"field,tag=259"`
	PhotometricInterpretation uint16   `tiff:"field,tag=262"`
	DocumentName              string   `tiff:"field,tag=269"`
	SamplesPerPixel           uint16   `tiff:"field,tag=277"`
	PlanarConfiguration       uint16   `tiff:"field,tag=284"`
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

	overview *IFD
	masks    []*IFD

	ntags            uint64
	ntilesx, ntilesy uint64
	nplanes          uint64 //1 if PlanarConfiguration==1, SamplesPerPixel if PlanarConfiguration==2
	tagsSize         uint64
	strileSize       uint64
	r                tiff.BReader
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

func (ifd *IFD) AddOverview(ovr *IFD) {
	ovr.SubfileType = SubfileTypeReducedImage
	ovr.ModelPixelScaleTag = nil
	ovr.ModelTiePointTag = nil
	ovr.ModelTransformationTag = nil
	ovr.GeoAsciiParamsTag = ""
	ovr.GeoDoubleParamsTag = nil
	ovr.GeoKeyDirectoryTag = nil
	ifd.overview = ovr
}

func (ifd *IFD) AddMask(msk *IFD) error {
	if len(msk.masks) > 0 || msk.overview != nil {
		return fmt.Errorf("cannot add mask with overviews or masks")
	}
	switch ifd.SubfileType {
	case SubfileTypeNone:
		msk.SubfileType = SubfileTypeMask
	case SubfileTypeReducedImage:
		msk.SubfileType = SubfileTypeMask | SubfileTypeReducedImage
	default:
		return fmt.Errorf("invalid subfiledtype")
	}
	msk.ModelPixelScaleTag = nil
	msk.ModelTiePointTag = nil
	msk.ModelTransformationTag = nil
	msk.GeoAsciiParamsTag = ""
	msk.GeoDoubleParamsTag = nil
	msk.GeoKeyDirectoryTag = nil
	ifd.masks = append(ifd.masks, msk)
	return nil
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

type Cog struct {
	enc     binary.ByteOrder
	ifd     *IFD
	bigtiff bool
}

func NewCog() *Cog {
	return &Cog{enc: binary.LittleEndian}
}

func (cog *Cog) writeHeader(w io.Writer) error {
	glen := uint64(len(ghost))
	if len(cog.ifd.masks) > 0 {
		glen = uint64(len(ghostmask))
	}
	var err error
	if cog.bigtiff {
		buf := [16]byte{}
		if cog.enc == binary.LittleEndian {
			copy(buf[0:], []byte("II"))
		} else {
			copy(buf[0:], []byte("MM"))
		}
		cog.enc.PutUint16(buf[2:], 43)
		cog.enc.PutUint16(buf[4:], 8)
		cog.enc.PutUint16(buf[6:], 0)
		cog.enc.PutUint64(buf[8:], 16+glen)
		_, err = w.Write(buf[:])
	} else {
		buf := [8]byte{}
		if cog.enc == binary.LittleEndian {
			copy(buf[0:], []byte("II"))
		} else {
			copy(buf[0:], []byte("MM"))
		}
		cog.enc.PutUint16(buf[2:], 42)
		cog.enc.PutUint32(buf[4:], 8+uint32(glen))
		_, err = w.Write(buf[:])
	}
	if err != nil {
		return err
	}
	if len(cog.ifd.masks) > 0 {
		_, err = w.Write([]byte(ghostmask))
	} else {
		_, err = w.Write([]byte(ghost))
	}
	return err
}

const (
	tByte      = 1
	tAscii     = 2
	tShort     = 3
	tLong      = 4
	tRational  = 5
	tSByte     = 6
	tUndefined = 7
	tSShort    = 8
	tSLong     = 9
	tSRational = 10
	tFloat     = 11
	tDouble    = 12
	tLong8     = 16
	tSLong8    = 17
	tIFD8      = 18
)

func (cog *Cog) computeStructure() {
	ifd := cog.ifd
	for ifd != nil {
		ifd.ntags, ifd.tagsSize, ifd.strileSize, ifd.nplanes = ifd.structure(cog.bigtiff)

		ifd.ntilesx = (ifd.ImageWidth + uint64(ifd.TileWidth) - 1) / uint64(ifd.TileWidth)
		ifd.ntilesy = (ifd.ImageLength + uint64(ifd.TileLength) - 1) / uint64(ifd.TileLength)

		for _, mifd := range ifd.masks {
			mifd.ntags, mifd.tagsSize, mifd.strileSize, mifd.nplanes = mifd.structure(cog.bigtiff)

			mifd.ntilesx = (mifd.ImageWidth + uint64(mifd.TileWidth) - 1) / uint64(mifd.TileWidth)
			mifd.ntilesy = (mifd.ImageLength + uint64(mifd.TileLength) - 1) / uint64(mifd.TileLength)
		}
		ifd = ifd.overview
	}
}

const ghost = `GDAL_STRUCTURAL_METADATA_SIZE=000140 bytes
LAYOUT=IFDS_BEFORE_DATA
BLOCK_ORDER=ROW_MAJOR
BLOCK_LEADER=SIZE_AS_UINT4
BLOCK_TRAILER=LAST_4_BYTES_REPEATED
KNOWN_INCOMPATIBLE_EDITION=NO
  ` //2 spaces: 1 for the gdal spec, and one to ensure the actual start offset is on a word boundary

const ghostmask = `GDAL_STRUCTURAL_METADATA_SIZE=000174 bytes
LAYOUT=IFDS_BEFORE_DATA
BLOCK_ORDER=ROW_MAJOR
BLOCK_LEADER=SIZE_AS_UINT4
BLOCK_TRAILER=LAST_4_BYTES_REPEATED
KNOWN_INCOMPATIBLE_EDITION=NO
 MASK_INTERLEAVED_WITH_IMAGERY=YES
`

func (cog *Cog) computeImageryOffsets() error {
	ifd := cog.ifd
	for ifd != nil {
		if cog.bigtiff {
			ifd.NewTileOffsets64 = make([]uint64, len(ifd.OriginalTileOffsets))
			ifd.NewTileOffsets32 = nil
		} else {
			ifd.NewTileOffsets32 = make([]uint32, len(ifd.OriginalTileOffsets))
			ifd.NewTileOffsets64 = nil
		}
		//mifd.NewTileOffsets = mifd.OriginalTileOffsets
		for _, sc := range ifd.masks {
			if cog.bigtiff {
				sc.NewTileOffsets64 = make([]uint64, len(sc.OriginalTileOffsets))
				sc.NewTileOffsets32 = nil
			} else {
				sc.NewTileOffsets32 = make([]uint32, len(sc.OriginalTileOffsets))
				sc.NewTileOffsets64 = nil
			}
			//sc.NewTileOffsets = sc.OriginalTileOffsets
		}
		ifd = ifd.overview
	}
	cog.computeStructure()

	//offset to start of image data
	dataOffset := uint64(16)
	if !cog.bigtiff {
		dataOffset = 8
	}
	if len(cog.ifd.masks) > 0 {
		dataOffset += uint64(len(ghostmask)) + 4
	} else {
		dataOffset += uint64(len(ghost)) + 4
	}

	ifd = cog.ifd
	for ifd != nil {
		dataOffset += ifd.strileSize + ifd.tagsSize
		for _, sc := range ifd.masks {
			dataOffset += sc.strileSize + sc.tagsSize
		}
		ifd = ifd.overview
	}

	datas := cog.dataInterlacing()
	tiles := datas.tiles()
	for tile := range tiles {
		tileidx := (tile.x+tile.y*tile.ifd.ntilesx)*tile.ifd.nplanes + tile.plane
		cnt := uint64(tile.ifd.TileByteCounts[tileidx])
		if cnt > 0 {
			if cog.bigtiff {
				tile.ifd.NewTileOffsets64[tileidx] = dataOffset
			} else {
				if dataOffset > uint64(^uint32(0)) { //^uint32(0) is max uint32
					//rerun with bigtiff support

					//first empty out the tiles channel to avoid a goroutine leak
					for range tiles {
						//skip
					}
					cog.bigtiff = true
					return cog.computeImageryOffsets()
				}
				tile.ifd.NewTileOffsets32[tileidx] = uint32(dataOffset)
			}
			dataOffset += uint64(tile.ifd.TileByteCounts[tileidx]) + 8
		} else {
			if cog.bigtiff {
				tile.ifd.NewTileOffsets64[tileidx] = 0
			} else {
				tile.ifd.NewTileOffsets32[tileidx] = 0
			}
		}
	}

	return nil
}

func (cog *Cog) Write(out io.Writer) error {
	err := cog.computeImageryOffsets()
	if err != nil {
		return err
	}

	strileData := &tagData{Offset: 16}
	if !cog.bigtiff {
		strileData.Offset = 8
	}
	if len(cog.ifd.masks) > 0 {
		strileData.Offset += uint64(len(ghostmask))
	} else {
		strileData.Offset += uint64(len(ghost))
	}

	ifd := cog.ifd
	for ifd != nil {
		strileData.Offset += ifd.tagsSize
		for _, sc := range ifd.masks {
			strileData.Offset += sc.tagsSize
		}
		ifd = ifd.overview
	}

	glen := uint64(len(ghost))
	if len(cog.ifd.masks) > 0 {
		glen = uint64(len(ghostmask))
	}
	cog.writeHeader(out)

	ifd = cog.ifd
	off := uint64(16 + glen)
	if !cog.bigtiff {
		off = 8 + glen
	}
	for ifd != nil {
		nmasks := len(ifd.masks)
		err := cog.writeIFD(out, ifd, off, strileData, nmasks > 0 || ifd.overview != nil)
		if err != nil {
			return fmt.Errorf("write ifd: %w", err)
		}
		off += ifd.tagsSize
		for i, si := range ifd.masks {
			err := cog.writeIFD(out, si, off, strileData, i != nmasks-1 || ifd.overview != nil)
			if err != nil {
				return fmt.Errorf("write ifd: %w", err)
			}
			off += si.tagsSize
		}
		ifd = ifd.overview
	}

	_, err = out.Write(strileData.Bytes())
	if err != nil {
		return fmt.Errorf("write strile pointers: %w", err)
	}

	datas := cog.dataInterlacing()
	tiles := datas.tiles()
	data := []byte{}
	for tile := range tiles {
		idx := (tile.x+tile.y*tile.ifd.ntilesx)*tile.ifd.nplanes + tile.plane
		bc := tile.ifd.TileByteCounts[idx]
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
	}

	return err
}

func (cog *Cog) writeIFD(w io.Writer, ifd *IFD, offset uint64, striledata *tagData, next bool) error {

	nextOff := uint64(0)
	if next {
		nextOff = offset + ifd.tagsSize
	}
	var err error

	overflow := &tagData{
		Offset: offset + 8 + 20*ifd.ntags + 8,
	}
	if !cog.bigtiff {
		overflow.Offset = offset + 2 + 12*ifd.ntags + 4
	}

	if cog.bigtiff {
		err = binary.Write(w, cog.enc, ifd.ntags)
	} else {
		err = binary.Write(w, cog.enc, uint16(ifd.ntags))
	}
	if err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	if ifd.SubfileType > 0 {
		err := cog.writeField(w, TagNewSubfileType, ifd.SubfileType)
		if err != nil {
			panic(err)
		}
	}
	if ifd.ImageWidth > 0 {
		err := cog.writeField(w, TagImageWidth, uint32(ifd.ImageWidth))
		if err != nil {
			panic(err)
		}
	}
	if ifd.ImageLength > 0 {
		err := cog.writeField(w, TagImageLength, uint32(ifd.ImageLength))
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.BitsPerSample) > 0 {
		err := cog.writeArray(w, TagBitsPerSample, ifd.BitsPerSample, overflow)
		if err != nil {
			panic(err)
		}
	}

	if ifd.Compression > 0 {
		err := cog.writeField(w, TagCompression, ifd.Compression)
		if err != nil {
			panic(err)
		}
	}

	err = cog.writeField(w, TagPhotometricInterpretation, ifd.PhotometricInterpretation)
	if err != nil {
		panic(err)
	}

	if len(ifd.DocumentName) > 0 {
		err := cog.writeArray(w, TagDocumentName, ifd.DocumentName, overflow)
		if err != nil {
			panic(err)
		}
	}

	if ifd.SamplesPerPixel > 0 {
		err := cog.writeField(w, TagSamplesPerPixel, ifd.SamplesPerPixel)
		if err != nil {
			panic(err)
		}
	}

	if ifd.PlanarConfiguration > 0 {
		err := cog.writeField(w, TagPlanarConfiguration, ifd.PlanarConfiguration)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.DateTime) > 0 {
		err := cog.writeArray(w, TagDateTime, ifd.DateTime, overflow)
		if err != nil {
			panic(err)
		}
	}

	if ifd.Predictor > 0 {
		err := cog.writeField(w, TagPredictor, ifd.Predictor)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.Colormap) > 0 {
		err := cog.writeArray(w, TagColorMap, ifd.Colormap, overflow)
		if err != nil {
			panic(err)
		}
	}

	if ifd.TileWidth > 0 {
		err := cog.writeField(w, TagTileWidth, ifd.TileWidth)
		if err != nil {
			panic(err)
		}
	}

	if ifd.TileLength > 0 {
		err := cog.writeField(w, TagTileLength, ifd.TileLength)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.NewTileOffsets32) > 0 {
		err := cog.writeArray(w, TagTileOffsets, ifd.NewTileOffsets32, striledata)
		if err != nil {
			panic(err)
		}
	} else {
		err := cog.writeArray(w, 324, ifd.NewTileOffsets64, striledata)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.TileByteCounts) > 0 {
		err := cog.writeArray(w, TagTileByteCounts, ifd.TileByteCounts, striledata)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.ExtraSamples) > 0 {
		err := cog.writeArray(w, TagExtraSamples, ifd.ExtraSamples, overflow)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.SampleFormat) > 0 {
		err := cog.writeArray(w, TagSampleFormat, ifd.SampleFormat, overflow)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.JPEGTables) > 0 {
		err := cog.writeArray(w, TagJPEGTables, ifd.JPEGTables, overflow)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.ModelPixelScaleTag) > 0 {
		err := cog.writeArray(w, TagModelPixelScaleTag, ifd.ModelPixelScaleTag, overflow)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.ModelTiePointTag) > 0 {
		err := cog.writeArray(w, TagModelTiepointTag, ifd.ModelTiePointTag, overflow)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.ModelTransformationTag) > 0 {
		err := cog.writeArray(w, TagModelTransformationTag, ifd.ModelTransformationTag, overflow)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.GeoKeyDirectoryTag) > 0 {
		err := cog.writeArray(w, TagGeoKeyDirectoryTag, ifd.GeoKeyDirectoryTag, overflow)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.GeoDoubleParamsTag) > 0 {
		err := cog.writeArray(w, TagGeoDoubleParamsTag, ifd.GeoDoubleParamsTag, overflow)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.GeoAsciiParamsTag) > 0 {
		err := cog.writeArray(w, TagGeoAsciiParamsTag, ifd.GeoAsciiParamsTag, overflow)
		if err != nil {
			panic(err)
		}
	}

	if ifd.GDALMetaData != "" {
		err := cog.writeArray(w, TagGDAL_METADATA, ifd.GDALMetaData, overflow)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.NoData) > 0 {
		err := cog.writeArray(w, TagGDAL_NODATA, ifd.NoData, overflow)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.LERCParams) > 0 {
		err := cog.writeArray(w, TagLERCParams, ifd.LERCParams, overflow)
		if err != nil {
			panic(err)
		}
	}

	if len(ifd.RPCs) > 0 {
		err := cog.writeArray(w, TagRPCs, ifd.RPCs, overflow)
		if err != nil {
			panic(err)
		}
	}

	if cog.bigtiff {
		err = binary.Write(w, cog.enc, nextOff)
	} else {
		err = binary.Write(w, cog.enc, uint32(nextOff))
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

type Tile struct {
	ifd   *IFD
	x, y  uint64
	plane uint64
}

type datas [][]*IFD

func (cog *Cog) dataInterlacing() datas {
	ifdo := cog.ifd
	count := 0
	for ifdo != nil {
		count++
		ifdo = ifdo.overview
	}
	ret := make([][]*IFD, count)
	ifdo = cog.ifd
	for idx := count - 1; idx >= 0; idx-- {
		ret[idx] = append(ret[idx], ifdo)
		ret[idx] = append(ret[idx], ifdo.masks...)
		ifdo = ifdo.overview
	}
	return ret
}

func (d datas) tiles() chan Tile {
	ch := make(chan Tile)
	go func() {
		defer close(ch)

		for _, ovr := range d {
			for y := uint64(0); y < ovr[0].ntilesy; y++ {
				for x := uint64(0); x < ovr[0].ntilesx; x++ {
					for _, ifd := range ovr {
						for p := uint64(0); p < ifd.nplanes; p++ {
							ch <- Tile{
								ifd:   ifd,
								plane: p,
								x:     x,
								y:     y,
							}
						}
					}
				}
			}
		}

	}()
	return ch
}
