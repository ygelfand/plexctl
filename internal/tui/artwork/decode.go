package artwork

import (
	"bytes"
	"fmt"
	"image"

	_ "image/jpeg"
	_ "image/png"
)

// Decode checks dimensions before a decoder allocates the image's pixel buffer.
// Poster responses come from remote servers and may be tiny compressed images
// whose decoded dimensions would otherwise exhaust memory.
func Decode(data []byte) (image.Image, error) {
	info, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	const maxPixels = 16 << 20
	if info.Width <= 0 || info.Height <= 0 || info.Width > 8192 || info.Height > 8192 ||
		info.Width > maxPixels/info.Height {
		return nil, fmt.Errorf("poster dimensions exceed the 16 megapixel limit")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}
