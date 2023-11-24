package kikcode

import (
	"errors"
	"fmt"
	"image/color"
	"math"
)

const (
	maxKikCodePayloadDataLength = 40
)

var (
	ErrEmptyData   = errors.New("payload data is empty")
	ErrDataTooLong = errors.New("payload data is too long")
	ErrInvalidSize = errors.New("invalid size")
)

const (
	kikCodeInnerRingRatio = 0.32
	kikCodeFirstRingRatio = 0.425
	kikCodeLastRingRatio  = 0.95
	kikCodeScaleFactor    = 8.0
	kikCodeRingCount      = 6
)

type coordinate struct {
	X float64
	Y float64
}

// todo: Use proper arc struct
type Description struct {
	dimension        float64
	centerPathString string
	dotPathStrings   []string
	arcPathStrings   []string
	dotDimension     float64
}

type KikCodePayload []byte

func GenerateDescription(dimension float64, data KikCodePayload) (*Description, error) {
	if dimension <= 0 {
		return nil, ErrInvalidSize
	}

	if len(data) == 0 {
		return nil, ErrEmptyData
	} else if len(data) >= maxKikCodePayloadDataLength {
		return nil, ErrDataTooLong
	}

	center := coordinate{
		X: 0.5 * dimension,
		Y: 0.5 * dimension,
	}

	outerRingWidth := dimension * 0.5
	innerRingWidth := kikCodeInnerRingRatio * outerRingWidth
	firstRingWidth := kikCodeFirstRingRatio * outerRingWidth
	lastRingWidth := kikCodeLastRingRatio * outerRingWidth

	centerPathString := fmt.Sprintf(
		"M%[1]f,%[2]f m-%[3]f,0 a%[3]f,%[3]f 0 1,0 %[4]f,0 a%[3]f,%[3]f 0 1,0 -%[4]f,0",
		center.X,
		center.Y,
		innerRingWidth,
		2*innerRingWidth,
	)

	offset := 0

	ringWidth := (lastRingWidth - firstRingWidth) / kikCodeRingCount
	dotSize := (ringWidth * 3.0) / 4.0

	var dotPathStrings, arcPathStrings []string

	for ring := 0; ring < kikCodeRingCount; ring++ {
		r := ringWidth*float64(ring) + firstRingWidth
		if ring == 0 {
			r -= innerRingWidth / 10.0
		}

		n := kikCodeScaleFactor*ring + 32
		delta := (math.Pi * 2.0) / float64(n)

		startOffset := offset

		for a := 0; a < n; a++ {
			angle := float64(a)*delta - math.Pi/2.0

			bitMask := 0x1 << (offset % 8)
			byteIndex := int(math.Floor(float64(offset) / 8.0))
			currentBit := byteIndex < len(data) && (int(data[byteIndex])&bitMask) != 0

			if currentBit {
				radius := r + ringWidth/2
				arcCenter := coordinate{
					X: center.X + radius*math.Cos(angle),
					Y: center.Y + radius*math.Sin(angle),
				}

				nextOffset := ((offset - startOffset + 1) % n) + startOffset
				nextBitMask := 0x1 << (nextOffset % 8)
				nextIndex := int(math.Floor(float64(nextOffset) / 8.0))
				nextBit := nextIndex < len(data) && (int(data[nextIndex])&nextBitMask) != 0

				if nextBit {
					arcPathString := fmt.Sprintf(
						"M%[1]f,%[2]f A%[3]f,%[3]f 0 0,1 %[4]f,%[5]f",
						center.X+radius*math.Cos(angle),
						center.Y+radius*math.Sin(angle),
						radius,
						center.X+radius*math.Cos(angle+delta),
						center.Y+radius*math.Sin(angle+delta),
					)
					arcPathStrings = append(arcPathStrings, arcPathString)
				} else {
					dotPathString := fmt.Sprintf(
						"M%[1]f,%[2]f m-%[3]f,0 a%[3]f,%[3]f 0 1,0 %[4]f,0 a%[3]f,%[3]f 0 1,0 -%[4]f,0",
						arcCenter.X,
						arcCenter.Y,
						0.5*dotSize,
						dotSize,
					)
					dotPathStrings = append(dotPathStrings, dotPathString)
				}
			}

			offset += 1
		}
	}

	return &Description{
		dimension:        dimension,
		centerPathString: centerPathString,
		dotPathStrings:   dotPathStrings,
		arcPathStrings:   arcPathStrings,
		dotDimension:     dotSize,
	}, nil
}

func CreateKikCodePayload(data []byte) KikCodePayload {
	finderBytes := []byte{0xb2, 0xcb, 0x25, 0xc6}
	return append(finderBytes, data...)
}

type QrCodeRenderOptions struct {
	ForegroundColor color.Color

	IncludeBackground bool
	BackgroundColor   color.Color
}

func (d *Description) ToSvg(opts *QrCodeRenderOptions) string {
	foregroundColorHex := hexColor(opts.ForegroundColor)
	backgroundColorHex := hexColor(opts.BackgroundColor)

	svg := fmt.Sprintf(`<svg width="%[1]f" height="%[1]f" viewBox="0 0 %[1]f %[1]f">`, d.dimension)
	if opts.IncludeBackground {
		svg += fmt.Sprintf(`<circle cx="%[1]f" cy="%[1]f" r="%[1]f" fill="%[2]s"/>`, d.dimension/2, backgroundColorHex)
	}
	svg += fmt.Sprintf(`<path d="%s" fill="%s"/>`, d.centerPathString, foregroundColorHex)
	for _, arc := range d.arcPathStrings {
		svg += fmt.Sprintf(`<path d="%s" stroke="%s" stroke-linecap="round" stroke-width="11.5"/>`, arc, foregroundColorHex)
	}
	for _, dot := range d.dotPathStrings {
		svg += fmt.Sprintf(`<path d="%s" fill="%s"/>`, dot, foregroundColorHex)
	}
	svg += `</svg>`
	return svg
}

func hexColor(c color.Color) string {
	rgba := color.RGBAModel.Convert(c).(color.RGBA)
	return fmt.Sprintf("#%.2x%.2x%.2x", rgba.R, rgba.G, rgba.B)
}
