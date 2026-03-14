// this file defines the built-in render style used by dawnfetch.
// style customization from external files is intentionally disabled.
package core

import "strings"

type StyleConfig struct {
	Version  int
	Defaults StyleDefaults
	Layout   StyleLayout
	Text     StyleText
	Fields   StyleFields
	Swatch   StyleSwatch
	Box      StyleBox
}

type StyleDefaults struct {
	Theme    string
	UseColor bool
}

type StyleLayout struct {
	Mode                    string // auto|side|stack
	ShowLogo                bool
	LogoPosition            string // left|right
	SideBySideGap           int
	LogoLeftPadding         int
	OuterTopSpacing         int
	OuterBottomSpacing      int
	StackSpacingAfterLogo   int
	MinSideBySideWidth      int
	MinValueWidth           int
	SideBySideVerticalAlign string // top|center|bottom
	InfoAlign               string // left|center|right
}

type StyleText struct {
	TopLines        []string
	PreFieldLines   []string
	PostFieldLines  []string
	UserName        string
	HostName        string
	FieldPrefix     string
	FieldSuffix     string
	LabelPrefix     string
	LabelSuffix     string
	ValuePrefix     string
	ValueSuffix     string
	Separator       string
	SwatchTopGap    int
	CenterTopLines  bool
	ShowUserHost    bool
	ShowUserHostBar bool
}

type StyleFields struct {
	Order          []string
	Rename         map[string]string
	Titles         map[string]string
	Prefix         map[string]string
	Suffix         map[string]string
	Hide           []string
	Only           []string
	BoldTitles     bool
	Colorize       bool
	ColorizeLabels bool
}

type StyleSwatch struct {
	Show       bool
	Center     bool
	Block      string
	Rows       []string // normal|bright
	CustomRows [][]int
	Position   string // bottom|top|none
}

type StyleBox struct {
	Show        bool
	Horizontal  string
	Vertical    string
	TopLeft     string
	TopRight    string
	BottomLeft  string
	BottomRight string
	PaddingX    int
	PaddingY    int
}

func DefaultStyleConfig() StyleConfig {
	return StyleConfig{
		Version: 1,
		Defaults: StyleDefaults{
			Theme:    "",
			UseColor: true,
		},
		Layout: StyleLayout{
			Mode:                    "auto",
			ShowLogo:                true,
			LogoPosition:            "left",
			SideBySideGap:           6,
			LogoLeftPadding:         3,
			OuterTopSpacing:         1,
			OuterBottomSpacing:      1,
			StackSpacingAfterLogo:   1,
			MinSideBySideWidth:      72,
			MinValueWidth:           20,
			SideBySideVerticalAlign: "top",
			InfoAlign:               "left",
		},
		Text: StyleText{
			Separator:       " : ",
			SwatchTopGap:    1,
			ShowUserHost:    true,
			ShowUserHostBar: true,
		},
		Fields: StyleFields{
			Rename:         map[string]string{},
			Titles:         map[string]string{},
			Prefix:         map[string]string{},
			Suffix:         map[string]string{},
			BoldTitles:     true,
			Colorize:       false,
			ColorizeLabels: true,
		},
		Swatch: StyleSwatch{
			Show:     true,
			Center:   false,
			Block:    "   ",
			Rows:     []string{"normal", "bright"},
			Position: "bottom",
		},
		Box: StyleBox{
			Show:        false,
			Horizontal:  "─",
			Vertical:    "│",
			TopLeft:     "┌",
			TopRight:    "┐",
			BottomLeft:  "└",
			BottomRight: "┘",
			PaddingX:    1,
			PaddingY:    0,
		},
	}
}

func StylePaletteRowNames(style StyleConfig) []string {
	rows := make([]string, 0, len(style.Swatch.Rows))
	for _, r := range style.Swatch.Rows {
		v := strings.ToLower(strings.TrimSpace(r))
		if v == "normal" || v == "bright" {
			rows = append(rows, v)
		}
	}
	if len(rows) == 0 && len(style.Swatch.CustomRows) == 0 {
		rows = append(rows, "normal", "bright")
	}
	return rows
}
