// this file parses top-level cli flags and coordinates one dawnfetch execution.
package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"dawnfetch/internal/dawnfetch/config"
	"dawnfetch/internal/dawnfetch/core"
	"dawnfetch/internal/dawnfetch/platform"
	"dawnfetch/internal/dawnfetch/render"
	"dawnfetch/internal/dawnfetch/system"
	"dawnfetch/internal/dawnfetch/tui/onboarding"
)

var version = "0.1.5"

func Run() int {
	args := os.Args
	if len(args) > 1 && strings.EqualFold(strings.TrimSpace(args[1]), "dawnfetch") {
		args = append([]string{args[0]}, args[2:]...)
	}
	args = normalizePreviewArgs(args)

	if len(args) > 1 {
		first := strings.ToLower(strings.TrimSpace(args[1]))
		if first == "--help" || first == "-h" || first == "-help" {
			printMainHelp(os.Stdout)
			return 0
		}
		cmd := strings.ToLower(strings.TrimSpace(args[1]))
		if !strings.HasPrefix(cmd, "-") {
			switch cmd {
			case "help":
				return runHelpCommand(args[2:])
			case "list-themes", "themes":
				return runListThemesCommand(args[2:])
			case "doctor":
				return runDoctorCommand(args[2:])
			case "set-default-theme":
				return runSetDefaultThemeCommand(args[2:])
			case "version", "--version", "-v":
				fmt.Println("dawnfetch", version)
				return 0
			default:
				printCLIError(fmt.Sprintf("unknown command %q", strings.TrimSpace(args[1])), "run `dawnfetch --help` to list commands")
				return 2
			}
		}
	}

	flag.CommandLine = flag.NewFlagSet(args[0], flag.ContinueOnError)
	flag.CommandLine.SetOutput(io.Discard)
	flag.CommandLine.Usage = func() {}

	themeName := flag.String("theme", "", "theme palette")
	themesPath := flag.String("themes", "themes.json", "path to theme palettes json")
	noColor := flag.Bool("no-color", false, "disable ANSI colors")
	noLogo := flag.Bool("no-logo", false, "disable logo rendering")
	full := flag.Bool("full", false, "run slower probes for more complete output")
	imagePath := flag.String("image", "", "path to an image logo (png/jpg/webp/gif/bmp/tiff; no svg)")

	helpFlag := flag.Bool("help", false, "show help")
	listThemesFlag := flag.Bool("list-themes", false, "list available themes and exit")
	doctorFlag := flag.Bool("doctor", false, "run environment diagnostics and exit")
	versionFlag := flag.Bool("version", false, "show version and exit")
	previewThemeFlag := flag.Bool("preview-theme", false, "open interactive preview")
	setDefaultThemeFlag := flag.String("set-default-theme", "", "set persistent default theme and exit")

	if err := flag.CommandLine.Parse(args[1:]); err != nil {
		if err == flag.ErrHelp {
			printMainHelp(os.Stdout)
			return 0
		}
		return handleFlagParseError(err, "run `dawnfetch --help` for usage")
	}
	if *helpFlag {
		printMainHelp(os.Stdout)
		return 0
	}
	if *versionFlag {
		fmt.Println("dawnfetch", version)
		return 0
	}
	if *listThemesFlag {
		return runListThemes(*themesPath)
	}
	if *doctorFlag {
		return runDoctor(*themesPath)
	}
	if *previewThemeFlag {
		name := strings.TrimSpace(*themeName)
		if name == "" && flag.NArg() > 0 {
			name = strings.TrimSpace(flag.Arg(0))
		}
		return runPreviewTheme(name, *themesPath, *noColor)
	}
	if strings.TrimSpace(*setDefaultThemeFlag) != "" {
		return runSetDefaultTheme(strings.TrimSpace(*setDefaultThemeFlag), *themesPath)
	}

	runFast := !*full
	isTTY := stdoutIsTerminal()
	if err := onboarding.RunIfFirstLaunch(*themesPath, *noColor, isTTY, version); err != nil {
		printCLIError(err.Error(), "")
		return 1
	}

	// Non-interactive fast path for benchmarks/pipes.
	nonInteractiveFast := runFast && !isTTY
	if nonInteractiveFast {
		*noLogo = true
		*noColor = true
	}

	brandCfg := core.DefaultBrandConfig()
	skipPaletteLoad := nonInteractiveFast &&
		strings.TrimSpace(*themeName) == "" &&
		strings.TrimSpace(*themesPath) == "themes.json"
	if !skipPaletteLoad {
		if palettes, err := config.LoadThemePalettes(*themesPath); err == nil {
			brandCfg.Palettes = palettes
		} else {
			printCLIError(err.Error(), "")
			return 1
		}
	}

	styleCfg := core.DefaultStyleConfig()
	if *noLogo {
		styleCfg.Layout.ShowLogo = false
	}
	if !*noColor && isTTY && !platform.EnableANSIIfSupported() {
		*noColor = true
	}

	effectiveTheme := strings.TrimSpace(*themeName)
	if effectiveTheme == "" {
		if skipPaletteLoad {
			effectiveTheme = core.DefaultPalette
		} else {
			cfg, err := config.LoadUserConfig()
			if err != nil {
				printCLIError(fmt.Sprintf("failed to load user config: %v", err), "")
				return 1
			}
			if t := strings.TrimSpace(cfg.DefaultTheme); t != "" {
				effectiveTheme = t
			} else {
				effectiveTheme = core.DefaultPalette
			}
		}
	} else {
		normalized, ok := core.ResolvePaletteNameStrict(effectiveTheme, brandCfg.Palettes)
		if !ok {
			printCLIError(fmt.Sprintf("unknown theme %q", effectiveTheme), "run `dawnfetch --list-themes` to see valid themes")
			return 2
		}
		effectiveTheme = normalized
	}

	collected := system.Collect(runFast, *full)
	render.Print(collected, brandCfg, styleCfg, effectiveTheme, *imagePath, *noColor, *noLogo)
	return 0
}
