// this file exposes stable render helpers to other packages.
package render

import "dawnfetch/internal/dawnfetch/core"

func PreparePaletteForTerminal(palette []string, noColor bool) ([]string, bool) {
	if noColor {
		return palette, true
	}
	level := terminalColorLevel()
	if level == colorLevelNone {
		return palette, true
	}
	return normalizePaletteForColorLevel(palette, level), false
}

func RenderInfoLines(fields []core.Field, style core.StyleConfig, labelWidth int, valueWidth int, palette []string, noColor bool) []core.RenderedLine {
	return renderInfoLines(fields, style, labelWidth, valueWidth, palette, noColor)
}

func PaletteSwatchLines(noColor bool, fieldWidth int, style core.StyleConfig) []string {
	return paletteSwatchLines(noColor, fieldWidth, style)
}

func LabelWidth(fields []core.Field) int {
	return labelWidth(fields)
}

func ColorLine(code string, noColor bool, s string) string {
	return colorLine(code, noColor, s)
}

func DisplayWidth(s string) int {
	return displayWidth(s)
}

func StripANSI(s string) string {
	return stripANSI(s)
}

func TerminalWidth() int {
	return terminalWidth()
}

func TruncateRunes(s string, max int) string {
	return truncateRunes(s, max)
}

func PadRightStyled(styled string, raw string, width int) string {
	return padRightStyled(styled, raw, width)
}

func ChooseLayout(logoSet core.LogoSet, style core.StyleConfig, terminalW, labelWidth int) (bool, string, int, int) {
	return chooseLayout(logoSet, style, terminalW, labelWidth)
}

func RenderLogoLines(logoSet core.LogoSet, size string, palette []string, noColor bool) ([]core.RenderedLine, int) {
	return renderLogoLines(logoSet, size, palette, noColor)
}

func RenderUserHostLines(style core.StyleConfig, palette []string, noColor bool) []core.RenderedLine {
	return renderUserHostLines(style, palette, noColor)
}

func RenderedBlockWidth(blocks ...[]core.RenderedLine) int {
	return renderedBlockWidth(blocks...)
}

func SideBySideSafetyMargin() int {
	return sideBySideSafetyMargin()
}

func AlignSideBlocks(left []core.RenderedLine, right []core.RenderedLine, align string) ([]core.RenderedLine, []core.RenderedLine) {
	return alignSideBlocks(left, right, align)
}

func MaxInt(a, b int) int {
	return maxInt(a, b)
}
