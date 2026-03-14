// this file contains default field collectors and shared helpers.
package system

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

var xfconfValueRE = regexp.MustCompile(`value=\"([^\"]+)\"`)

func osNameVersion() string {
	switch runtime.GOOS {
	case "linux":
		rel := linuxOSRelease()
		if rel.PrettyName != "" {
			return rel.PrettyName
		}
		if out, _ := runCmd(160*time.Millisecond, "uname", "-sr"); out != "" {
			return out
		}
	case "darwin":
		name, _ := runCmd(220*time.Millisecond, "sw_vers", "-productName")
		ver, _ := runCmd(220*time.Millisecond, "sw_vers", "-productVersion")
		if name != "" || ver != "" {
			return strings.TrimSpace(name + " " + ver)
		}
	case "windows":
		if wf := windowsFacts(); wf.Valid {
			parts := nonEmpty([]string{wf.Caption, wf.Version})
			if len(parts) > 0 {
				return strings.Join(parts, " ")
			}
		}
		if major, minor, build, ok := windowsVersionAPI(); ok {
			return fmt.Sprintf("Microsoft Windows %d.%d.%d", major, minor, build)
		}
		if fastCollectMode.Load() || !windowsSlowProbesEnabled() {
			return "Microsoft Windows"
		}
		if out, _ := runCmd(500*time.Millisecond, "cmd", "/c", "ver"); out != "" {
			return out
		}
	}
	return runtime.GOOS
}

func hostModel(fast bool) string {
	switch runtime.GOOS {
	case "linux":
		parts := []string{readFirstLine("/sys/devices/virtual/dmi/id/sys_vendor"), readFirstLine("/sys/devices/virtual/dmi/id/product_name")}
		v := strings.TrimSpace(strings.Join(nonEmpty(parts), " "))
		if v != "" {
			return compactSpaces(v)
		}
	case "darwin":
		if out, _ := runCmd(160*time.Millisecond, "sysctl", "-n", "hw.model"); out != "" {
			return out
		}
	case "windows":
		if wf := windowsFacts(); wf.Valid {
			v := compactSpaces(strings.TrimSpace(wf.Manufacturer + " " + wf.Model))
			if v != "" {
				return v
			}
		}
		if fast || !windowsSlowProbesEnabled() {
			break
		}
		if out, _ := runCmd(600*time.Millisecond, "powershell", "-NoProfile", "-Command", "(Get-CimInstance Win32_ComputerSystem).Model"); out != "" {
			return out
		}
	}
	if !fast && runtime.GOOS == "linux" {
		if out, _ := runCmd(220*time.Millisecond, "hostnamectl", "--static"); out != "" {
			return out
		}
	}
	if hn, err := os.Hostname(); err == nil && hn != "" {
		return hn
	}
	return "unknown"
}

func kernelVersion() string {
	switch runtime.GOOS {
	case "linux":
		if v := readFirstLine("/proc/sys/kernel/osrelease"); v != "" {
			return v
		}
		if out, _ := runCmd(120*time.Millisecond, "uname", "-r"); out != "" {
			return out
		}
	case "windows":
		if wf := windowsFacts(); wf.Valid && strings.TrimSpace(wf.Version) != "" {
			return wf.Version
		}
		if major, minor, build, ok := windowsVersionAPI(); ok {
			return fmt.Sprintf("%d.%d.%d", major, minor, build)
		}
		if fastCollectMode.Load() || !windowsSlowProbesEnabled() {
			return "unknown"
		}
		if out, _ := runCmd(500*time.Millisecond, "cmd", "/c", "ver"); out != "" {
			re := regexp.MustCompile(`\b(\d+\.\d+\.\d+)\b`)
			if m := re.FindStringSubmatch(out); len(m) == 2 {
				return m[1]
			}
		}
		if out, _ := runCmd(1500*time.Millisecond, "powershell", "-NoProfile", "-Command", "(Get-CimInstance Win32_OperatingSystem).Version"); out != "" {
			return out
		}
	default:
		if out, _ := runCmd(160*time.Millisecond, "uname", "-r"); out != "" {
			return out
		}
	}
	return "unknown"
}

func uptimeString() string {
	u := uptimeSeconds()
	if u <= 0 {
		return "unknown"
	}
	d := u / 86400
	h := (u % 86400) / 3600
	m := (u % 3600) / 60
	if d > 0 {
		return fmt.Sprintf("%dd %dh %dm", d, h, m)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}

func uptimeSeconds() int64 {
	switch runtime.GOOS {
	case "linux":
		line := readFirstLine("/proc/uptime")
		if line != "" {
			f := strings.Fields(line)
			if len(f) > 0 {
				if secs, err := strconv.ParseFloat(f[0], 64); err == nil {
					return int64(secs)
				}
			}
		}
	case "darwin":
		out, _ := runCmd(220*time.Millisecond, "sysctl", "-n", "kern.boottime")
		re := regexp.MustCompile(`sec\s*=\s*(\d+)`)
		if m := re.FindStringSubmatch(out); len(m) == 2 {
			if sec, err := strconv.ParseInt(m[1], 10, 64); err == nil {
				return time.Now().Unix() - sec
			}
		}
	case "windows":
		if wf := windowsFacts(); wf.Valid {
			if bt := parseWindowsBootTime(wf.LastBoot); !bt.IsZero() {
				return time.Now().Unix() - bt.Unix()
			}
		}
		if u := windowsUptimeSecondsAPI(); u > 0 {
			return u
		}
		if fastCollectMode.Load() || !windowsSlowProbesEnabled() {
			return 0
		}
		ps := "[math]::Floor((Get-Date).ToUniversalTime().Subtract((Get-CimInstance Win32_OperatingSystem).LastBootUpTime.ToUniversalTime()).TotalSeconds)"
		out, _ := runCmd(1800*time.Millisecond, "powershell", "-NoProfile", "-Command", ps)
		if v, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64); err == nil {
			return v
		}
	}
	return 0
}

func packagesCount(fast bool) string {
	if runtime.GOOS == "windows" {
		return ""
	}

	if runtime.GOOS == "darwin" {
		counts := map[string]int{}
		if count := fastMacPackageCount(); count > 0 {
			counts["pkgutil"] = count
		}
		if count := fastMacBrewCount(); count > 0 {
			counts["brew"] = count
		}
		if len(counts) == 0 && commandExists("pkgutil") {
			if out, _ := runCmd(800*time.Millisecond, "pkgutil", "--pkgs"); out != "" {
				counts["pkgutil"] = countLines(out)
			}
		}
		if len(counts) == 0 {
			return "n/a"
		}
		keys := make([]string, 0, len(counts))
		total := 0
		for k, v := range counts {
			keys = append(keys, k)
			total += v
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s %d", k, counts[k]))
		}
		return fmt.Sprintf("%d (%s)", total, strings.Join(parts, ", "))
	}

	if runtime.GOOS == "linux" && fast {
		if v := fastLinuxPackageCount(); v != "" {
			return v
		}
		return "n/a"
	}

	counts := map[string]int{}
	if commandExists("dpkg-query") {
		if out, _ := runCmd(500*time.Millisecond, "dpkg-query", "-f", ".", "-W"); out != "" {
			counts["dpkg"] = len(out)
		}
	}
	if commandExists("rpm") {
		if out, _ := runCmd(500*time.Millisecond, "rpm", "-qa"); out != "" {
			counts["rpm"] = countLines(out)
		}
	}
	if commandExists("pacman") {
		if out, _ := runCmd(500*time.Millisecond, "pacman", "-Qq"); out != "" {
			counts["pacman"] = countLines(out)
		}
	}
	if !fast && commandExists("flatpak") {
		if out, _ := runCmd(700*time.Millisecond, "flatpak", "list", "--app"); out != "" {
			counts["flatpak"] = maxInt(0, countLines(out)-1)
		}
	}
	if !fast && commandExists("brew") {
		if out, _ := runCmd(700*time.Millisecond, "brew", "list"); out != "" {
			counts["brew"] = countLines(out)
		}
	}
	if len(counts) == 0 {
		return "n/a"
	}

	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	total := 0
	for _, k := range keys {
		total += counts[k]
		parts = append(parts, fmt.Sprintf("%s %d", k, counts[k]))
	}
	return fmt.Sprintf("%d (%s)", total, strings.Join(parts, ", "))
}

func shellInfo(fast bool) string {
	if runtime.GOOS == "windows" {
		if ps := os.Getenv("PSModulePath"); ps != "" {
			if fast || !windowsSlowProbesEnabled() {
				return "PowerShell"
			}
			if out, _ := runCmd(300*time.Millisecond, "powershell", "-NoProfile", "-Command", "$PSVersionTable.PSVersion.ToString()"); out != "" {
				return "PowerShell " + out
			}
			return "PowerShell"
		}
		if comspec := os.Getenv("ComSpec"); comspec != "" {
			return filepath.Base(comspec)
		}
		return "cmd"
	}
	sh := os.Getenv("SHELL")
	if sh == "" {
		return "unknown"
	}
	name := filepath.Base(sh)
	if fast {
		return name
	}
	if out, _ := runCmd(220*time.Millisecond, name, "--version"); out != "" {
		if line := firstNonEmptyLine(out); line != "" {
			return line
		}
	}
	return name
}

func resolutionInfo(fast bool) string {
	switch runtime.GOOS {
	case "linux":
		if fast {
			if v := fastLinuxResolution(); v != "" {
				return v
			}
			return "unknown"
		}
		if commandExists("xrandr") {
			if out, _ := runCmd(220*time.Millisecond, "xrandr", "--current"); out != "" {
				if v := parseXrandr(out); v != "" {
					return v
				}
			}
		}
		if !fast && commandExists("wlr-randr") {
			if out, _ := runCmd(260*time.Millisecond, "wlr-randr"); out != "" {
				if v := parseWlrRandr(out); v != "" {
					return v
				}
			}
		}
	case "darwin":
		if out, _ := runCmd(800*time.Millisecond, "system_profiler", "SPDisplaysDataType"); out != "" {
			res, rr := parseMacDisplay(out)
			if res != "" && rr != "" {
				return res + " @ " + rr
			}
			if res != "" {
				return res
			}
		}
	case "windows":
		if wf := windowsFacts(); wf.Valid {
			if wf.ResX > 0 && wf.ResY > 0 {
				if wf.ResHz > 0 {
					return fmt.Sprintf("%dx%d @ %dHz", wf.ResX, wf.ResY, wf.ResHz)
				}
				return fmt.Sprintf("%dx%d", wf.ResX, wf.ResY)
			}
		}
		if w, h, ok := windowsResolutionAPI(); ok {
			return fmt.Sprintf("%dx%d", w, h)
		}
		if fast || !windowsSlowProbesEnabled() {
			return "unknown"
		}
		ps := "$v=Get-CimInstance Win32_VideoController | Select-Object -First 1; if($v){\"$($v.CurrentHorizontalResolution)x$($v.CurrentVerticalResolution) @ $($v.CurrentRefreshRate)Hz\"}"
		if out, _ := runCmd(1800*time.Millisecond, "powershell", "-NoProfile", "-Command", ps); out != "" {
			return out
		}
	}
	return "unknown"
}

func desktopEnvironment() string {
	switch runtime.GOOS {
	case "linux":
		for _, k := range []string{"XDG_CURRENT_DESKTOP", "DESKTOP_SESSION", "GDMSESSION"} {
			if v := os.Getenv(k); strings.TrimSpace(v) != "" {
				return v
			}
		}
	case "darwin":
		return "Aqua"
	case "windows":
		return "Explorer"
	}
	return "unknown"
}

func windowManager(fast bool) string {
	switch runtime.GOOS {
	case "linux":
		if fast {
			if os.Getenv("WAYLAND_DISPLAY") != "" {
				return "Wayland compositor"
			}
			if os.Getenv("DISPLAY") != "" {
				return "X11 window manager"
			}
			return "unknown"
		}
		if commandExists("wmctrl") {
			if out, _ := runCmd(220*time.Millisecond, "wmctrl", "-m"); out != "" {
				for _, l := range strings.Split(out, "\n") {
					if strings.HasPrefix(l, "Name:") {
						return strings.TrimSpace(strings.TrimPrefix(l, "Name:"))
					}
				}
			}
		}
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			return "Wayland compositor"
		}
		if os.Getenv("DISPLAY") != "" {
			return "X11 window manager"
		}
	case "darwin":
		return "WindowServer"
	case "windows":
		return "Desktop Window Manager"
	}
	return "unknown"
}

func visualSettings(fast bool) string {
	switch runtime.GOOS {
	case "linux":
		if fast {
			parts := []string{}
			if gtk := strings.TrimSpace(os.Getenv("GTK_THEME")); gtk != "" {
				parts = append(parts, "theme "+gtk)
			}
			if icons := strings.TrimSpace(os.Getenv("ICON_THEME")); icons != "" {
				parts = append(parts, "icons "+icons)
			}
			if font := strings.TrimSpace(os.Getenv("FONTCONFIG_FILE")); font != "" {
				parts = append(parts, "font "+filepath.Base(font))
			}
			if len(parts) == 0 {
				parts = linuxThemeFromFiles()
			}
			if len(parts) > 0 {
				return strings.Join(parts, " | ")
			}
			return "unknown"
		}
		parts := linuxThemeFromFiles()
		if len(parts) == 0 {
			parts = linuxThemeFromCommands(false)
		}
		if len(parts) > 0 {
			return strings.Join(parts, " | ")
		}
	case "darwin":
		theme := "Light"
		if out, _ := runCmd(180*time.Millisecond, "defaults", "read", "-g", "AppleInterfaceStyle"); strings.Contains(strings.ToLower(out), "dark") {
			theme = "Dark"
		}
		if font, _ := runCmd(180*time.Millisecond, "defaults", "read", "-g", "AppleFontSmoothing"); font != "" {
			return fmt.Sprintf("theme %s | font smoothing %s", theme, strings.TrimSpace(font))
		}
		return "theme " + theme
	case "windows":
		if wf := windowsFacts(); wf.Valid {
			switch wf.AppsUseLightTheme {
			case 1:
				return "theme Light"
			case 0:
				return "theme Dark"
			}
		}
		if theme, ok := windowsThemeFromRegistry(); ok {
			return theme
		}
		if fast || !windowsSlowProbesEnabled() {
			return "theme unknown"
		}
		ps := "$v=Get-ItemProperty -Path 'HKCU:\\Software\\Microsoft\\Windows\\CurrentVersion\\Themes\\Personalize' -Name AppsUseLightTheme -ErrorAction SilentlyContinue; if($null -ne $v){if($v.AppsUseLightTheme -eq 1){'theme Light'}else{'theme Dark'}}"
		if out, _ := runCmd(900*time.Millisecond, "powershell", "-NoProfile", "-Command", ps); out != "" {
			return out
		}
	}
	return "unknown"
}

func windowsThemeFromRegistry() (string, bool) {
	out, _ := runCmd(
		650*time.Millisecond,
		"reg",
		"query",
		`HKCU\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`,
		"/v",
		"AppsUseLightTheme",
	)
	if strings.TrimSpace(out) == "" {
		return "", false
	}
	re := regexp.MustCompile(`(?i)\bAppsUseLightTheme\b[^\n\r]*\b0x([0-9a-f]+)\b`)
	m := re.FindStringSubmatch(out)
	if len(m) != 2 {
		return "", false
	}
	v, err := strconv.ParseInt(m[1], 16, 64)
	if err != nil {
		return "", false
	}
	if v == 1 {
		return "theme Light", true
	}
	if v == 0 {
		return "theme Dark", true
	}
	return "", false
}

func linuxThemeFromFiles() []string {
	paths := []string{
		filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "gtk-4.0", "settings.ini"),
		filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "gtk-3.0", "settings.ini"),
		filepath.Join(os.Getenv("HOME"), ".config", "gtk-4.0", "settings.ini"),
		filepath.Join(os.Getenv("HOME"), ".config", "gtk-3.0", "settings.ini"),
		filepath.Join(os.Getenv("HOME"), ".gtkrc-2.0"),
		filepath.Join(os.Getenv("HOME"), ".config", "xfce4", "xfconf", "xfce-perchannel-xml", "xsettings.xml"),
	}
	keys := map[string]string{
		"gtk-theme-name":        "theme",
		"gtk-icon-theme-name":   "icons",
		"gtk-font-name":         "font",
		"gtk-cursor-theme-name": "cursor",
	}
	found := map[string]string{}
	for _, p := range paths {
		if strings.TrimSpace(p) == "" {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		values := readConfigValues(string(b))
		for k := range keys {
			if _, ok := found[k]; ok {
				continue
			}
			if v := values[k]; v != "" {
				found[k] = v
			}
		}
		if len(found) == len(keys) {
			break
		}
	}
	out := []string{}
	order := []string{"gtk-theme-name", "gtk-icon-theme-name", "gtk-font-name", "gtk-cursor-theme-name"}
	for _, k := range order {
		if v := strings.TrimSpace(found[k]); v != "" {
			out = append(out, keys[k]+" "+v)
		}
	}
	return out
}

func readConfigValues(content string) map[string]string {
	out := map[string]string{}
	stack := make([]string, 0, 8)
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		// xfconf xml style can be nested:
		// <property name="Net"><property name="ThemeName" value="Adwaita-dark"/>
		// also handle flattened names like Net/ThemeName or /Net/ThemeName.
		if strings.Contains(line, "<property") {
			rest := line
			for {
				i := strings.Index(rest, "<property")
				if i < 0 {
					break
				}
				rest = rest[i:]
				j := strings.Index(rest, ">")
				if j < 0 {
					break
				}
				tag := rest[:j+1]
				rest = rest[j+1:]

				name := trimQuotes(extractTagAttr(tag, "name"))
				value := trimQuotes(extractTagAttr(tag, "value"))
				selfClosing := strings.Contains(tag, "/>")
				if name == "" {
					continue
				}
				name = strings.TrimPrefix(name, "/")
				stack = append(stack, name)
				path := strings.ToLower(strings.Join(stack, "/"))
				setThemeValueFromPath(out, path, name, value)
				if selfClosing && len(stack) > 0 {
					stack = stack[:len(stack)-1]
				}
			}
		}
		if closeCount := strings.Count(line, "</property>"); closeCount > 0 {
			for i := 0; i < closeCount && len(stack) > 0; i++ {
				stack = stack[:len(stack)-1]
			}
		}

		// ini/gtkrc style: key=value
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(parts[0]))
		if k == "gtk-theme-name" || k == "gtk-icon-theme-name" || k == "gtk-font-name" || k == "gtk-cursor-theme-name" {
			if _, ok := out[k]; !ok {
				out[k] = trimQuotes(parts[1])
			}
		}
	}
	return out
}

func setThemeValueFromPath(out map[string]string, path string, name string, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	nameLower := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.Contains(path, "net/themename") || nameLower == "net/themename":
		if _, ok := out["gtk-theme-name"]; !ok {
			out["gtk-theme-name"] = value
		}
	case strings.Contains(path, "net/iconthemename") || nameLower == "net/iconthemename":
		if _, ok := out["gtk-icon-theme-name"]; !ok {
			out["gtk-icon-theme-name"] = value
		}
	case strings.Contains(path, "gtk/fontname") || nameLower == "gtk/fontname":
		if _, ok := out["gtk-font-name"]; !ok {
			out["gtk-font-name"] = value
		}
	case strings.Contains(path, "gtk/cursorthemename") || nameLower == "gtk/cursorthemename":
		if _, ok := out["gtk-cursor-theme-name"]; !ok {
			out["gtk-cursor-theme-name"] = value
		}
	}
}

func extractTagAttr(tag string, attr string) string {
	needle := attr + "=\""
	i := strings.Index(tag, needle)
	if i < 0 {
		return ""
	}
	start := i + len(needle)
	j := strings.Index(tag[start:], "\"")
	if j < 0 {
		return ""
	}
	return tag[start : start+j]
}

func linuxThemeFromCommands(fast bool) []string {
	timeout := 350 * time.Millisecond
	if fast {
		timeout = 220 * time.Millisecond
	}
	found := map[string]string{}

	if commandExists("xfconf-query") {
		if v, _ := runCmd(timeout, "xfconf-query", "-c", "xsettings", "-p", "/Net/ThemeName"); v != "" {
			found["theme"] = trimQuotes(v)
		}
		if v, _ := runCmd(timeout, "xfconf-query", "-c", "xsettings", "-p", "/Net/IconThemeName"); v != "" {
			found["icons"] = trimQuotes(v)
		}
		if v, _ := runCmd(timeout, "xfconf-query", "-c", "xsettings", "-p", "/Gtk/FontName"); v != "" {
			found["font"] = trimQuotes(v)
		}
	}

	if commandExists("gsettings") {
		if _, ok := found["theme"]; !ok {
			if v, _ := runCmd(timeout, "gsettings", "get", "org.gnome.desktop.interface", "gtk-theme"); v != "" {
				found["theme"] = trimQuotes(v)
			}
		}
		if _, ok := found["icons"]; !ok {
			if v, _ := runCmd(timeout, "gsettings", "get", "org.gnome.desktop.interface", "icon-theme"); v != "" {
				found["icons"] = trimQuotes(v)
			}
		}
		if _, ok := found["font"]; !ok {
			if v, _ := runCmd(timeout, "gsettings", "get", "org.gnome.desktop.interface", "font-name"); v != "" {
				found["font"] = trimQuotes(v)
			}
		}
	}

	out := []string{}
	if v := strings.TrimSpace(found["theme"]); v != "" {
		out = append(out, "theme "+v)
	}
	if v := strings.TrimSpace(found["icons"]); v != "" {
		out = append(out, "icons "+v)
	}
	if v := strings.TrimSpace(found["font"]); v != "" {
		out = append(out, "font "+v)
	}
	return out
}

func terminalInfo() string {
	if runtime.GOOS == "windows" {
		if os.Getenv("WT_SESSION") != "" {
			return "Windows Terminal"
		}
		if os.Getenv("TERM_PROGRAM") != "" {
			return os.Getenv("TERM_PROGRAM")
		}
		return "Console Host"
	}
	if v := os.Getenv("TERM_PROGRAM"); v != "" {
		if vv := os.Getenv("TERM_PROGRAM_VERSION"); vv != "" {
			return v + " " + vv
		}
		return v
	}
	if v := os.Getenv("TERMINAL_EMULATOR"); v != "" {
		return filepath.Base(v)
	}
	if v := os.Getenv("TERM"); v != "" {
		return v
	}
	return "unknown"
}

func cpuInfo(fast bool) string {
	switch runtime.GOOS {
	case "linux":
		model := ""
		freq := ""
		f, err := os.Open("/proc/cpuinfo")
		if err == nil {
			defer f.Close()
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := scanner.Text()
				if model == "" && strings.HasPrefix(line, "model name") {
					parts := strings.SplitN(line, ":", 2)
					if len(parts) == 2 {
						model = strings.TrimSpace(parts[1])
					}
				}
				if freq == "" && strings.HasPrefix(line, "cpu MHz") {
					parts := strings.SplitN(line, ":", 2)
					if len(parts) == 2 {
						if mhz, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64); err == nil {
							freq = fmt.Sprintf("%.2fGHz", mhz/1000.0)
						}
					}
				}
				if model != "" && freq != "" {
					break
				}
			}
		}
		if model != "" {
			if freq != "" {
				return fmt.Sprintf("%s (%d cores, %s)", model, runtime.NumCPU(), freq)
			}
			return fmt.Sprintf("%s (%d cores)", model, runtime.NumCPU())
		}
	case "darwin":
		model, _ := runCmd(220*time.Millisecond, "sysctl", "-n", "machdep.cpu.brand_string")
		cores, _ := runCmd(220*time.Millisecond, "sysctl", "-n", "hw.ncpu")
		freqHz, _ := runCmd(220*time.Millisecond, "sysctl", "-n", "hw.cpufrequency")
		if model != "" {
			if ghz := hzToGHz(freqHz); ghz != "" {
				return fmt.Sprintf("%s (%s cores, %s)", model, strings.TrimSpace(cores), ghz)
			}
			if cores != "" {
				return fmt.Sprintf("%s (%s cores)", model, strings.TrimSpace(cores))
			}
			return model
		}
	case "windows":
		if wf := windowsFacts(); wf.Valid {
			if wf.CPUName != "" {
				cores := wf.CPUCores
				if cores <= 0 {
					cores = runtime.NumCPU()
				}
				if wf.CPUMHz > 0 {
					return fmt.Sprintf("%s (%d cores, %.2fGHz)", wf.CPUName, cores, float64(wf.CPUMHz)/1000.0)
				}
				return fmt.Sprintf("%s (%d cores)", wf.CPUName, cores)
			}
		}
		if fast || !windowsSlowProbesEnabled() {
			return fmt.Sprintf("%d cores", runtime.NumCPU())
		}
		ps := "$c=Get-CimInstance Win32_Processor | Select-Object -First 1; if($c){\"$($c.Name) ($($c.NumberOfCores) cores, $([math]::Round($c.MaxClockSpeed/1000,2))GHz)\"}"
		if out, _ := runCmd(1800*time.Millisecond, "powershell", "-NoProfile", "-Command", ps); out != "" {
			return out
		}
	}
	if !fast {
		if out, _ := runCmd(200*time.Millisecond, "uname", "-p"); out != "" {
			return fmt.Sprintf("%s (%d cores)", out, runtime.NumCPU())
		}
	}
	return fmt.Sprintf("%d cores", runtime.NumCPU())
}

func gpuInfo(fast bool) string {
	switch runtime.GOOS {
	case "linux":
		if fast {
			if v := fastLinuxGPU(); v != "" {
				return v
			}
			return "unknown"
		}
		if commandExists("lspci") {
			if out, _ := runCmd(400*time.Millisecond, "lspci"); out != "" {
				var gpus []string
				for _, line := range strings.Split(out, "\n") {
					lo := strings.ToLower(line)
					if strings.Contains(lo, "vga") || strings.Contains(lo, "3d controller") || strings.Contains(lo, "display controller") {
						parts := strings.SplitN(line, ":", 3)
						if len(parts) >= 3 {
							gpus = append(gpus, strings.TrimSpace(parts[2]))
						} else {
							gpus = append(gpus, strings.TrimSpace(line))
						}
					}
				}
				if len(gpus) > 0 {
					return strings.Join(unique(gpus), " | ")
				}
			}
		}
	case "darwin":
		if out, _ := runCmd(1000*time.Millisecond, "system_profiler", "SPDisplaysDataType"); out != "" {
			lines := strings.Split(out, "\n")
			var gpus []string
			for _, l := range lines {
				t := strings.TrimSpace(l)
				if strings.HasPrefix(t, "Chipset Model:") || strings.HasPrefix(t, "Graphics:") {
					gpus = append(gpus, strings.TrimSpace(strings.SplitN(t, ":", 2)[1]))
				}
			}
			if len(gpus) > 0 {
				return strings.Join(unique(gpus), " | ")
			}
		}
	case "windows":
		if wf := windowsFacts(); wf.Valid && len(wf.GPUNames) > 0 {
			return strings.Join(unique(wf.GPUNames), " | ")
		}
		if out := windowsGPUFromCommands(fast); out != "" {
			return out
		}
		if fast || !windowsSlowProbesEnabled() {
			return "unknown"
		}
		ps := "(Get-CimInstance Win32_VideoController | Select-Object -ExpandProperty Name) -join ' | '"
		if out, _ := runCmd(1800*time.Millisecond, "powershell", "-NoProfile", "-Command", ps); out != "" {
			return out
		}
	}
	if fast {
		return "unknown"
	}
	return "unknown"
}

func windowsGPUFromCommands(fast bool) string {
	if commandExists("wmic") {
		timeout := 1200 * time.Millisecond
		if fast {
			timeout = 700 * time.Millisecond
		}
		out, _ := runCmd(timeout, "wmic", "path", "win32_VideoController", "get", "Name")
		if strings.TrimSpace(out) != "" {
			names := make([]string, 0, 4)
			for _, line := range strings.Split(out, "\n") {
				l := strings.TrimSpace(line)
				if l == "" || strings.EqualFold(l, "Name") {
					continue
				}
				names = append(names, l)
			}
			if len(names) > 0 {
				return strings.Join(unique(names), " | ")
			}
		}
	}

	timeout := 1800 * time.Millisecond
	if fast {
		timeout = 900 * time.Millisecond
	}
	ps := "(Get-CimInstance Win32_VideoController | Select-Object -ExpandProperty Name) -join ' | '"
	if out, _ := runCmd(timeout, "powershell", "-NoProfile", "-Command", ps); strings.TrimSpace(out) != "" {
		return strings.TrimSpace(out)
	}
	return ""
}

func memoryInfo() string {
	switch runtime.GOOS {
	case "linux":
		var totalKB, availKB int64
		b, err := os.ReadFile("/proc/meminfo")
		if err == nil {
			for _, line := range strings.Split(string(b), "\n") {
				if strings.HasPrefix(line, "MemTotal:") {
					totalKB = parseMeminfoKB(line)
				}
				if strings.HasPrefix(line, "MemAvailable:") {
					availKB = parseMeminfoKB(line)
				}
			}
		}
		if totalKB > 0 && availKB >= 0 {
			used := totalKB - availKB
			return fmt.Sprintf("%s / %s", formatBytes(used*1024), formatBytes(totalKB*1024))
		}
	case "darwin":
		total, _ := runCmd(200*time.Millisecond, "sysctl", "-n", "hw.memsize")
		vm, _ := runCmd(250*time.Millisecond, "vm_stat")
		if total != "" {
			totalB, _ := strconv.ParseInt(strings.TrimSpace(total), 10, 64)
			usedB := parseVMStatUsed(vm)
			if totalB > 0 && usedB > 0 {
				return fmt.Sprintf("%s / %s", formatBytes(usedB), formatBytes(totalB))
			}
			if totalB > 0 {
				return formatBytes(totalB)
			}
		}
	case "windows":
		if wf := windowsFacts(); wf.Valid && wf.MemTotalKB > 0 {
			used := (wf.MemTotalKB - wf.MemFreeKB) * 1024
			total := wf.MemTotalKB * 1024
			if total > 0 {
				return fmt.Sprintf("%s / %s", formatBytes(used), formatBytes(total))
			}
		}
		if total, avail, ok := windowsMemoryAPI(); ok && total > 0 {
			used := total - avail
			return fmt.Sprintf("%s / %s", formatBytes(used), formatBytes(total))
		}
		if fastCollectMode.Load() || !windowsSlowProbesEnabled() {
			return "unknown"
		}
		ps := "$o=Get-CimInstance Win32_OperatingSystem; $total=$o.TotalVisibleMemorySize*1KB; $free=$o.FreePhysicalMemory*1KB; $used=$total-$free; \"$used,$total\""
		if out, _ := runCmd(1800*time.Millisecond, "powershell", "-NoProfile", "-Command", ps); out != "" {
			parts := strings.Split(strings.TrimSpace(out), ",")
			if len(parts) == 2 {
				used, _ := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
				total, _ := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
				if total > 0 {
					return fmt.Sprintf("%s / %s", formatBytes(used), formatBytes(total))
				}
			}
		}
	}
	return "unknown"
}

func swapUsageSummary() string {
	switch runtime.GOOS {
	case "windows":
		if used, total, ok := windowsSwapUsage(); ok && total > 0 {
			pct := int64(0)
			if total > 0 {
				pct = (used * 100) / total
			}
			return fmt.Sprintf("%s / %s (%d%%)", formatGiB2(used), formatGiB2(total), pct)
		}
		return "n/a"
	case "linux":
	default:
		return "n/a"
	}
	var totalKB, freeKB int64
	if b, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, "SwapTotal:") {
				totalKB = parseMeminfoKB(line)
			}
			if strings.HasPrefix(line, "SwapFree:") {
				freeKB = parseMeminfoKB(line)
			}
		}
	}
	if totalKB <= 0 {
		return "n/a"
	}
	used := totalKB - freeKB
	pct := int64(0)
	if totalKB > 0 {
		pct = (used * 100) / totalKB
	}
	return fmt.Sprintf("%s / %s (%d%%)", formatGiB2(used*1024), formatGiB2(totalKB*1024), pct)
}

func diskRootUsageDetailed() string {
	if runtime.GOOS == "linux" && !linuxHasMultiplePhysicalDisks() {
		total, free, err := diskUsage("/")
		if err != nil {
			return "unknown"
		}
		used := total - free
		pct := int64(0)
		if total > 0 {
			pct = (used * 100) / total
		}
		fs := rootFSType()
		if fs == "" {
			fs = "unknown"
		}
		return fmt.Sprintf("%s / %s (%d%%) - %s", formatGiB2(used), formatGiB2(total), pct, fs)
	}

	if usedAll, totalAll, diskCount, fsAll, ok := aggregateLocalDiskUsage(); ok && diskCount > 1 {
		pctAll := int64(0)
		if totalAll > 0 {
			pctAll = (usedAll * 100) / totalAll
		}
		if fsAll == "" {
			fsAll = "unknown"
		}
		return fmt.Sprintf(
			"%s / %s (%d%%) - %s (%d disks)",
			formatGiB2(usedAll),
			formatGiB2(totalAll),
			pctAll,
			fsAll,
			diskCount,
		)
	}

	total, free, err := diskUsage("/")
	if err != nil {
		return "unknown"
	}
	used := total - free
	pct := int64(0)
	if total > 0 {
		pct = (used * 100) / total
	}
	fs := rootFSType()
	if fs == "" {
		fs = "unknown"
	}
	return fmt.Sprintf("%s / %s (%d%%) - %s", formatGiB2(used), formatGiB2(total), pct, fs)
}

func fastMacPackageCount() int {
	entries, err := os.ReadDir("/var/db/receipts")
	if err != nil {
		return 0
	}
	seen := map[string]struct{}{}
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name())
		if name == "" || entry.IsDir() {
			continue
		}
		switch filepath.Ext(name) {
		case ".plist", ".bom":
			base := strings.TrimSuffix(name, filepath.Ext(name))
			if base != "" {
				seen[base] = struct{}{}
			}
		}
	}
	return len(seen)
}

func fastMacBrewCount() int {
	for _, cellar := range []string{"/opt/homebrew/Cellar", "/usr/local/Cellar"} {
		entries, err := os.ReadDir(cellar)
		if err != nil {
			continue
		}
		count := 0
		for _, entry := range entries {
			if entry.IsDir() {
				count++
			}
		}
		if count > 0 {
			return count
		}
	}
	return 0
}

func windowsSwapUsage() (used int64, total int64, ok bool) {
	if wf := windowsFacts(); wf.Valid && wf.PageTotalMB > 0 {
		total = wf.PageTotalMB * 1024 * 1024
		used = wf.PageUsedMB * 1024 * 1024
		if used < 0 {
			used = 0
		}
		if used > total {
			used = total
		}
		return used, total, true
	}

	if commandExists("wmic") {
		out, _ := runCmd(1200*time.Millisecond, "wmic", "pagefile", "list", "/format:value")
		if used, total, ok := parseWindowsPagefileWMIC(out); ok {
			return used, total, true
		}
	}

	ps := "$items=Get-CimInstance Win32_PageFileUsage; if($items){$total=($items|Measure-Object -Property AllocatedBaseSize -Sum).Sum; $used=($items|Measure-Object -Property CurrentUsage -Sum).Sum; \"$used,$total\"}"
	out, _ := runCmd(1600*time.Millisecond, "powershell", "-NoProfile", "-Command", ps)
	if strings.TrimSpace(out) == "" {
		return 0, 0, false
	}
	parts := strings.Split(strings.TrimSpace(out), ",")
	if len(parts) != 2 {
		return 0, 0, false
	}
	usedMB, errUsed := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	totalMB, errTotal := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if errUsed != nil || errTotal != nil || totalMB <= 0 {
		return 0, 0, false
	}
	return usedMB * 1024 * 1024, totalMB * 1024 * 1024, true
}

func parseWindowsPagefileWMIC(out string) (used int64, total int64, ok bool) {
	if strings.TrimSpace(out) == "" {
		return 0, 0, false
	}
	var usedMB int64
	var totalMB int64
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "AllocatedBaseSize=") {
			v, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "AllocatedBaseSize=")), 10, 64)
			if err == nil && v > 0 {
				totalMB += v
			}
		}
		if strings.HasPrefix(line, "CurrentUsage=") {
			v, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "CurrentUsage=")), 10, 64)
			if err == nil && v >= 0 {
				usedMB += v
			}
		}
	}
	if totalMB <= 0 {
		return 0, 0, false
	}
	return usedMB * 1024 * 1024, totalMB * 1024 * 1024, true
}

func aggregateLocalDiskUsage() (used int64, total int64, diskCount int, fsSummary string, ok bool) {
	switch runtime.GOOS {
	case "linux":
		b, err := os.ReadFile("/proc/mounts")
		if err != nil {
			return 0, 0, 0, "", false
		}

		type mountInfo struct {
			mountPoint string
			fsType     string
		}
		devices := map[string]mountInfo{}

		for _, line := range strings.Split(string(b), "\n") {
			f := strings.Fields(line)
			if len(f) < 3 {
				continue
			}
			src := strings.TrimSpace(f[0])
			mnt := strings.TrimSpace(f[1])
			fs := strings.TrimSpace(f[2])

			if !strings.HasPrefix(src, "/dev/") {
				continue
			}
			if strings.HasPrefix(src, "/dev/loop") || strings.HasPrefix(src, "/dev/zram") {
				continue
			}
			if mnt == "" {
				continue
			}
			if _, exists := devices[src]; exists {
				continue
			}
			devices[src] = mountInfo{mountPoint: mnt, fsType: fs}
		}

		if len(devices) == 0 {
			return 0, 0, 0, "", false
		}

		fsKinds := map[string]int{}
		for _, item := range devices {
			t, f, err := diskUsage(item.mountPoint)
			if err != nil || t <= 0 {
				continue
			}
			total += t
			used += (t - f)
			diskCount++
			if item.fsType != "" {
				fsKinds[item.fsType]++
			}
		}
		if diskCount == 0 || total <= 0 {
			return 0, 0, 0, "", false
		}
		return used, total, diskCount, dominantFSType(fsKinds), true

	case "darwin":
		out, _ := runCmd(300*time.Millisecond, "df", "-kP")
		if strings.TrimSpace(out) == "" {
			return 0, 0, 0, "", false
		}
		deviceMount := map[string]string{}
		for _, line := range strings.Split(out, "\n") {
			f := strings.Fields(line)
			if len(f) < 6 {
				continue
			}
			src := strings.TrimSpace(f[0])
			mnt := strings.TrimSpace(f[len(f)-1])
			if !strings.HasPrefix(src, "/dev/disk") || mnt == "" {
				continue
			}
			if _, exists := deviceMount[src]; !exists {
				deviceMount[src] = mnt
			}
		}
		if len(deviceMount) == 0 {
			return 0, 0, 0, "", false
		}
		for _, mnt := range deviceMount {
			t, f, err := diskUsage(mnt)
			if err != nil || t <= 0 {
				continue
			}
			total += t
			used += (t - f)
			diskCount++
		}
		if diskCount == 0 || total <= 0 {
			return 0, 0, 0, "", false
		}
		return used, total, diskCount, "apfs", true
	}

	return 0, 0, 0, "", false
}

func linuxHasMultiplePhysicalDisks() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	entries, err := os.ReadDir("/sys/block")
	if err != nil {
		return true
	}
	count := 0
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name())
		if name == "" {
			continue
		}
		if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") || strings.HasPrefix(name, "zram") ||
			strings.HasPrefix(name, "fd") || strings.HasPrefix(name, "sr") || strings.HasPrefix(name, "dm-") {
			continue
		}
		count++
		if count > 1 {
			return true
		}
	}
	return false
}

func dominantFSType(kinds map[string]int) string {
	best := ""
	bestCount := -1
	for fs, c := range kinds {
		if c > bestCount || (c == bestCount && fs < best) {
			best = fs
			bestCount = c
		}
	}
	return best
}

func localIPSummary() string {
	if runtime.GOOS == "linux" && fastCollectMode.Load() {
		if v := localIPFromDefaultRoute(); v != "" {
			return v
		}
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return "disconnected"
	}
	for _, iface := range ifaces {
		if isVirtualInterface(iface.Name) {
			continue
		}
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP == nil || ipNet.IP.IsLoopback() {
				continue
			}
			ip4 := ipNet.IP.To4()
			if ip4 == nil {
				continue
			}
			ones, _ := ipNet.Mask.Size()
			return fmt.Sprintf("(%s): %s/%d", iface.Name, ip4.String(), ones)
		}
	}
	return "disconnected"
}

func localIPFromDefaultRoute() string {
	b, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return ""
	}
	lines := strings.Split(string(b), "\n")
	for _, line := range lines[1:] {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		ifaceName := strings.TrimSpace(f[0])
		destHex := strings.TrimSpace(f[1])
		if ifaceName == "" || destHex != "00000000" || isVirtualInterface(ifaceName) {
			continue
		}
		iface, err := net.InterfaceByName(ifaceName)
		if err != nil {
			continue
		}
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP == nil || ipNet.IP.IsLoopback() {
				continue
			}
			ip4 := ipNet.IP.To4()
			if ip4 == nil {
				continue
			}
			ones, _ := ipNet.Mask.Size()
			return fmt.Sprintf("(%s): %s/%d", iface.Name, ip4.String(), ones)
		}
	}
	return ""
}

func isVirtualInterface(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return false
	}
	for _, p := range []string{"docker", "veth", "br-", "virbr", "vmnet", "vboxnet", "zt", "tailscale", "wg"} {
		if strings.HasPrefix(n, p) {
			return true
		}
	}
	return false
}

func rootFSType() string {
	return rootFSTypeOS("/")
}

func formatGiB2(b int64) string {
	if b < 0 {
		b = 0
	}
	return fmt.Sprintf("%.2f GiB", float64(b)/float64(1024*1024*1024))
}
