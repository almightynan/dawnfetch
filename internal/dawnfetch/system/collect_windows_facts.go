// this file collects a live windows snapshot used by multiple fields.
package system

import (
	"encoding/json"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type windowsFactsData struct {
	Caption           string   `json:"caption"`
	Version           string   `json:"version"`
	Build             string   `json:"build"`
	LastBoot          string   `json:"lastBoot"`
	Manufacturer      string   `json:"manufacturer"`
	Model             string   `json:"model"`
	Architecture      string   `json:"arch"`
	CPUName           string   `json:"cpuName"`
	CPUCores          int      `json:"cpuCores"`
	CPUThreads        int      `json:"cpuThreads"`
	CPUMHz            int      `json:"cpuMHz"`
	GPUNames          []string `json:"gpuNames"`
	ResX              int      `json:"resX"`
	ResY              int      `json:"resY"`
	ResHz             int      `json:"resHz"`
	MemTotalKB        int64    `json:"memTotalKB"`
	MemFreeKB         int64    `json:"memFreeKB"`
	PageTotalMB       int64    `json:"pageTotalMB"`
	PageUsedMB        int64    `json:"pageUsedMB"`
	DiskTotalB        int64    `json:"diskTotalB"`
	DiskFreeB         int64    `json:"diskFreeB"`
	DiskFS            string   `json:"diskFS"`
	Locale            string   `json:"locale"`
	AppsUseLightTheme int      `json:"theme"`
	Valid             bool     `json:"-"`
}

func windowsFacts() windowsFactsData {
	if runtime.GOOS != "windows" {
		return windowsFactsData{}
	}
	if !windowsSlowProbesEnabled() {
		return windowsFactsData{}
	}
	ps := "$ErrorActionPreference='SilentlyContinue'; " +
		"$os=Get-CimInstance Win32_OperatingSystem; " +
		"$cs=Get-CimInstance Win32_ComputerSystem; " +
		"$cpu=Get-CimInstance Win32_Processor | Select-Object -First 1; " +
		"$gpu=Get-CimInstance Win32_VideoController; " +
		"$page=Get-CimInstance Win32_PageFileUsage; " +
		"$sysDrive=$env:SystemDrive; if([string]::IsNullOrWhiteSpace($sysDrive)){$sysDrive='C:'}; " +
		"$drive=Get-CimInstance Win32_LogicalDisk -Filter (\"DeviceID='\"+$sysDrive+\"'\"); " +
		"$theme=(Get-ItemProperty -Path 'HKCU:\\Software\\Microsoft\\Windows\\CurrentVersion\\Themes\\Personalize' -Name AppsUseLightTheme -ErrorAction SilentlyContinue).AppsUseLightTheme; " +
		"$obj=[ordered]@{caption=[string]$os.Caption;version=[string]$os.Version;build=[string]$os.BuildNumber;lastBoot=[string]$os.LastBootUpTime;manufacturer=[string]$cs.Manufacturer;model=[string]$cs.Model;arch=[string]$os.OSArchitecture;cpuName=[string]$cpu.Name;cpuCores=$(if($cpu.NumberOfCores){[int]$cpu.NumberOfCores}else{0});cpuThreads=$(if($cpu.NumberOfLogicalProcessors){[int]$cpu.NumberOfLogicalProcessors}else{0});cpuMHz=$(if($cpu.MaxClockSpeed){[int]$cpu.MaxClockSpeed}else{0});gpuNames=@($gpu|ForEach-Object{[string]$_.Name}|Where-Object{$_}|Select-Object -Unique);resX=$(if(($gpu|Select-Object -First 1).CurrentHorizontalResolution){[int](($gpu|Select-Object -First 1).CurrentHorizontalResolution)}else{0});resY=$(if(($gpu|Select-Object -First 1).CurrentVerticalResolution){[int](($gpu|Select-Object -First 1).CurrentVerticalResolution)}else{0});resHz=$(if(($gpu|Select-Object -First 1).CurrentRefreshRate){[int](($gpu|Select-Object -First 1).CurrentRefreshRate)}else{0});memTotalKB=$(if($os.TotalVisibleMemorySize){[int64]$os.TotalVisibleMemorySize}else{0});memFreeKB=$(if($os.FreePhysicalMemory){[int64]$os.FreePhysicalMemory}else{0});pageTotalMB=$(if($page){[int64](($page|Measure-Object -Property AllocatedBaseSize -Sum).Sum)}else{0});pageUsedMB=$(if($page){[int64](($page|Measure-Object -Property CurrentUsage -Sum).Sum)}else{0});diskTotalB=$(if($drive.Size){[int64]$drive.Size}else{0});diskFreeB=$(if($drive.FreeSpace){[int64]$drive.FreeSpace}else{0});diskFS=[string]$drive.FileSystem;locale=[string]$os.Locale;theme=$(if($null -eq $theme){-1}else{[int]$theme})}; " +
		"$obj | ConvertTo-Json -Compress -Depth 4"

	out, err := runCmd(2200*time.Millisecond, "powershell", "-NoProfile", "-Command", ps)
	if err != nil || strings.TrimSpace(out) == "" {
		return windowsFactsData{}
	}
	var wf windowsFactsData
	if err := json.Unmarshal([]byte(out), &wf); err != nil {
		return windowsFactsData{}
	}
	if wf.Caption == "" && wf.Version == "" && wf.Model == "" {
		return windowsFactsData{}
	}
	wf.Valid = true
	return wf
}

func parseWindowsBootTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	// wmi/dmtf format example: 20260214120333.123456+330
	if len(s) >= 22 && s[14] == '.' {
		base := s[:14]
		micros := s[15:21]
		sign := s[21:22]
		offsetMin := 0
		if len(s) >= 25 {
			if v, err := strconv.Atoi(s[22:25]); err == nil {
				offsetMin = v
			}
		}
		if t, err := time.Parse("20060102150405", base); err == nil {
			if us, err := strconv.Atoi(micros); err == nil {
				t = t.Add(time.Duration(us) * time.Microsecond)
			}
			if sign == "-" {
				return t.Add(time.Duration(offsetMin) * time.Minute)
			}
			return t.Add(-time.Duration(offsetMin) * time.Minute)
		}
	}
	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"20060102150405.000000-0700",
		"20060102150405.000000+000",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
