# dawnfetch

a fast, themed, cross-platform system info CLI inspired by neofetch and fastfetch.

[![release build](https://img.shields.io/github/actions/workflow/status/almightynan/dawnfetch/release-build.yml?label=release%20build)](https://github.com/almightynan/dawnfetch/actions/workflows/release-build.yml)
[![latest release](https://img.shields.io/github/v/release/almightynan/dawnfetch?display_name=tag)](https://github.com/almightynan/dawnfetch/releases)
![go](https://img.shields.io/badge/go-1.20+-00ADD8?logo=go&logoColor=white)
[![stars](https://img.shields.io/github/stars/almightynan/dawnfetch)](https://github.com/almightynan/dawnfetch/stargazers)

Quick links: [Install](#install) • [Build](#build) • [Benchmark](#benchmark-100-runs) • [Showcase](#showcase)

![](assets/dawnfetch_debian12.png)

Logo credits: all distro ASCII logos in `ascii/` are sourced from the [fastfetch](https://github.com/fastfetch-cli/fastfetch) project. Credit goes to the fastfetch contributors.

## Install

Choose one method:

1. Linux/macOS (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/almightynan/dawnfetch/main/cli/install.sh | bash
```

2. Windows (PowerShell)

```powershell
powershell -c "irm https://raw.githubusercontent.com/almightynan/dawnfetch/main/cli/install.ps1 | iex"
```

3. Windows 7 / old PowerShell fallback

```powershell
$f="$env:TEMP\dawnfetch-install.ps1";(New-Object Net.WebClient).DownloadFile('https://raw.githubusercontent.com/almightynan/dawnfetch/main/cli/install.ps1',$f);powershell -File $f
```

4. npm

```bash
npm i -g dawnfetch
```

After npm install (if command is not recognized):

- Linux (current shell): `source ~/.bashrc` or `source ~/.zshrc`
- Windows PowerShell (current window):

```powershell
$env:Path=[Environment]::GetEnvironmentVariable("Path","User")+";"+[Environment]::GetEnvironmentVariable("Path","Machine")
```

- Or simply close and reopen the terminal.

If your system is on older Node/npm (example: Node 12 / npm 6), prefer direct install instead of npm:

```powershell
powershell -c "irm https://raw.githubusercontent.com/almightynan/dawnfetch/main/cli/install.ps1 | iex"
```

If you use bun and see `Blocked postinstall`, allow trusted scripts and reinstall:

```bash
bun pm -g untrusted
bun remove -g dawnfetch && bun add -g dawnfetch
```


To verify installation run:

```bash
dawnfetch --version
dawnfetch
```

Default per-user config paths:
- Linux: `~/.config/dawnfetch/dawnfetch_config.json`
- macOS: `~/.config/dawnfetch/dawnfetch_config.json`
- Windows: `%AppData%\\dawnfetch\\dawnfetch_config.json`

Portable installs may keep `dawnfetch_config.json` beside the binary.

Example:

```json
{
  "default_theme": "transgender",
  "bold_titles": true
}
```

## Build from source

All build scripts output binaries to the repo root `dist/` directory.

Linux:

```bash
bash build/build-linux.sh
```

macOS:

```bash
bash build/build-macos.sh
```

Windows (PowerShell):

```powershell
powershell -File build/build-windows.ps1
```

Expected outputs:

- `dist/dawnfetch-linux-amd64`
- `dist/dawnfetch-linux-arm64`
- `dist/dawnfetch-linux-386`
- `dist/dawnfetch-macos-amd64`
- `dist/dawnfetch-macos-arm64`
- `dist/dawnfetch-windows-amd64.exe`
- `dist/dawnfetch-windows-386.exe`
- `dist/dawnfetch-windows-arm64.exe`

## Benchmark (100 runs)

Run benchmark:

```bash
python3 bench/benchmark.py --runs 100 --warmup 1
```

Hyperfine results (100 runs):

| Tool | Runs | Mean (ms) | Median (ms) | P95 (ms) | Min (ms) | Max (ms) | StdDev (ms) |
|---|---:|---:|---:|---:|---:|---:|---:|
| dawnfetch | 100 | 7.43 | 6.60 | 7.55 | 6.01 | 42.79 | 4.53 |
| fastfetch | 100 | 11.91 | 10.99 | 14.33 | 10.13 | 52.03 | 5.24 |
| hifetch | 100 | 17.55 | 17.10 | 17.75 | 16.60 | 56.41 | 3.93 |
| macchina | 100 | 100.95 | 75.37 | 221.11 | 48.82 | 578.82 | 71.91 |
| neofetch | 100 | 398.99 | 386.21 | 449.06 | 361.81 | 771.10 | 46.58 |
| screenfetch | 100 | 848.98 | 687.62 | 1374.44 | 640.72 | 6485.31 | 626.92 |

benchmark ran using hyperfine via: [`bench/benchmark.py`](bench/benchmark.py)
