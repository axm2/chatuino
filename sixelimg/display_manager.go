// Package sixelimg provides sixel graphics support for terminal image display.
package sixelimg

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	imgdraw "image/draw"
	"image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"math"
	"path/filepath"
	"sync/atomic"
	"time"

	_ "golang.org/x/image/webp"

	"github.com/adrg/xdg"
	"github.com/gen2brain/avif"
	awebp "github.com/gen2brain/webp"
	"github.com/julez-dev/chatuino/kittyimg"
	"github.com/mattn/go-sixel"
	"github.com/rs/zerolog/log"
	"github.com/spf13/afero"
	"golang.org/x/image/draw"
	"golang.org/x/sync/syncmap"
)

var (
	// BaseImageDirectory is the base directory for cached images.
	BaseImageDirectory = filepath.Join(xdg.DataHome, "chatuino")
)

// ErrUnsupportedAnimatedFormat is returned when an animated image format is not supported.
var ErrUnsupportedAnimatedFormat = errors.New("emote is animated but in non supported format")

var (
	globalImagePlacementIDCounter atomic.Int32 = atomic.Int32{}
	globalPlacedImages                         = &syncmap.Map{}
)

// DecodedImage represents a cached decoded image with its sixel data.
type DecodedImage struct {
	ID        int32  `json:"-"`
	Cols      int    `json:"cols"`
	SixelData string `json:"sixel_data"` // Cached sixel escape sequence

	lastUsed time.Time `json:"-"`
}

// DisplayManager handles sixel image conversion and caching.
type DisplayManager struct {
	fs                    afero.Fs
	cellWidth, cellHeight float32
}

// NewDisplayManager creates a new sixel DisplayManager.
func NewDisplayManager(fs afero.Fs, cellWidth, cellHeight float32) *DisplayManager {
	return &DisplayManager{
		fs:         fs,
		cellWidth:  cellWidth,
		cellHeight: cellHeight,
	}
}

// Convert converts a kittyimg.DisplayUnit to a kittyimg.KittyDisplayUnit using sixel encoding.
func (d *DisplayManager) Convert(unit kittyimg.DisplayUnit) (kittyimg.KittyDisplayUnit, error) {
	// 1st: image was already placed in this session, reusing cached sixel
	if cached, ok := globalPlacedImages.Load(unit.ID); ok {
		i, ok := cached.(DecodedImage)
		if !ok {
			log.Logger.Error().Str("id", unit.ID).Type("type", cached).Msg("unexpected type in session cache")
			globalPlacedImages.Delete(unit.ID)
		} else {
			i.lastUsed = time.Now()
			globalPlacedImages.Swap(unit.ID, i)

			return kittyimg.KittyDisplayUnit{
				ReplacementText: i.SixelData,
			}, nil
		}
	}

	// 2nd: image was not placed in session yet, but is already cached on FS
	incrementID := globalImagePlacementIDCounter.Add(1)

	cachedDecoded, found, err := d.openCached(unit)
	if err != nil {
		log.Logger.Warn().Err(err).Str("id", unit.ID).Msg("failed to open cached image, will re-download")
	}

	if found {
		cachedDecoded.ID = incrementID
		cachedDecoded.lastUsed = time.Now()

		globalPlacedImages.Store(unit.ID, cachedDecoded)
		return kittyimg.KittyDisplayUnit{
			ReplacementText: cachedDecoded.SixelData,
		}, nil
	}

	// 3rd: image was not downloaded yet, download and convert and save
	imageBody, contentType, err := unit.Load()
	if err != nil {
		return kittyimg.KittyDisplayUnit{}, err
	}

	log.Logger.Info().Str("id", unit.ID).Str("type", contentType).Msg("downloaded image for sixel")

	defer imageBody.Close()

	decoded, err := d.convertImageBytes(imageBody, unit, contentType)
	if err != nil {
		log.Logger.Err(err).Any("unit", unit).Send()
		return kittyimg.KittyDisplayUnit{}, err
	}

	decoded.ID = incrementID
	decoded.lastUsed = time.Now()
	globalPlacedImages.Store(unit.ID, decoded)
	if err := d.cacheDecodedImage(decoded, unit); err != nil {
		log.Logger.Warn().Err(err).Str("id", unit.ID).Msg("failed to cache decoded image")
	}

	return kittyimg.KittyDisplayUnit{
		ReplacementText: decoded.SixelData,
	}, nil
}

// CleanupOldImagesCommand returns an empty string for sixel (no cleanup needed).
func (d *DisplayManager) CleanupOldImagesCommand(maxAge time.Duration) string {
	// Sixel images don't persist in terminal memory like Kitty images
	// Just clean up our session cache
	globalPlacedImages.Range(func(key, value any) bool {
		c, ok := value.(DecodedImage)
		if !ok {
			globalPlacedImages.Delete(key)
			return true
		}
		if time.Since(c.lastUsed) > maxAge {
			globalPlacedImages.Delete(key)
		}
		return true
	})
	return ""
}

// CleanupAllImagesCommand returns an empty string for sixel (no cleanup needed).
func (d *DisplayManager) CleanupAllImagesCommand() string {
	return ""
}

func (d *DisplayManager) convertImageBytes(r io.Reader, unit kittyimg.DisplayUnit, contentType string) (DecodedImage, error) {
	// For sixel, we only support the first frame of animated images
	// since sixel doesn't have native animation support in most terminals

	if contentType == "image/avif" {
		images, err := avif.DecodeAll(r)
		if err != nil {
			return DecodedImage{}, fmt.Errorf("failed to convert avif: %w", err)
		}
		if len(images.Image) > 0 {
			return d.convertSingleImage(images.Image[0], unit)
		}
		return DecodedImage{}, fmt.Errorf("avif has no frames")
	}

	if unit.IsAnimated && contentType == "image/webp" {
		images, err := awebp.DecodeAll(r)
		if err != nil {
			return DecodedImage{}, fmt.Errorf("failed to convert animated webp: %w", err)
		}
		if len(images.Image) > 0 {
			return d.convertSingleImage(images.Image[0], unit)
		}
		return DecodedImage{}, fmt.Errorf("webp has no frames")
	}

	if unit.IsAnimated && contentType == "image/gif" {
		images, err := gif.DecodeAll(r)
		if err != nil {
			return DecodedImage{}, fmt.Errorf("failed to convert animated gif: %w", err)
		}
		if len(images.Image) > 0 {
			// Composite first frame onto canvas
			width, height := images.Config.Width, images.Config.Height
			if width == 0 || height == 0 {
				width = images.Image[0].Bounds().Dx()
				height = images.Image[0].Bounds().Dy()
			}
			canvas := image.NewRGBA(image.Rect(0, 0, width, height))
			imgdraw.Draw(canvas, images.Image[0].Bounds(), images.Image[0], images.Image[0].Bounds().Min, imgdraw.Over)
			return d.convertSingleImage(canvas, unit)
		}
		return DecodedImage{}, fmt.Errorf("gif has no frames")
	}

	if unit.IsAnimated {
		// For unsupported animated formats, we'll try to decode as static
		log.Logger.Warn().Str("content-type", contentType).Msg("animated format not fully supported for sixel, using first frame")
	}

	img, format, err := image.Decode(r)
	if err != nil {
		log.Logger.Error().Err(err).Str("format", format).Send()
		return DecodedImage{}, fmt.Errorf("failed to convert %s: %w", format, err)
	}

	return d.convertSingleImage(img, unit)
}

func (d *DisplayManager) convertSingleImage(img image.Image, unit kittyimg.DisplayUnit) (DecodedImage, error) {
	bounds := img.Bounds()
	height := bounds.Dy()
	width := bounds.Dx()

	// Apply right padding if specified
	if unit.RightPadding > 0 {
		img = addRightPadding(img, unit.RightPadding)
		bounds = img.Bounds()
		width = bounds.Dx()
	}

	// Scale image to cell height
	ratio := d.cellHeight / float32(height)
	newWidth := int(math.Round(float64(float32(width) * ratio)))
	newHeight := int(d.cellHeight)

	// Resize image
	resized := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	draw.CatmullRom.Scale(resized, resized.Bounds(), img, bounds, draw.Over, nil)

	// Calculate columns
	cols := int(math.Ceil(float64(float32(newWidth) / d.cellWidth)))

	// Convert to sixel
	sixelData, err := imageToSixel(resized)
	if err != nil {
		return DecodedImage{}, fmt.Errorf("failed to encode sixel: %w", err)
	}

	return DecodedImage{
		Cols:      cols,
		SixelData: sixelData,
	}, nil
}

func (d *DisplayManager) cacheDecodedImage(decoded DecodedImage, unit kittyimg.DisplayUnit) error {
	cacheDir, err := d.createGetCacheDirectory(unit.Directory)
	if err != nil {
		return err
	}

	metaImageFilePath := filepath.Join(cacheDir, fmt.Sprintf("%s.sixel.json", filepath.Clean(unit.ID)))

	f, err := d.fs.Create(metaImageFilePath)
	if err != nil {
		return err
	}
	defer f.Close()

	// Use zlib compression for the sixel data
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	if _, err := w.Write([]byte(decoded.SixelData)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}

	// Store compressed data path
	compressedPath := filepath.Join(cacheDir, fmt.Sprintf("%s.sixel.zlib", filepath.Clean(unit.ID)))
	compressedFile, err := d.fs.Create(compressedPath)
	if err != nil {
		return err
	}
	defer compressedFile.Close()

	if _, err := compressedFile.Write(buf.Bytes()); err != nil {
		return err
	}

	// Store metadata
	meta := struct {
		Cols        int    `json:"cols"`
		EncodedPath string `json:"encoded_path"`
	}{
		Cols:        decoded.Cols,
		EncodedPath: compressedPath,
	}

	return json.NewEncoder(f).Encode(meta)
}

func (d *DisplayManager) openCached(unit kittyimg.DisplayUnit) (DecodedImage, bool, error) {
	dir, err := d.createGetCacheDirectory(unit.Directory)
	if err != nil {
		return DecodedImage{}, false, err
	}

	metaImageFilePath := filepath.Join(dir, fmt.Sprintf("%s.sixel.json", filepath.Clean(unit.ID)))

	data, err := afero.ReadFile(d.fs, metaImageFilePath)
	if err != nil {
		if errors.Is(err, afero.ErrFileNotFound) {
			return DecodedImage{}, false, nil
		}
		return DecodedImage{}, false, err
	}

	var meta struct {
		Cols        int    `json:"cols"`
		EncodedPath string `json:"encoded_path"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return DecodedImage{}, false, err
	}

	// Read compressed sixel data
	compressedData, err := afero.ReadFile(d.fs, meta.EncodedPath)
	if err != nil {
		return DecodedImage{}, false, err
	}

	// Decompress
	r, err := zlib.NewReader(bytes.NewReader(compressedData))
	if err != nil {
		return DecodedImage{}, false, err
	}
	defer r.Close()

	sixelData, err := io.ReadAll(r)
	if err != nil {
		return DecodedImage{}, false, err
	}

	return DecodedImage{
		Cols:      meta.Cols,
		SixelData: string(sixelData),
	}, true, nil
}

func (d *DisplayManager) createGetCacheDirectory(dir string) (string, error) {
	path := filepath.Join(BaseImageDirectory, "sixel", dir)

	if err := d.fs.MkdirAll(path, 0o755); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return path, nil
		}
		return "", err
	}

	return path, nil
}

// addRightPadding creates a new image with transparent padding on the right side.
func addRightPadding(img image.Image, padding int) image.Image {
	bounds := img.Bounds()
	newWidth := bounds.Dx() + padding
	newHeight := bounds.Dy()

	padded := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	imgdraw.Draw(padded, bounds, img, bounds.Min, imgdraw.Src)

	return padded
}

// imageToSixel converts an image to a sixel escape sequence string.
func imageToSixel(img image.Image) (string, error) {
	var buf bytes.Buffer
	enc := sixel.NewEncoder(&buf)
	enc.Dither = true // Enable dithering for better quality

	if err := enc.Encode(img); err != nil {
		return "", err
	}

	return buf.String(), nil
}
