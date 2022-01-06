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
}

type TiffSource struct {
	ifd *IFD
}

type TiledRawSource struct {
	dataOrImage               interface{} // []float64 | image.Image
	rect                      image.Rectangle
	ctype                     CompressionType
	predictor                 bool
	photometricInterpretation uint32
	samplesPerPixel           uint32
	bitsPerSample             []uint32
	extraSamples              uint32
	colorMap                  []uint32
}

func (p *TiledRawSource) Bounds() image.Rectangle {
	return p.rect
}

func (s *TiledRawSource) Encode(w io.Writer, enc binary.ByteOrder) error {
	d := s.Bounds().Size()

	compression := s.ctype
	predictor := s.predictor && compression == CTLZW

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
		default:
			imageLen = d.X * d.Y * 4
		}
		err := binary.Write(w, enc, uint32(imageLen+8))
		if err != nil {
			return err
		}
	case CTDeflate:
		dst = zlib.NewWriter(&buf)
	case CTLZW:
		dst = lzw.NewWriter(&buf, true)
	}

	s.photometricInterpretation = uint32(PI_RGB)
	s.samplesPerPixel = uint32(4)
	s.bitsPerSample = []uint32{8, 8, 8, 8}
	s.extraSamples = uint32(0)
	s.colorMap = []uint32{}

	var err error
	switch m := s.dataOrImage.(type) {
	case *image.Paletted:
		s.photometricInterpretation = PI_Paletted
		s.samplesPerPixel = 1
		s.bitsPerSample = []uint32{8}
		s.colorMap = make([]uint32, 256*3)
		for i := 0; i < 256 && i < len(m.Palette); i++ {
			r, g, b, _ := m.Palette[i].RGBA()
			s.colorMap[i+0*256] = uint32(r)
			s.colorMap[i+1*256] = uint32(g)
			s.colorMap[i+2*256] = uint32(b)
		}
		err = encodeGray(dst, m.Pix, d.X, d.Y, m.Stride, predictor)
	case *image.Gray:
		s.photometricInterpretation = PI_BlackIsZero
		s.samplesPerPixel = 1
		s.bitsPerSample = []uint32{8}
		err = encodeGray(dst, m.Pix, d.X, d.Y, m.Stride, predictor)
	case *image.Gray16:
		s.photometricInterpretation = PI_BlackIsZero
		s.samplesPerPixel = 1
		s.bitsPerSample = []uint32{16}
		err = encodeGray16(dst, m.Pix, d.X, d.Y, m.Stride, predictor)
	case *image.NRGBA:
		s.extraSamples = 2
		err = encodeRGBA(dst, m.Pix, d.X, d.Y, m.Stride, predictor)
	case *image.NRGBA64:
		s.extraSamples = 2
		s.bitsPerSample = []uint32{16, 16, 16, 16}
		err = encodeRGBA64(dst, m.Pix, d.X, d.Y, m.Stride, predictor)
	case *image.RGBA:
		s.extraSamples = 1
		err = encodeRGBA(dst, m.Pix, d.X, d.Y, m.Stride, predictor)
	case *image.RGBA64:
		s.extraSamples = 1
		s.bitsPerSample = []uint32{16, 16, 16, 16}
		err = encodeRGBA64(dst, m.Pix, d.X, d.Y, m.Stride, predictor)
	default:
		s.extraSamples = 1
		err = encode(dst, m.(image.Image), predictor)
	}
	if err != nil {
		return err
	}

	if compression != CTNone {
		if err = dst.(io.Closer).Close(); err != nil {
			return err
		}
		imageLen = buf.Len()
		if err = binary.Write(w, enc, uint32(imageLen+8)); err != nil {
			return err
		}
		if _, err = buf.WriteTo(w); err != nil {
			return err
		}
	}

	return nil
}
