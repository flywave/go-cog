package cog

import (
	"io"

	"github.com/google/tiff"
)

func minInt(a, b int) int {
	if a <= b {
		return a
	}
	return b
}

const (
	TagNewSubfileType            = 254
	TagImageWidth                = 256
	TagImageLength               = 257
	TagBitsPerSample             = 258
	TagCompression               = 259
	TagPhotometricInterpretation = 262
	TagFillOrder                 = 266
	TagDocumentName              = 269
	TagPlanarConfiguration       = 284

	TagStripOffsets    = 273
	TagOrientation     = 274
	TagSamplesPerPixel = 277
	TagRowsPerStrip    = 278
	TagStripByteCounts = 279

	TagTileWidth      = 322
	TagTileLength     = 323
	TagTileOffsets    = 324
	TagTileByteCounts = 325

	TagXResolution    = 282
	TagYResolution    = 283
	TagResolutionUnit = 296

	TagSoftware     = 305
	TagDateTime     = 306
	TagPredictor    = 317
	TagColorMap     = 320
	TagExtraSamples = 338
	TagSampleFormat = 339

	TagJPEGTables = 347

	TagGDAL_METADATA = 42112
	TagGDAL_NODATA   = 42113

	TagModelPixelScaleTag     = 33550
	TagModelTransformationTag = 34264
	TagModelTiepointTag       = 33922
	TagGeoKeyDirectoryTag     = 34735
	TagGeoDoubleParamsTag     = 34736
	TagGeoAsciiParamsTag      = 34737
	TagIntergraphMatrixTag    = 33920

	TagLERCParams = 50674
	TagRPCs       = 50844

	TagGTModelTypeGeoKey              = 1024
	TagGTRasterTypeGeoKey             = 1025
	TagGTCitationGeoKey               = 1026
	TagGeographicTypeGeoKey           = 2048
	TagGeogCitationGeoKey             = 2049
	TagGeogGeodeticDatumGeoKey        = 2050
	TagGeogPrimeMeridianGeoKey        = 2051
	TagGeogLinearUnitsGeoKey          = 2052
	TagGeogLinearUnitSizeGeoKey       = 2053
	TagGeogAngularUnitsGeoKey         = 2054
	TagGeogAngularUnitSizeGeoKey      = 2055
	TagGeogEllipsoidGeoKey            = 2056
	TagGeogSemiMajorAxisGeoKey        = 2057
	TagGeogSemiMinorAxisGeoKey        = 2058
	TagGeogInvFlatteningGeoKey        = 2059
	TagGeogAzimuthUnitsGeoKey         = 2060
	TagGeogPrimeMeridianLongGeoKey    = 2061
	TagProjectedCSTypeGeoKey          = 3072
	TagPCSCitationGeoKey              = 3073
	TagProjectionGeoKey               = 3074
	TagProjCoordTransGeoKey           = 3075
	TagProjLinearUnitsGeoKey          = 3076
	TagProjLinearUnitSizeGeoKey       = 3077
	TagProjStdParallel1GeoKey         = 3078
	TagProjStdParallel2GeoKey         = 3079
	TagProjNatOriginLongGeoKey        = 3080
	TagProjNatOriginLatGeoKey         = 3081
	TagProjFalseEastingGeoKey         = 3082
	TagProjFalseNorthingGeoKey        = 3083
	TagProjFalseOriginLongGeoKey      = 3084
	TagProjFalseOriginLatGeoKey       = 3085
	TagProjFalseOriginEastingGeoKey   = 3086
	TagProjFalseOriginNorthingGeoKey  = 3087
	TagProjCenterLongGeoKey           = 3088
	TagProjCenterLatGeoKey            = 3089
	TagProjCenterEastingGeoKey        = 3090
	TagProjCenterNorthingGeoKey       = 3091
	TagProjScaleAtNatOriginGeoKey     = 3092
	TagProjScaleAtCenterGeoKey        = 3093
	TagProjAzimuthAngleGeoKey         = 3094
	TagProjStraightVertPoleLongGeoKey = 3095
	TagVerticalCSTypeGeoKey           = 4096
	TagVerticalCitationGeoKey         = 4097
	TagVerticalDatumGeoKey            = 4098
	TagVerticalUnitsGeoKey            = 4099

	TagPhotoshop = 34377
)

type SubfileType uint32

const (
	SubfileTypeNone         = 0
	SubfileTypeReducedImage = 1
	SubfileTypePage         = 2
	SubfileTypeMask         = 4
)

type PlanarConfiguration uint16

const (
	PlanarConfigurationContig   = 1
	PlanarConfigurationSeparate = 2
)

type Predictor uint16

const (
	PredictorNone          = 1
	PredictorHorizontal    = 2
	PredictorFloatingPoint = 3
)

type SampleFormat uint16

const (
	SampleFormatUInt          = 1
	SampleFormatInt           = 2
	SampleFormatIEEEFP        = 3
	SampleFormatVoid          = 4
	SampleFormatComplexInt    = 5
	SampleFormatComplexIEEEFP = 6
)

type ExtraSamples uint16

const (
	ExtraSamplesUnspecified = 0
	ExtraSamplesAssocAlpha  = 1
	ExtraSamplesUnassAlpha  = 2
)

type PhotometricInterpretation uint16

const (
	PhotometricInterpretationMinIsWhite = 0
	PhotometricInterpretationMinIsBlack = 1
	PhotometricInterpretationRGB        = 2
	PhotometricInterpretationPalette    = 3
	PhotometricInterpretationMask       = 4
	PhotometricInterpretationSeparated  = 5
	PhotometricInterpretationYCbCr      = 6
	PhotometricInterpretationCIELab     = 8
	PhotometricInterpretationICCLab     = 9
	PhotometricInterpretationITULab     = 10
	PhotometricInterpretationLOGL       = 32844
	PhotometricInterpretationLOGLUV     = 32845
)

type CompressionType uint16

const (
	CTNone       = 1
	CTCCITT      = 2
	CTG3         = 3 // Group 3 Fax.
	CTG4         = 4 // Group 4 Fax.
	CTLZW        = 5
	CTJPEGOld    = 6 // Superseded by cJPEG.
	CTJPEG       = 7
	CTDeflate    = 8 // zlib compression.
	CTPackBits   = 32773
	CTDeflateOld = 32946 // Superseded by cDeflate.
)

type ResolutionUnit uint16

const (
	ResNone    = 1
	ResPerInch = 2 // Dots per inch.
	ResPerCM   = 3 // Dots per centimeter.
)

type ImageMode int

const (
	IBilevel ImageMode = iota
	IPaletted
	IGray
	IGrayInvert
	IRGB
	IRGBA
	INRGBA
)

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

const (
	PI_WhiteIsZero = 0
	PI_BlackIsZero = 1
	PI_RGB         = 2
	PI_Paletted    = 3
	PI_TransMask   = 4 // transparency mask
	PI_CMYK        = 5
	PI_YCbCr       = 6
	PI_CIELab      = 8
)

func ToRGB(data []float64) [][3]uint8 {
	bytes := make([][3]uint8, len(data))
	i := 0
	for _, v := range data {
		val := uint32(v)
		red := uint8((val >> 16) & 0xFF)
		green := uint8((val >> 8) & 0xFF)
		blue := uint8(val & 0xFF)
		bytes[i][0] = red
		bytes[i][1] = green
		bytes[i][2] = blue
		i += 3
	}
	return bytes
}

func ToRGBA(data []float64) [][4]uint8 {
	bytes := make([][4]uint8, len(data))
	i := 0
	for _, v := range data {
		val := uint32(v)
		alpha := uint8((val >> 24) & 0xFF)
		red := uint8((val >> 16) & 0xFF)
		green := uint8((val >> 8) & 0xFF)
		blue := uint8(val & 0xFF)
		bytes[i][0] = red
		bytes[i][1] = green
		bytes[i][2] = blue
		bytes[i][3] = alpha
		i += 4
	}
	return bytes
}

func loadMultipleTIFFs(tifs []tiff.TIFF) ([]*IFD, error) {
	ifds := make([]*IFD, 0)
	for _, tif := range tifs {
		tifds := tif.IFDs()
		for i := range tifds {
			ifd, err := loadIFD(tif.R(), tifds[i])
			if err != nil {
				return nil, err
			}
			ifds = append(ifds, ifd)
		}
	}
	return ifds, nil
}

func loadSingleTIFF(tif tiff.TIFF) ([]*IFD, error) {
	tifds := tif.IFDs()
	ifds := make([]*IFD, len(tifds))
	var err error
	for i := range tifds {
		ifds[i], err = loadIFD(tif.R(), tifds[i])
		if err != nil {
			return nil, err
		}
	}
	return ifds, nil
}

func loadIFD(r tiff.BReader, tifd tiff.IFD) (*IFD, error) {
	ifd := &IFD{r: r}
	err := tiff.UnmarshalIFD(tifd, ifd)
	if err != nil {
		return nil, err
	}
	if len(ifd.TempTileByteCounts) > 0 {
		ifd.TileByteCounts = make([]uint32, len(ifd.TempTileByteCounts))
		for i := range ifd.TempTileByteCounts {
			ifd.TileByteCounts[i] = uint32(ifd.TempTileByteCounts[i])
		}
		ifd.TempTileByteCounts = nil //reclaim mem
	}
	return ifd, nil
}

func encodeGray(w io.Writer, pix []uint8, dx, dy, stride int, predictor bool) error {
	if !predictor {
		return writePix(w, pix, dy, dx, stride)
	}
	buf := make([]byte, dx)
	for y := 0; y < dy; y++ {
		min := y*stride + 0
		max := y*stride + dx
		off := 0
		var v0 uint8
		for i := min; i < max; i++ {
			v1 := pix[i]
			buf[off] = v1 - v0
			v0 = v1
			off++
		}
		if _, err := w.Write(buf); err != nil {
			return err
		}
	}
	return nil
}

func encodeGray16(w io.Writer, pix []uint8, dx, dy, stride int, predictor bool) error {
	buf := make([]byte, dx*2)
	for y := 0; y < dy; y++ {
		min := y*stride + 0
		max := y*stride + dx*2
		off := 0
		var v0 uint16
		for i := min; i < max; i += 2 {
			v1 := uint16(pix[i])<<8 | uint16(pix[i+1])
			if predictor {
				v0, v1 = v1, v1-v0
			}
			buf[off+0] = byte(v1)
			buf[off+1] = byte(v1 >> 8)
			off += 2
		}
		if _, err := w.Write(buf); err != nil {
			return err
		}
	}
	return nil
}

func encodeRGBA(w io.Writer, pix []uint8, dx, dy, stride int, predictor bool) error {
	if !predictor {
		return writePix(w, pix, dy, dx*4, stride)
	}
	buf := make([]byte, dx*4)
	for y := 0; y < dy; y++ {
		min := y*stride + 0
		max := y*stride + dx*4
		off := 0
		var r0, g0, b0, a0 uint8
		for i := min; i < max; i += 4 {
			r1, g1, b1, a1 := pix[i+0], pix[i+1], pix[i+2], pix[i+3]
			buf[off+0] = r1 - r0
			buf[off+1] = g1 - g0
			buf[off+2] = b1 - b0
			buf[off+3] = a1 - a0
			off += 4
			r0, g0, b0, a0 = r1, g1, b1, a1
		}
		if _, err := w.Write(buf); err != nil {
			return err
		}
	}
	return nil
}

func encodeRGBA64(w io.Writer, pix []uint8, dx, dy, stride int, predictor bool) error {
	buf := make([]byte, dx*8)
	for y := 0; y < dy; y++ {
		min := y*stride + 0
		max := y*stride + dx*8
		off := 0
		var r0, g0, b0, a0 uint16
		for i := min; i < max; i += 8 {
			r1 := uint16(pix[i+0])<<8 | uint16(pix[i+1])
			g1 := uint16(pix[i+2])<<8 | uint16(pix[i+3])
			b1 := uint16(pix[i+4])<<8 | uint16(pix[i+5])
			a1 := uint16(pix[i+6])<<8 | uint16(pix[i+7])
			if predictor {
				r0, r1 = r1, r1-r0
				g0, g1 = g1, g1-g0
				b0, b1 = b1, b1-b0
				a0, a1 = a1, a1-a0
			}
			buf[off+0] = byte(r1)
			buf[off+1] = byte(r1 >> 8)
			buf[off+2] = byte(g1)
			buf[off+3] = byte(g1 >> 8)
			buf[off+4] = byte(b1)
			buf[off+5] = byte(b1 >> 8)
			buf[off+6] = byte(a1)
			buf[off+7] = byte(a1 >> 8)
			off += 8
		}
		if _, err := w.Write(buf); err != nil {
			return err
		}
	}
	return nil
}

func writePix(w io.Writer, pix []byte, nrows, length, stride int) error {
	if length == stride {
		_, err := w.Write(pix[:nrows*length])
		return err
	}
	for ; nrows > 0; nrows-- {
		if _, err := w.Write(pix[:length]); err != nil {
			return err
		}
		pix = pix[stride:]
	}
	return nil
}
