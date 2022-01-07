package cog

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"image"
	"io"

	"github.com/hhrutter/lzw"
)

type TileSource interface {
	Bounds() image.Rectangle
	Encode(w io.Writer, ifd *IFD) (uint32, *IFD, error)
	Reset()
}

type RawSource struct {
	dataOrImage               interface{} // []uint16 |  []uint32 | []uint64 | []int16 |  []int32 | []int64 | []float32 | []float64 | image.Image
	rect                      image.Rectangle
	ctype                     CompressionType
	photometricInterpretation uint32
	samplesPerPixel           uint32
	bitsPerSample             []uint16
	extraSamples              uint16
	colorMap                  []uint16
	sampleFormat              []uint16
	enc                       binary.ByteOrder
}

func (s *RawSource) Reset() {
	s.dataOrImage = nil
}

func (s *RawSource) Bounds() image.Rectangle {
	switch m := s.dataOrImage.(type) {
	case *image.Paletted:
		return m.Bounds()
	case *image.Gray:
		return m.Bounds()
	case *image.Gray16:
		return m.Bounds()
	case *image.RGBA64:
		return m.Bounds()
	case *image.NRGBA64:
		return m.Bounds()
	}
	return s.rect
}

func (s *RawSource) Encode(w io.Writer, ifd *IFD) (uint32, *IFD, error) {
	d := s.Bounds().Size()

	compression := s.ctype

	var buf bytes.Buffer
	var dst io.Writer
	var imageLen int

	switch compression {
	case CTNone:
		dst = w
		switch s.dataOrImage.(type) {
		case *image.Paletted:
			imageLen = d.X * d.Y * 1
		case *image.Gray:
			imageLen = d.X * d.Y * 1
		case *image.Gray16:
			imageLen = d.X * d.Y * 2
		case *image.RGBA64:
			imageLen = d.X * d.Y * 8
		case *image.NRGBA64:
			imageLen = d.X * d.Y * 8
		case []uint16:
			imageLen = d.X * d.Y * 2
		case []uint32:
			imageLen = d.X * d.Y * 4
		case []uint64:
			imageLen = d.X * d.Y * 8
		case []int16:
			imageLen = d.X * d.Y * 2
		case []int32:
			imageLen = d.X * d.Y * 4
		case []int64:
			imageLen = d.X * d.Y * 8
		case []float32:
			imageLen = d.X * d.Y * 4
		case []float64:
			imageLen = d.X * d.Y * 8
		default:
			imageLen = d.X * d.Y * 4
		}
		err := binary.Write(w, s.enc, uint32(imageLen+8))
		if err != nil {
			return 0, nil, err
		}
	case CTDeflate:
		dst = zlib.NewWriter(&buf)
	case CTLZW:
		dst = lzw.NewWriter(&buf, true)
	}

	s.photometricInterpretation = uint32(PI_RGB)
	s.samplesPerPixel = uint32(4)
	s.bitsPerSample = []uint16{8, 8, 8, 8}
	s.extraSamples = uint16(0)
	s.colorMap = []uint16{}
	s.sampleFormat = []uint16{}

	var err error
	switch m := s.dataOrImage.(type) {
	case *image.Paletted:
		s.photometricInterpretation = PI_Paletted
		s.samplesPerPixel = 1
		s.bitsPerSample = []uint16{8}
		s.colorMap = make([]uint16, 256*3)
		for i := 0; i < 256 && i < len(m.Palette); i++ {
			r, g, b, _ := m.Palette[i].RGBA()
			s.colorMap[i+0*256] = uint16(r)
			s.colorMap[i+1*256] = uint16(g)
			s.colorMap[i+2*256] = uint16(b)
		}
		err = encodeGray(dst, m.Pix, d.X, d.Y, m.Stride)
	case *image.Gray:
		s.photometricInterpretation = PI_BlackIsZero
		s.samplesPerPixel = 1
		s.bitsPerSample = []uint16{8}
		s.sampleFormat = []uint16{1}
		err = encodeGray(dst, m.Pix, d.X, d.Y, m.Stride)
	case *image.Gray16:
		s.photometricInterpretation = PI_BlackIsZero
		s.samplesPerPixel = 1
		s.bitsPerSample = []uint16{16}
		s.sampleFormat = []uint16{1}
		err = encodeGray16(dst, m.Pix, d.X, d.Y, m.Stride)
	case *image.NRGBA:
		s.extraSamples = 2
		err = encodeRGBA(dst, m.Pix, d.X, d.Y, m.Stride)
	case *image.NRGBA64:
		s.extraSamples = 2
		s.bitsPerSample = []uint16{16, 16, 16, 16}
		err = encodeRGBA64(dst, m.Pix, d.X, d.Y, m.Stride)
	case *image.RGBA:
		s.extraSamples = 1
		err = encodeRGBA(dst, m.Pix, d.X, d.Y, m.Stride)
	case *image.RGBA64:
		s.extraSamples = 1
		s.bitsPerSample = []uint16{16, 16, 16, 16}
		err = encodeRGBA64(dst, m.Pix, d.X, d.Y, m.Stride)
	case []uint16:
		s.photometricInterpretation = PI_BlackIsZero
		s.samplesPerPixel = 1
		s.bitsPerSample = []uint16{16}
		s.sampleFormat = []uint16{1}
		err = encodeUInt16(dst, s.Bounds(), s.enc, m)
	case []uint32:
		s.photometricInterpretation = PI_BlackIsZero
		s.samplesPerPixel = 1
		s.bitsPerSample = []uint16{32}
		s.sampleFormat = []uint16{1}
		err = encodeUInt32(dst, s.Bounds(), s.enc, m)
	case []uint64:
		s.photometricInterpretation = PI_BlackIsZero
		s.samplesPerPixel = 1
		s.bitsPerSample = []uint16{64}
		s.sampleFormat = []uint16{1}
		err = encodeUInt64(dst, s.Bounds(), s.enc, m)
	case []int16:
		s.photometricInterpretation = PI_BlackIsZero
		s.samplesPerPixel = 1
		s.bitsPerSample = []uint16{16}
		s.sampleFormat = []uint16{2}
		err = encodeInt16(dst, s.Bounds(), s.enc, m)
	case []int32:
		s.photometricInterpretation = PI_BlackIsZero
		s.samplesPerPixel = 1
		s.bitsPerSample = []uint16{32}
		s.sampleFormat = []uint16{2}
		err = encodeInt32(dst, s.Bounds(), s.enc, m)
	case []int64:
		s.photometricInterpretation = PI_BlackIsZero
		s.samplesPerPixel = 1
		s.bitsPerSample = []uint16{64}
		s.sampleFormat = []uint16{2}
		err = encodeInt64(dst, s.Bounds(), s.enc, m)
	case []float32:
		s.photometricInterpretation = PI_BlackIsZero
		s.samplesPerPixel = 1
		s.bitsPerSample = []uint16{32}
		s.sampleFormat = []uint16{3}
		err = encodeFloat32(dst, s.Bounds(), s.enc, m)
	case []float64:
		s.photometricInterpretation = PI_BlackIsZero
		s.samplesPerPixel = 1
		s.bitsPerSample = []uint16{64}
		s.sampleFormat = []uint16{3}
		err = encodeFloat64(dst, s.Bounds(), s.enc, m)
	default:
		s.extraSamples = 1
		err = encode(dst, m.(image.Image))
	}
	if err != nil {
		return 0, nil, err
	}

	if compression != CTNone {
		if err = dst.(io.Closer).Close(); err != nil {
			return 0, nil, err
		}
		imageLen = buf.Len()
		if err = binary.Write(w, s.enc, uint32(imageLen+8)); err != nil {
			return 0, nil, err
		}
		if _, err = buf.WriteTo(w); err != nil {
			return 0, nil, err
		}
		if err = binary.Write(w, s.enc, uint32(0)); err != nil {
			return 0, nil, err
		}
	}

	if ifd != nil {
		ifd.TileWidth = uint16(d.X)
		ifd.TileLength = uint16(d.Y)
		ifd.BitsPerSample = s.bitsPerSample
		ifd.Compression = uint16(s.ctype)
		ifd.PhotometricInterpretation = uint16(s.photometricInterpretation)
		ifd.SamplesPerPixel = uint16(s.samplesPerPixel)

		if len(s.colorMap) != 0 {
			ifd.Colormap = s.colorMap
		}
		if s.extraSamples > 0 {
			ifd.ExtraSamples = []uint16{s.extraSamples}
		}
		if s.samplesPerPixel > 1 {
			ifd.PlanarConfiguration = 1
		}
	}

	return uint32(imageLen), ifd, nil
}

type TiffSource struct {
	RawSource
	ifd *IFD
}

func NewTiffSource(ifd *IFD, enc binary.ByteOrder) *TiffSource {
	m := &Reader{ifds: []*IFD{ifd}}
	d, r, _ := m.readData(0)
	return &TiffSource{ifd: ifd, RawSource: RawSource{dataOrImage: d, rect: r, ctype: CompressionType(ifd.Compression), enc: enc}}
}

func (s *TiffSource) Bounds() image.Rectangle {
	return image.Rect(0, 0, int(s.ifd.TileWidth), int(s.ifd.TileLength))
}
