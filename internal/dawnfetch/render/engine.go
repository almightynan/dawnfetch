// this file renders logos, fields, and color swatches to the terminal.
package render

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"dawnfetch/internal/dawnfetch/core"
	"dawnfetch/internal/dawnfetch/logo"
	"dawnfetch/internal/dawnfetch/platform"
)

const (
	stackCompactWidth = 72
	stackTinyWidth    = 50
	windowsWidthBoost = 8

	colorLevelNone = iota
	colorLevelANSI16
	colorLevelANSI256
	colorLevelTrueColor
)

var ansiStripRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func Print(fields []core.Field, cfg core.BrandConfig, style core.StyleConfig, themeName string, imagePath string, noColor bool, noLogo bool) {
	if os.Getenv("NO_COLOR") != "" {
		noColor = true
	}

	paletteName := core.ResolvePaletteName(themeName, cfg.Palettes)
	palette := cfg.Palettes[paletteName]
	if len(palette) == 0 {
		palette = cfg.Palettes[core.DefaultPalette]
	}
	if len(palette) == 0 {
		palette = []string{"37"}
	}
	palette, noColor = PreparePaletteForTerminal(palette, noColor)

	terminalW := terminalWidth()
	labelWidth := labelWidth(fields)
	logoSet := core.LogoSet{}
	sideBySide, logoSize, logoWidth, valueWidth := false, "normal", 0, 0
	logoLines := []core.RenderedLine(nil)
	logoEnabled := !noLogo && style.Layout.ShowLogo
	if logoEnabled {
		logoSet = logo.ResolveLogoSet(imagePath, cfg)
		sideBySide, logoSize, logoWidth, valueWidth = chooseLayout(logoSet, style, terminalW, labelWidth)
		logoLines, _ = renderLogoLines(logoSet, logoSize, palette, noColor)
		// when logo and fields are stacked, make sure chosen logo variant fits the available width.
		if !sideBySide && terminalW > 0 {
			maxLogoW := terminalW - style.Layout.LogoLeftPadding
			if maxLogoW < 8 {
				maxLogoW = 8
			}
			for _, size := range []string{"normal", "compact", "tiny"} {
				candidate, w := renderLogoLines(logoSet, size, palette, noColor)
				if w <= maxLogoW {
					logoLines = candidate
					logoWidth = w
					break
				}
			}
		}
	}
	infoLines := renderInfoLines(fields, style, labelWidth, valueWidth, palette, noColor)
	topLines := renderFreeLines(style.Text.TopLines, style, palette, noColor, terminalW)
	preFieldLines := renderFreeLines(style.Text.PreFieldLines, style, palette, noColor, 0)
	postFieldLines := renderFreeLines(style.Text.PostFieldLines, style, palette, noColor, 0)
	if identity := renderUserHostLines(style, palette, noColor); len(identity) > 0 {
		preFieldLines = append(identity, preFieldLines...)
	}
	infoWidth := renderedBlockWidth(infoLines, preFieldLines, postFieldLines)

	if sideBySide && logoEnabled {
		needed := style.Layout.LogoLeftPadding + logoWidth + style.Layout.SideBySideGap + infoWidth + sideBySideSafetyMargin()
		if needed > terminalW {
			sideBySide = false
		}
	}

	if logoEnabled && sideBySide && strings.TrimSpace(imagePath) != "" {
		maxLogoWidth := terminalW - style.Layout.SideBySideGap - (1 + labelWidth + 3 + style.Layout.MinValueWidth)
		if maxLogoWidth < logoWidth {
			maxLogoWidth = logoWidth
		}
		if fittedLines, fittedWidth, ok := logo.FitImageLogoForHeight(imagePath, logoWidth, maxLogoWidth, len(infoLines), noColor); ok {
			logoLines = fittedLines
			if fittedWidth != logoWidth {
				logoWidth = fittedWidth
				valueWidth = terminalW - logoWidth - style.Layout.SideBySideGap - (1 + labelWidth + 3)
				if valueWidth < style.Layout.MinValueWidth {
					valueWidth = style.Layout.MinValueWidth
				}
				infoLines = renderInfoLines(fields, style, labelWidth, valueWidth, palette, noColor)
			}
		}
	}

	// re-check after image fitting and truncation adjustments.
	infoWidth = renderedBlockWidth(infoLines, preFieldLines, postFieldLines)
	if sideBySide && logoEnabled {
		needed := style.Layout.LogoLeftPadding + logoWidth + style.Layout.SideBySideGap + infoWidth + sideBySideSafetyMargin()
		if needed > terminalW {
			sideBySide = false
		}
	}

	if !sideBySide {
		valueWidth = 0
		infoLines = renderInfoLines(fields, style, labelWidth, valueWidth, palette, noColor)
		infoWidth = renderedBlockWidth(infoLines, preFieldLines, postFieldLines)
	}

	leftPad := strings.Repeat(" ", style.Layout.LogoLeftPadding)
	swatchLines := paletteSwatchLines(noColor, infoWidth, style)
	coreLines := make([]string, 0, len(topLines)+len(preFieldLines)+len(infoLines)+len(postFieldLines)+len(swatchLines)+8)
	for _, l := range topLines {
		coreLines = append(coreLines, l.Styled)
	}

	if logoEnabled && sideBySide {
		rightBlock := make([]core.RenderedLine, 0, len(preFieldLines)+len(infoLines)+len(postFieldLines))
		rightBlock = append(rightBlock, preFieldLines...)
		rightBlock = append(rightBlock, infoLines...)
		rightBlock = append(rightBlock, postFieldLines...)
		if style.Layout.InfoAlign != "left" {
			rightBlock = alignRenderedBlock(rightBlock, infoWidth, style.Layout.InfoAlign)
		}

		leftBlock, rightBlock := alignSideBlocks(logoLines, rightBlock, style.Layout.SideBySideVerticalAlign)
		if style.Swatch.Show && style.Swatch.Position == "top" {
			if style.Layout.LogoPosition == "right" {
				for _, sw := range swatchLines {
					coreLines = append(coreLines, leftPad+sw+strings.Repeat(" ", style.Layout.SideBySideGap))
				}
			} else {
				for _, sw := range swatchLines {
					coreLines = append(coreLines, leftPad+strings.Repeat(" ", logoWidth+style.Layout.SideBySideGap)+sw)
				}
			}
		}
		total := maxInt(len(leftBlock), len(rightBlock))
		for i := 0; i < total; i++ {
			leftRaw := ""
			leftStyled := ""
			rightStyled := ""
			if i < len(leftBlock) {
				leftRaw = leftBlock[i].Raw
				leftStyled = leftBlock[i].Styled
			}
			if i < len(rightBlock) {
				rightStyled = rightBlock[i].Styled
			}
			logoPart := padRightStyled(leftStyled, leftRaw, logoWidth)
			if style.Layout.LogoPosition == "right" {
				coreLines = append(coreLines, leftPad+rightStyled+strings.Repeat(" ", style.Layout.SideBySideGap)+logoPart)
			} else {
				coreLines = append(coreLines, leftPad+logoPart+strings.Repeat(" ", style.Layout.SideBySideGap)+rightStyled)
			}
		}
		if style.Swatch.Show && style.Swatch.Position != "top" {
			if style.Layout.LogoPosition == "right" {
				for _, sw := range swatchLines {
					coreLines = append(coreLines, leftPad+sw+strings.Repeat(" ", style.Layout.SideBySideGap))
				}
			} else {
				for _, sw := range swatchLines {
					coreLines = append(coreLines, leftPad+strings.Repeat(" ", logoWidth+style.Layout.SideBySideGap)+sw)
				}
			}
		}
		emitRenderedOutput(coreLines, style)
		return
	}

	if logoEnabled {
		for _, l := range logoLines {
			coreLines = append(coreLines, leftPad+l.Styled)
		}
		for i := 0; i < style.Layout.StackSpacingAfterLogo; i++ {
			coreLines = append(coreLines, "")
		}
	}
	if style.Layout.InfoAlign != "left" {
		infoLines = alignRenderedBlock(infoLines, infoWidth, style.Layout.InfoAlign)
		postFieldLines = alignRenderedBlock(postFieldLines, infoWidth, style.Layout.InfoAlign)
		preFieldLines = alignRenderedBlock(preFieldLines, infoWidth, style.Layout.InfoAlign)
	}
	if style.Swatch.Show && style.Swatch.Position == "top" {
		for _, sw := range swatchLines {
			coreLines = append(coreLines, sw)
		}
	}
	for _, l := range preFieldLines {
		coreLines = append(coreLines, l.Styled)
	}
	for _, l := range infoLines {
		coreLines = append(coreLines, l.Styled)
	}
	for _, l := range postFieldLines {
		coreLines = append(coreLines, l.Styled)
	}
	if style.Swatch.Show && style.Swatch.Position != "top" {
		for _, sw := range swatchLines {
			coreLines = append(coreLines, sw)
		}
	}
	emitRenderedOutput(coreLines, style)
}

func emitRenderedOutput(coreLines []string, style core.StyleConfig) {
	lines := coreLines
	if style.Box.Show {
		lines = wrapBoxLines(lines, style.Box)
	}
	for i := 0; i < style.Layout.OuterTopSpacing; i++ {
		fmt.Println()
	}
	for _, line := range lines {
		fmt.Println(line)
	}
	for i := 0; i < style.Layout.OuterBottomSpacing; i++ {
		fmt.Println()
	}
}

func wrapBoxLines(lines []string, box core.StyleBox) []string {
	if len(lines) == 0 {
		lines = []string{""}
	}
	contentW := 0
	for _, line := range lines {
		w := displayWidth(stripANSI(line))
		if w > contentW {
			contentW = w
		}
	}
	innerW := contentW + (box.PaddingX * 2)
	if innerW < 0 {
		innerW = 0
	}

	top := box.TopLeft + strings.Repeat(box.Horizontal, innerW) + box.TopRight
	bottom := box.BottomLeft + strings.Repeat(box.Horizontal, innerW) + box.BottomRight
	blank := box.Vertical + strings.Repeat(" ", innerW) + box.Vertical

	out := make([]string, 0, len(lines)+(box.PaddingY*2)+2)
	out = append(out, top)
	for i := 0; i < box.PaddingY; i++ {
		out = append(out, blank)
	}
	leftPad := strings.Repeat(" ", box.PaddingX)
	for _, line := range lines {
		w := displayWidth(stripANSI(line))
		rightCount := innerW - box.PaddingX - w
		if rightCount < 0 {
			rightCount = 0
		}
		rightPad := strings.Repeat(" ", rightCount)
		out = append(out, box.Vertical+leftPad+ensureLineColorReset(line)+rightPad+box.Vertical)
	}
	for i := 0; i < box.PaddingY; i++ {
		out = append(out, blank)
	}
	out = append(out, bottom)
	return out
}

func ensureLineColorReset(line string) string {
	if !strings.Contains(line, "\x1b[") {
		return line
	}
	if strings.HasSuffix(line, "\x1b[0m") {
		return line
	}
	return line + "\x1b[0m"
}

func chooseLayout(logoSet core.LogoSet, style core.StyleConfig, terminalW, labelWidth int) (bool, string, int, int) {
	baseInfoWidth := 1 + labelWidth + 3 + style.Layout.MinValueWidth
	if style.Layout.Mode == "stack" {
		if terminalW < stackTinyWidth {
			return false, "tiny", 0, 0
		}
		if terminalW < stackCompactWidth {
			return false, "compact", 0, 0
		}
		return false, "normal", 0, 0
	}
	if terminalW < style.Layout.MinSideBySideWidth {
		if terminalW < stackTinyWidth {
			return false, "tiny", 0, 0
		}
		if terminalW < stackCompactWidth {
			return false, "compact", 0, 0
		}
		return false, "normal", 0, 0
	}

	for _, size := range []string{"normal", "compact", "tiny"} {
		_, logoWidth := renderLogoLines(logoSet, size, []string{"37"}, true)
		needed := logoWidth + style.Layout.SideBySideGap + baseInfoWidth
		if terminalW >= needed {
			valueWidth := terminalW - logoWidth - style.Layout.SideBySideGap - (1 + labelWidth + 3)
			if valueWidth < style.Layout.MinValueWidth {
				valueWidth = style.Layout.MinValueWidth
			}
			return true, size, logoWidth, valueWidth
		}
	}
	if style.Layout.Mode == "side" {
		// forced side mode; pick tiny and clamp value width
		_, logoWidth := renderLogoLines(logoSet, "tiny", []string{"37"}, true)
		valueWidth := terminalW - logoWidth - style.Layout.SideBySideGap - (1 + labelWidth + 3)
		if valueWidth < style.Layout.MinValueWidth {
			valueWidth = style.Layout.MinValueWidth
		}
		return true, "tiny", logoWidth, valueWidth
	}

	if terminalW < stackTinyWidth {
		return false, "tiny", 0, 0
	}
	if terminalW < stackCompactWidth {
		return false, "compact", 0, 0
	}
	return false, "normal", 0, 0
}

func sideBySideSafetyMargin() int {
	// older Windows hosts can report optimistic widths
	// so layout falls back to stack before glyph wrapping corrupts the output.
	if runtime.GOOS != "windows" {
		return 0
	}
	return 12
}

func renderInfoLines(fields []core.Field, style core.StyleConfig, labelWidth int, valueWidth int, palette []string, noColor bool) []core.RenderedLine {
	out := make([]core.RenderedLine, 0, len(fields))
	colorEnabled := !noColor && len(palette) > 0
	colorizeFields := colorEnabled && style.Fields.Colorize
	colorizeLabels := colorEnabled && style.Fields.ColorizeLabels
	labelColor, accentColor := labelPaletteColors(palette, noColor)
	for i, f := range fields {
		value := f.Value
		if valueWidth > 0 {
			value = truncateRunes(value, valueWidth)
		}
		labelText := fmt.Sprintf(" %-*s", labelWidth, f.Label)
		separatorText := style.Text.Separator
		raw := labelText + separatorText + value
		if !colorizeFields && !colorizeLabels {
			out = append(out, core.RenderedLine{Raw: raw, Styled: raw})
			continue
		}
		color := palette[i%len(palette)]
		if colorizeFields {
			out = append(out, core.RenderedLine{Raw: raw, Styled: colorLine(color, false, raw)})
			continue
		}
		styledLabel := labelText
		styledSep := separatorText
		if labelColor != "" {
			styledLabel = colorLine(labelColor, false, labelText)
		}
		if accentColor != "" {
			styledSep = colorLine(accentColor, false, separatorText)
		}
		out = append(out, core.RenderedLine{
			Raw:    raw,
			Styled: styledLabel + styledSep + value,
		})
	}
	return out
}

func renderUserHostLines(style core.StyleConfig, palette []string, noColor bool) []core.RenderedLine {
	if !style.Text.ShowUserHost {
		return nil
	}
	identity := userHostIdentity(style)
	if identity == "" {
		return nil
	}
	identity = " " + identity
	out := make([]core.RenderedLine, 0, 2)
	labelColor, accentColor := labelPaletteColors(palette, noColor)
	if labelColor == "" || (!style.Fields.Colorize && !style.Fields.ColorizeLabels) {
		out = append(out, core.RenderedLine{Raw: identity, Styled: identity})
	} else {
		out = append(out, core.RenderedLine{
			Raw:    identity,
			Styled: colorizeIdentity(identity, labelColor, accentColor),
		})
	}
	if style.Text.ShowUserHostBar {
		barWidth := displayWidth(identity) - 1
		if barWidth < 1 {
			barWidth = 1
		}
		bar := " " + strings.Repeat("-", barWidth)
		out = append(out, core.RenderedLine{Raw: bar, Styled: bar})
	}
	return out
}

func userHostIdentity(style core.StyleConfig) string {
	user := sanitizeIdentityPart(style.Text.UserName)
	if user == "" {
		user = sanitizeIdentityPart(os.Getenv("USER"))
	}
	if user == "" {
		user = sanitizeIdentityPart(os.Getenv("USERNAME"))
	}
	host := sanitizeIdentityPart(style.Text.HostName)
	if host == "" {
		h, _ := os.Hostname()
		host = sanitizeIdentityPart(h)
	}
	switch {
	case user == "" && host == "":
		return ""
	case user == "":
		return host
	case host == "":
		return user
	default:
		return user + "@" + host
	}
}

func sanitizeIdentityPart(s string) string {
	s = strings.TrimSpace(stripANSI(s))
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

func labelPaletteColors(palette []string, noColor bool) (string, string) {
	if noColor || len(palette) == 0 {
		return "", ""
	}
	labelColor := palette[0]
	accentColor := ""
	whiteCandidate := ""
	for _, c := range palette[1:] {
		c = strings.TrimSpace(c)
		if c == "" || c == labelColor {
			continue
		}
		if isDefaultLikeWhite(c) {
			if whiteCandidate == "" {
				whiteCandidate = c
			}
			continue
		}
		accentColor = c
		break
	}
	if accentColor == "" {
		if whiteCandidate != "" {
			accentColor = whiteCandidate
		} else {
			accentColor = labelColor
		}
	}
	return labelColor, accentColor
}

func isDefaultLikeWhite(code string) bool {
	c := strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	switch c {
	case "37", "97", "39", "38;2;255;255;255", "38;5;15", "38;5;231":
		return true
	default:
		return false
	}
}

func colorizeIdentity(identity, labelColor, accentColor string) string {
	_ = accentColor
	at := strings.Index(identity, "@")
	if at < 0 || labelColor == "" {
		return identity
	}
	left := identity[:at]
	right := identity[at+1:]
	return colorLine(labelColor, false, left) +
		"@" +
		colorLine(labelColor, false, right)
}

func renderFreeLines(lines []string, style core.StyleConfig, palette []string, noColor bool, width int) []core.RenderedLine {
	out := make([]core.RenderedLine, 0, len(lines))
	for i, line := range lines {
		raw := line
		if width > 0 && style.Text.CenterTopLines {
			pad := width - displayWidth(stripANSI(raw))
			if pad > 0 {
				raw = strings.Repeat(" ", pad/2) + raw
			}
		}
		if strings.Contains(raw, "$") {
			styled, clean, ok := applyPaletteMarkers(raw, palette, noColor, i)
			if ok {
				out = append(out, core.RenderedLine{Raw: clean, Styled: styled})
				continue
			}
		}
		if strings.Contains(raw, "\x1b[") {
			clean := stripANSI(raw)
			if noColor {
				out = append(out, core.RenderedLine{Raw: clean, Styled: clean})
			} else {
				out = append(out, core.RenderedLine{Raw: clean, Styled: ensureLineColorReset(raw)})
			}
			continue
		}
		color := palette[i%maxInt(1, len(palette))]
		out = append(out, core.RenderedLine{Raw: raw, Styled: colorLine(color, noColor, raw)})
	}
	return out
}

func alignSideBlocks(left []core.RenderedLine, right []core.RenderedLine, align string) ([]core.RenderedLine, []core.RenderedLine) {
	lh := len(left)
	rh := len(right)
	if lh == rh {
		return left, right
	}
	padLines := func(n int) []core.RenderedLine {
		p := make([]core.RenderedLine, n)
		for i := 0; i < n; i++ {
			p[i] = core.RenderedLine{Raw: "", Styled: ""}
		}
		return p
	}
	if lh < rh {
		diff := rh - lh
		switch align {
		case "bottom":
			left = append(padLines(diff), left...)
		case "center":
			top := diff / 2
			bot := diff - top
			left = append(append(padLines(top), left...), padLines(bot)...)
		default:
			left = append(left, padLines(diff)...)
		}
		return left, right
	}
	diff := lh - rh
	switch align {
	case "bottom":
		right = append(padLines(diff), right...)
	case "center":
		top := diff / 2
		bot := diff - top
		right = append(append(padLines(top), right...), padLines(bot)...)
	default:
		right = append(right, padLines(diff)...)
	}
	return left, right
}

func alignRenderedBlock(lines []core.RenderedLine, width int, align string) []core.RenderedLine {
	if align == "left" || width <= 0 {
		return lines
	}
	out := make([]core.RenderedLine, 0, len(lines))
	for _, line := range lines {
		pad := 0
		w := displayWidth(line.Raw)
		if width > w {
			if align == "right" {
				pad = width - w
			} else if align == "center" {
				pad = (width - w) / 2
			}
		}
		if pad <= 0 {
			out = append(out, line)
			continue
		}
		spaces := strings.Repeat(" ", pad)
		out = append(out, core.RenderedLine{
			Raw:    spaces + line.Raw,
			Styled: spaces + line.Styled,
		})
	}
	return out
}

func renderLogoLines(logo core.LogoSet, size string, palette []string, noColor bool) ([]core.RenderedLine, int) {
	var lines []core.LogoLine
	switch size {
	case "tiny":
		lines = logo.Tiny
	case "compact":
		lines = logo.Compact
	default:
		lines = logo.Normal
	}
	if len(lines) == 0 {
		lines = []core.LogoLine{{Text: "dawnfetch", ColorIndex: 0}}
	}

	out := make([]core.RenderedLine, 0, len(lines))
	maxW := 0
	if len(palette) == 0 {
		palette = []string{"37"}
	}
	for i, l := range lines {
		raw := l.Text
		if l.RawANSI {
			clean := stripANSI(raw)
			styled := raw
			if noColor {
				styled = clean
			}
			out = append(out, core.RenderedLine{Raw: clean, Styled: styled})
			if len(clean) > maxW {
				maxW = displayWidth(clean)
			}
			continue
		}
		if strings.Contains(raw, "$") {
			styled, clean, ok := applyPaletteMarkers(raw, palette, noColor, i)
			if ok {
				out = append(out, core.RenderedLine{Raw: clean, Styled: styled})
				if w := displayWidth(clean); w > maxW {
					maxW = w
				}
				continue
			}
		}
		idx := l.ColorIndex
		if idx < 0 {
			idx = i
		}
		color := palette[idx%len(palette)]
		styled := colorLine(color, noColor, raw)
		out = append(out, core.RenderedLine{Raw: raw, Styled: styled})
		if w := displayWidth(raw); w > maxW {
			maxW = w
		}
	}
	return out, maxW
}

func stripANSI(s string) string {
	return ansiStripRE.ReplaceAllString(s, "")
}

func applyPaletteMarkers(line string, palette []string, noColor bool, defaultIdx int) (string, string, bool) {
	if len(palette) == 0 {
		palette = []string{"37"}
	}
	r := []rune(line)
	var plain strings.Builder
	var styled strings.Builder
	curColor := markerColorCode(defaultIdx+1, palette)
	usedMarker := false
	wroteVisible := false
	i := 0
	for i < len(r) {
		if r[i] == '$' && i+1 < len(r) && unicode.IsDigit(r[i+1]) {
			j := i + 1
			for j < len(r) && unicode.IsDigit(r[j]) {
				j++
			}
			n, _ := strconv.Atoi(string(r[i+1 : j]))
			if n > 0 {
				curColor = markerColorCode(n, palette)
				if !noColor && wroteVisible {
					styled.WriteString("\x1b[0m\x1b[")
					styled.WriteString(curColor)
					styled.WriteString("m")
				}
			}
			usedMarker = true
			i = j
			continue
		}
		ch := string(r[i])
		plain.WriteString(ch)
		if !noColor {
			if !wroteVisible {
				styled.WriteString("\x1b[")
				styled.WriteString(curColor)
				styled.WriteString("m")
				wroteVisible = true
			}
			styled.WriteString(ch)
		}
		i++
	}
	if !usedMarker {
		return line, line, false
	}
	if noColor {
		return plain.String(), plain.String(), true
	}
	if wroteVisible {
		styled.WriteString("\x1b[0m")
	}
	return styled.String(), plain.String(), true
}

func markerColorCode(marker int, palette []string) string {
	if marker <= 0 {
		marker = 1
	}
	if len(palette) == 0 {
		return "37"
	}
	// keep marker colors tied to the active theme palette.
	return palette[(marker-1)%len(palette)]
}

func labelWidth(fields []core.Field) int {
	max := 0
	for _, f := range fields {
		if len(f.Label) > max {
			max = len(f.Label)
		}
	}
	return max
}

func terminalWidth() int {
	if n := platform.GetTerminalWidth(); n > 0 {
		if runtime.GOOS == "windows" {
			return n + windowsWidthBoost
		}
		return n
	}
	if runtime.GOOS == "windows" {
		return 80 + windowsWidthBoost
	}
	return 140
}

func colorLine(code string, noColor bool, s string) string {
	if noColor {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func terminalColorLevel() int {
	if os.Getenv("NO_COLOR") != "" {
		return colorLevelNone
	}
	force := strings.TrimSpace(os.Getenv("CLICOLOR_FORCE"))
	forceEnabled := force != "" && force != "0"
	if v := strings.TrimSpace(os.Getenv("CLICOLOR")); v == "0" && !forceEnabled {
		return colorLevelNone
	}
	term := strings.ToLower(strings.TrimSpace(os.Getenv("TERM")))
	colorterm := strings.ToLower(strings.TrimSpace(os.Getenv("COLORTERM")))

	if strings.Contains(colorterm, "truecolor") || strings.Contains(colorterm, "24bit") ||
		strings.Contains(term, "truecolor") || strings.Contains(term, "24bit") {
		return colorLevelTrueColor
	}
	if strings.Contains(term, "256color") {
		return colorLevelANSI256
	}
	if term == "" || term == "dumb" {
		if forceEnabled {
			return colorLevelANSI16
		}
		return colorLevelNone
	}
	return colorLevelANSI16
}

func normalizePaletteForColorLevel(palette []string, level int) []string {
	if level >= colorLevelTrueColor {
		return palette
	}
	out := make([]string, 0, len(palette))
	for _, code := range palette {
		out = append(out, normalizeColorCode(code, level))
	}
	return out
}

func normalizeColorCode(code string, level int) string {
	c := strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if c == "" {
		return "37"
	}
	r, g, b, ok := parseTrueColorCode(c)
	if !ok {
		return c
	}
	if level == colorLevelANSI256 {
		return fmt.Sprintf("38;5;%d", rgbToANSI256(r, g, b))
	}
	return strconv.Itoa(rgbToANSI16(r, g, b))
}

func parseTrueColorCode(code string) (int, int, int, bool) {
	parts := strings.Split(code, ";")
	if len(parts) != 5 || parts[0] != "38" || parts[1] != "2" {
		return 0, 0, 0, false
	}
	r, errR := strconv.Atoi(parts[2])
	g, errG := strconv.Atoi(parts[3])
	b, errB := strconv.Atoi(parts[4])
	if errR != nil || errG != nil || errB != nil {
		return 0, 0, 0, false
	}
	return clamp255(r), clamp255(g), clamp255(b), true
}

func clamp255(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

func rgbToANSI256(r, g, b int) int {
	if r == g && g == b {
		if r < 8 {
			return 16
		}
		if r > 248 {
			return 231
		}
		return 232 + ((r-8)*24+123)/247
	}
	ri := (r*5 + 127) / 255
	gi := (g*5 + 127) / 255
	bi := (b*5 + 127) / 255
	return 16 + (36 * ri) + (6 * gi) + bi
}

func rgbToANSI16(r, g, b int) int {
	ansi16 := [16][3]int{
		{0, 0, 0},
		{205, 0, 0},
		{0, 205, 0},
		{205, 205, 0},
		{0, 0, 238},
		{205, 0, 205},
		{0, 205, 205},
		{229, 229, 229},
		{127, 127, 127},
		{255, 0, 0},
		{0, 255, 0},
		{255, 255, 0},
		{92, 92, 255},
		{255, 0, 255},
		{0, 255, 255},
		{255, 255, 255},
	}
	bestIdx := 7
	bestDist := int(^uint(0) >> 1)
	for i, c := range ansi16 {
		dr := r - c[0]
		dg := g - c[1]
		db := b - c[2]
		dist := dr*dr + dg*dg + db*db
		if dist < bestDist {
			bestDist = dist
			bestIdx = i
		}
	}
	if bestIdx < 8 {
		return 30 + bestIdx
	}
	return 90 + (bestIdx - 8)
}

func padRightStyled(styled string, raw string, width int) string {
	pad := width - displayWidth(raw)
	if pad < 0 {
		pad = 0
	}
	return styled + strings.Repeat(" ", pad)
}

func displayWidth(s string) int {
	return utf8.RuneCountInString(s)
}

func paletteSwatchLines(noColor bool, fieldWidth int, style core.StyleConfig) []string {
	if noColor || !style.Swatch.Show {
		return nil
	}
	block := style.Swatch.Block
	if block == "" {
		block = "   "
	}
	targetWidth := fieldWidth
	if tw := terminalWidth(); tw > 0 {
		// keep swatch rows within visible terminal width to prevent wrapped/split artifacts.
		if targetWidth <= 0 || targetWidth > tw {
			targetWidth = tw
		}
	}
	rowBlock := block
	blockW := displayWidth(rowBlock)
	if blockW <= 0 {
		blockW = 1
	}
	if targetWidth > 0 && blockW > targetWidth {
		rowBlock = " "
		blockW = 1
	}
	paintBG := func(code int) string { return fmt.Sprintf("\x1b[%dm%s\x1b[0m", code, rowBlock) }
	buildRow := func(codes []int) string {
		if targetWidth > 0 {
			maxCodes := targetWidth / blockW
			if maxCodes <= 0 {
				maxCodes = 1
			}
			if len(codes) > maxCodes {
				codes = codes[:maxCodes]
			}
		}
		var b strings.Builder
		for _, c := range codes {
			b.WriteString(paintBG(c))
		}
		row := b.String()
		if !style.Swatch.Center {
			return row
		}
		pad := 0
		rowW := displayWidth(strings.Repeat(rowBlock, len(codes)))
		if targetWidth > rowW {
			pad = (targetWidth - rowW) / 2
		}
		return strings.Repeat(" ", pad) + row
	}
	out := make([]string, 0, style.Text.SwatchTopGap+4)
	for i := 0; i < style.Text.SwatchTopGap; i++ {
		out = append(out, "")
	}
	if len(style.Swatch.CustomRows) > 0 {
		for _, row := range style.Swatch.CustomRows {
			out = append(out, buildRow(row))
		}
		return out
	}
	for _, row := range core.StylePaletteRowNames(style) {
		if row == "normal" {
			out = append(out, buildRow([]int{40, 41, 42, 43, 44, 45, 46, 47}))
		} else if row == "bright" {
			out = append(out, buildRow([]int{100, 101, 102, 103, 104, 105, 106, 107}))
		}
	}
	return out
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}

func renderedBlockWidth(blocks ...[]core.RenderedLine) int {
	maxW := 0
	for _, lines := range blocks {
		for _, l := range lines {
			if w := displayWidth(l.Raw); w > maxW {
				maxW = w
			}
		}
	}
	return maxW
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
