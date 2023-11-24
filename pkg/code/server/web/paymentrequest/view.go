package paymentrequest

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"strings"

	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	"github.com/pkg/errors"
	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
	"github.com/srwiley/scanFT"
	"golang.org/x/image/font"

	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/kikcode"
)

func (s *Server) drawRequestCard(paymentRequest *trustedPaymentRequest) (*image.RGBA, error) {
	//
	// Part 1: Setup individual layers
	//

	//
	// Part 1.1: Background layer
	//

	backgroundLayer := s.assets.RequestCardBackgroundLayer
	backgroundCenterX := float64(backgroundLayer.Bounds().Max.X) / 2
	backgroundCenterY := float64(backgroundLayer.Bounds().Max.Y) / 2

	//
	// Part 1.2: QR code layer
	//

	qrCodeDimension := 350.0
	qrCodeStartX := backgroundCenterX - qrCodeDimension/2
	qrCodeStartY := 0.9*backgroundCenterY - qrCodeDimension/2

	qrCodeRenderOptions := &kikcode.QrCodeRenderOptions{
		ForegroundColor: color.White,

		IncludeBackground: true,
		BackgroundColor: color.RGBA{
			R: 86,
			G: 92,
			B: 134,
			A: 255,
		},
	}

	qrCodeLayer := image.NewRGBA(backgroundLayer.Bounds())
	qrCodeDescription, err := paymentRequest.ToKikCodePayload().ToQrCodeDescription(qrCodeDimension)
	if err != nil {
		return nil, errors.Wrap(err, "error generating qr code description")
	}
	qrCodeIcon, err := oksvg.ReadIconStream(
		strings.NewReader(qrCodeDescription.ToSvg(qrCodeRenderOptions)),
		oksvg.StrictErrorMode,
	)
	if err != nil {
		return nil, errors.Wrap(err, "invalid qr code svg")
	}
	qrCodeIcon.SetTarget(qrCodeStartX, qrCodeStartY, qrCodeDimension, qrCodeDimension)
	qrCodeIcon.Draw(
		rasterx.NewDasher(
			backgroundLayer.Bounds().Max.X,
			backgroundLayer.Bounds().Max.Y,
			scanFT.NewScannerFT(
				backgroundLayer.Bounds().Max.X,
				backgroundLayer.Bounds().Max.Y,
				scanFT.NewRGBAPainter(qrCodeLayer),
			),
		),
		1,
	)

	//
	// Part 1.3: Code logo layer
	//

	codeLogoDimension := 72.0
	codeLogoStartX := backgroundCenterX - codeLogoDimension/2
	codeLogoStartY := 0.9*backgroundCenterY - codeLogoDimension/2

	codeLogoIcon, err := oksvg.ReadIconStream(
		strings.NewReader(getCodeLogoSvg(qrCodeRenderOptions.BackgroundColor)),
		oksvg.StrictErrorMode,
	)
	if err != nil {
		return nil, errors.Wrap(err, "invalid code logo svg")
	}
	codeLogoIcon.SetTarget(codeLogoStartX, codeLogoStartY, codeLogoDimension, codeLogoDimension)
	codeLogoLayer := image.NewRGBA(backgroundLayer.Bounds())
	codeLogoIcon.Draw(
		rasterx.NewDasher(
			backgroundLayer.Bounds().Max.X,
			backgroundLayer.Bounds().Max.Y,
			rasterx.NewScannerGV(
				backgroundLayer.Bounds().Max.X,
				backgroundLayer.Bounds().Max.Y,
				codeLogoLayer,
				backgroundLayer.Bounds(),
			),
		),
		1,
	)

	//
	// Part 1.4: Flag with amount text layer
	//

	// todo: Add localization
	// todo: Add flags
	// todo: Use proper currency symbols

	var amountText string
	amountTextFont := s.assets.RequestCardAmountTextFont

	switch paymentRequest.currency {
	case currency_lib.KIN:
		kinNativeAmount := int(paymentRequest.nativeAmount)
		if int(100.0*paymentRequest.nativeAmount)%100 != 0 {
			kinNativeAmount += 1
		}

		amountText = fmt.Sprintf("%d Kin", kinNativeAmount)
	default:
		amountText = fmt.Sprintf(
			"%s %.2f of Kin",
			strings.ToUpper(string(paymentRequest.currency)),
			paymentRequest.nativeAmount,
		)
	}

	fontFaceOpts := &truetype.Options{
		Size: 30,
	}
	fontFace := truetype.NewFace(amountTextFont, fontFaceOpts)

	textWidth := font.MeasureString(fontFace, amountText).Ceil()
	amountStartX := (backgroundLayer.Bounds().Max.X - textWidth) / 2
	amountStartY := 1.5 * backgroundCenterY

	amountLayer := image.NewRGBA(backgroundLayer.Bounds())
	ftCtx := freetype.NewContext()
	ftCtx.SetFont(amountTextFont)
	ftCtx.SetFontSize(fontFaceOpts.Size)
	ftCtx.SetClip(backgroundLayer.Bounds())
	ftCtx.SetSrc(image.Black)
	ftCtx.SetDst(amountLayer)
	_, err = ftCtx.DrawString(amountText, freetype.Pt(int(amountStartX), int(amountStartY)))
	if err != nil {
		return nil, err
	}

	//
	// Part 2: Combine layers into a single image
	//

	combined := image.NewRGBA(backgroundLayer.Bounds())
	draw.Draw(combined, backgroundLayer.Bounds(), backgroundLayer, image.Point{0, 0}, draw.Src)
	draw.Draw(combined, qrCodeLayer.Bounds(), qrCodeLayer, image.Point{0, 0}, draw.Over)
	draw.Draw(combined, codeLogoLayer.Bounds(), codeLogoLayer, image.Point{0, 0}, draw.Over)
	draw.Draw(combined, amountLayer.Bounds(), amountLayer, image.Point{0, 0}, draw.Over)
	return combined, nil
}

func getCodeLogoSvg(color color.Color) string {
	colorValue := hexColor(color)
	return fmt.Sprintf(`
	<svg width="26" height="27" viewBox="0 0 26 27" fill="none">
		<path d="M6.81623 10.5846C8.50271 10.5846 9.88001 9.22136 9.88001 7.52082C9.88001 5.82027 8.51677 4.45703 6.81623 4.45703C5.11568 4.45703 3.75244 5.82027 3.75244 7.52082C3.75244 9.22136 5.12974 10.5846 6.81623 10.5846Z" fill="%[1]s"/>
		<path d="M19.0574 10.5846C20.7439 10.5846 22.1212 9.22136 22.1212 7.52082C22.1212 5.82027 20.758 4.45703 19.0574 4.45703C17.371 4.45703 15.9937 5.82027 15.9937 7.52082C15.9937 9.22136 17.371 10.5846 19.0574 10.5846Z" fill="%[1]s"/>
		<path d="M19.0574 22.8268C20.7439 22.8268 22.1212 21.4635 22.1212 19.763C22.1212 18.0765 20.758 16.6992 19.0574 16.6992C17.371 16.6992 15.9937 18.0625 15.9937 19.763C16.0077 21.4495 17.371 22.8268 19.0574 22.8268Z" fill="%[1]s"/>
		<path d="M6.81623 22.8268C8.50271 22.8268 9.88001 21.4635 9.88001 19.763C9.88001 18.0765 8.51677 16.6992 6.81623 16.6992C5.11568 16.6992 3.75244 18.0625 3.75244 19.763C3.7665 21.4495 5.12974 22.8268 6.81623 22.8268Z" fill="%[1]s"/>
		<path d="M6.88654 16.7131C8.57302 16.7131 9.95032 15.3499 9.95032 13.6493C9.93627 11.9629 8.57302 10.5996 6.88654 10.5996C5.20005 10.5996 3.82275 11.9629 3.82275 13.6634C3.82275 15.3358 5.20005 16.7131 6.88654 16.7131Z" fill="%[1]s"/>
		<path d="M12.9439 10.6139C14.6304 10.6139 16.0077 9.25065 16.0077 7.55011C16.0077 5.86363 14.6445 4.48633 12.9439 4.48633C11.2574 4.48633 9.88013 5.84957 9.88013 7.55011C9.88013 9.25065 11.2574 10.6139 12.9439 10.6139Z" fill="%[1]s"/>
		<path d="M12.9438 26.6215C13.9979 26.6215 14.8552 25.7642 14.8552 24.7102C14.8552 23.6561 13.9979 22.7988 12.9438 22.7988C11.8898 22.7988 11.0325 23.6561 11.0325 24.7102C11.0325 25.7642 11.8898 26.6215 12.9438 26.6215Z" fill="%[1]s"/>
		<path d="M12.9438 4.4438C13.9979 4.4438 14.8552 3.5865 14.8552 2.53245C14.8552 1.47839 13.9979 0.621094 12.9438 0.621094C11.8898 0.621094 11.0325 1.47839 11.0325 2.53245C11.0325 3.5865 11.8898 4.4438 12.9438 4.4438Z" fill="%[1]s"/>
		<path d="M1.91135 15.561C2.96541 15.561 3.8227 14.7037 3.8227 13.6496C3.8227 12.5956 2.96541 11.7383 1.91135 11.7383C0.857297 11.7383 0 12.5956 0 13.6496C0 14.7037 0.857297 15.561 1.91135 15.561Z" fill="%[1]s"/>
		<path d="M24.0324 15.561C25.0865 15.561 25.9438 14.7037 25.9438 13.6496C25.9438 12.5956 25.0865 11.7383 24.0324 11.7383C22.9784 11.7383 22.1211 12.5956 22.1211 13.6496C22.1211 14.7037 22.9643 15.561 24.0324 15.561Z" fill="%[1]s"/>
		<path d="M12.9439 22.7975C14.6304 22.7975 16.0077 21.4342 16.0077 19.7337C16.0077 18.0472 14.6445 16.6699 12.9439 16.6699C11.2574 16.6699 9.88013 18.0332 9.88013 19.7337C9.88013 21.4202 11.2574 22.7975 12.9439 22.7975Z" fill="%[1]s"/>
	</svg>
	`, colorValue)
}

func hexColor(c color.Color) string {
	rgba := color.RGBAModel.Convert(c).(color.RGBA)
	return fmt.Sprintf("#%.2x%.2x%.2x", rgba.R, rgba.G, rgba.B)
}
