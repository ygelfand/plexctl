package artwork

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"strings"
	"testing"
)

func TestDecodeRejectsOversizedImageBeforeDecodingPixels(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name          string
		width, height uint32
	}{
		{"huge width", 100000, 1},
		{"huge height", 1, 100000},
		{"too many pixels", 8192, 8192},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// A complete PNG header is enough to trigger the dimension check;
			// no pixel data or large allocations are needed for this regression.
			data := append([]byte{}, []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x0dIHDR")...)
			data = binary.BigEndian.AppendUint32(data, test.width)
			data = binary.BigEndian.AppendUint32(data, test.height)
			data = append(data, 8, 2, 0, 0, 0)
			data = binary.BigEndian.AppendUint32(data, crc32.ChecksumIEEE(data[12:]))
			if _, err := Decode(data); err == nil || !strings.Contains(err.Error(), "limit") {
				t.Fatalf("expected dimension limit rejection, got %v", err)
			}
		})
	}
}

func TestDecodeSmallPoster(t *testing.T) {
	t.Parallel()
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatal(err)
	}
	img, err := Decode(encoded.Bytes())
	if err != nil || img.Bounds() != image.Rect(0, 0, 2, 3) {
		t.Fatalf("decode small poster: %v", err)
	}
}
