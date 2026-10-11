package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// DisplayInlineImage returns a string that renders the image in the terminal.
// Priority: Kitty → Sixel (img2sixel) → chafa → half-block fallback
func DisplayInlineImage(imgBytes []byte, termWidth int) string {
	var rawImage string

	if os.Getenv("KITTY_WINDOW_ID") != "" || os.Getenv("TERM") == "xterm-kitty" {
		rawImage = renderKitty(imgBytes)
	} else if _, err := exec.LookPath("img2sixel"); err == nil {
		rawImage = renderSixel(imgBytes, termWidth)
	} else if _, err := exec.LookPath("chafa"); err == nil {
		rawImage = renderChafa(imgBytes, termWidth)
	} else {
		rawImage = renderHalfBlockMilitary(imgBytes, termWidth)
	}

	return wrapInCyberFrame(rawImage, " NEURAL ARTIFACT RECONSTRUCTION ")
}

// renderSixel uses img2sixel for true-color pixel-accurate terminal rendering.
func renderSixel(imgBytes []byte, termWidth int) string {
	tmp, err := os.CreateTemp("", "nexus-sixel-*.png")
	if err != nil {
		return renderHalfBlockMilitary(imgBytes, termWidth)
	}
	defer os.Remove(tmp.Name())
	tmp.Write(imgBytes)
	tmp.Close()

	cmd := exec.Command("img2sixel",
		"--width", "512",
		"--height", "512",
		tmp.Name(),
	)
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return renderChafaOrBlock(imgBytes, termWidth)
	}
	return strings.TrimRight(string(out), "\n")
}

// renderChafaOrBlock tries chafa then falls back to half-block.
func renderChafaOrBlock(imgBytes []byte, termWidth int) string {
	if _, err := exec.LookPath("chafa"); err == nil {
		return renderChafa(imgBytes, termWidth)
	}
	return renderHalfBlockMilitary(imgBytes, termWidth)
}

func renderKitty(imgBytes []byte) string {
	encoded := base64.StdEncoding.EncodeToString(imgBytes)
	return fmt.Sprintf("\x1b_Ga=T,f=100,m=0;%s\x1b\\", encoded)
}

// renderChafa uses the chafa CLI to render the image with braille+color symbols.
// This gives 4-8x the effective resolution of half-block with proper color dithering.
// chafa auto-selects the best symbol set for the current terminal environment.
func renderChafa(imgBytes []byte, termWidth int) string {
	// Write to a temp file — chafa reads from file path
	tmp, err := os.CreateTemp("", "nexus-chafa-*.png")
	if err != nil {
		return renderHalfBlockMilitary(imgBytes, termWidth)
	}
	defer os.Remove(tmp.Name())
	tmp.Write(imgBytes)
	tmp.Close()

	renderW := termWidth
	if renderW > 160 {
		renderW = 160
	}
	if renderW < 10 {
		renderW = 10
	}
	// ~75% of width for height — square images look correct at this ratio
	renderH := renderW * 3 / 4

	// Use symbols mode (works through tmux, pure ANSI)
	// braille+border gives the sharpest result inside tmux
	cmd := exec.Command("chafa",
		"--format=symbols",
		"--symbols=braille+border",
		"--colors=full",
		"--color-extractor=average",
		"--dither=ordered",
		fmt.Sprintf("--size=%dx%d", renderW, renderH),
		tmp.Name(),
	)

	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		// Fallback: try with default symbols (half-block)
		cmd2 := exec.Command("chafa",
			"--format=symbols",
			fmt.Sprintf("--size=%dx%d", renderW, renderH),
			tmp.Name(),
		)
		out, err = cmd2.Output()
		if err != nil || len(out) == 0 {
			return renderHalfBlockMilitary(imgBytes, termWidth)
		}
	}

	return strings.TrimRight(string(out), "\n")
}

// renderHalfBlockMilitary uses 2 colors per cell (fg/bg) + Bilinear interpolation for elite color fidelity.
// Used as fallback when chafa is not available.
func renderHalfBlockMilitary(imgBytes []byte, termWidth int) string {
	srcImg, _, err := image.Decode(bytes.NewReader(imgBytes))
	if err != nil {
		return fmt.Sprintf("[Image decode error: %v]", err)
	}

	bounds := srcImg.Bounds()
	imgW, imgH := bounds.Dx(), bounds.Dy()

	if termWidth <= 0 {
		termWidth = 60
	}
	renderW := termWidth
	if renderW > 160 {
		renderW = 160
	}
	if renderW < 10 {
		renderW = 10
	}

	renderH := int(float64(renderW) * float64(imgH) / float64(imgW) * 0.5)
	if renderH%2 != 0 {
		renderH++
	}

	// Bilinear Sampler for smoother color transitions
	sample := func(x, y float64) (uint8, uint8, uint8) {
		srcX := x * float64(imgW-1) / float64(renderW)
		srcY := y * float64(imgH-1) / float64(renderH*2) // *2 because half-block splits the cell

		x1, y1 := int(srcX), int(srcY)
		x2, y2 := x1+1, y1+1
		if x2 >= imgW {
			x2 = imgW - 1
		}
		if y2 >= imgH {
			y2 = imgH - 1
		}

		dx, dy := srcX-float64(x1), srcY-float64(y1)

		c11r, c11g, c11b, _ := color.RGBAModel.Convert(srcImg.At(bounds.Min.X+x1, bounds.Min.Y+y1)).RGBA()
		c12r, c12g, c12b, _ := color.RGBAModel.Convert(srcImg.At(bounds.Min.X+x1, bounds.Min.Y+y2)).RGBA()
		c21r, c21g, c21b, _ := color.RGBAModel.Convert(srcImg.At(bounds.Min.X+x2, bounds.Min.Y+y1)).RGBA()
		c22r, c22g, c22b, _ := color.RGBAModel.Convert(srcImg.At(bounds.Min.X+x2, bounds.Min.Y+y2)).RGBA()

		tr := float64(c11r>>8)*(1-dx)*(1-dy) + float64(c21r>>8)*dx*(1-dy) + float64(c12r>>8)*(1-dx)*dy + float64(c22r>>8)*dx*dy
		tg := float64(c11g>>8)*(1-dx)*(1-dy) + float64(c21g>>8)*dx*(1-dy) + float64(c12g>>8)*(1-dx)*dy + float64(c22g>>8)*dx*dy
		tb := float64(c11b>>8)*(1-dx)*(1-dy) + float64(c21b>>8)*dx*(1-dy) + float64(c12b>>8)*(1-dx)*dy + float64(c22b>>8)*dx*dy

		return uint8(tr), uint8(tg), uint8(tb)
	}

	var sb strings.Builder
	for y := 0; y < renderH*2; y += 2 {
		for x := 0; x < renderW; x++ {
			r1, g1, b1 := sample(float64(x), float64(y))
			r2, g2, b2 := sample(float64(x), float64(y+1))

			// ▀  upper half block: fg = top pixel color, bg = bottom pixel color
			sb.WriteString(fmt.Sprintf("\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀", r1, g1, b1, r2, g2, b2))
		}
		sb.WriteString("\x1b[0m\n")
	}

	return sb.String()
}

func wrapInCyberFrame(content, label string) string {
	frameStyle := lipgloss.NewStyle().
		Border(lipgloss.ThickBorder()).
		BorderForeground(ActiveTheme.Primary).
		Padding(0, 1)

	title := lipgloss.NewStyle().
		Foreground(ActiveTheme.Background).
		Background(ActiveTheme.Primary).
		Bold(true).
		Render(" " + label + " ")

	return lipgloss.JoinVertical(lipgloss.Center, title, frameStyle.Render(content))
}
