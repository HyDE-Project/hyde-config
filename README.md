# Hyde Configuration Parser (Go Implementation)

A Go-based tool for parsing TOML configuration files for HyDE and Hyprland. This is a direct port of the Python implementation at `~/.local/lib/hyde/parse.config.py`.

## Features

- Parse TOML configuration files into environment variables
- Parse Hyprland-specific sections into Hyprland configuration
- Watch for configuration file changes and automatically reprocess
- Supports exporting environment variables

## Installation

### Prerequisites

- Go 1.20 or later
- Git

### Build from Source

```bash
# Clone the repository
git clone https://github.com/hyde-project/hyde-config.git
cd hyde-config

# Build the application
make build

# Install the application
make install
```

## Usage

```bash
# Run with default settings (daemon mode, export enabled)
hyde-config

# Run once without daemon mode
hyde-config --no-daemon

# Run without exporting environment variables
hyde-config --no-export

# Specify custom file locations
hyde-config --input /path/to/config.toml --env /path/to/env --hypr /path/to/hyprland.conf
```

### Command-Line Options

- `--input`: Path to TOML configuration file (default: `$XDG_CONFIG_HOME/hyde/config.toml`)
- `--env`: Path to output environment variables file (default: `$XDG_STATE_HOME/hyde/config`)
- `--hypr`: Path to output Hyprland configuration file (default: `$XDG_STATE_HOME/hyde/hyprland.conf`)
- `--no-daemon`: Run in one-off mode without watching for changes
- `--no-export`: Disable exporting environment variables

## Configuration File Example

Hyde-config expects a TOML configuration file with environment variables and Hyprland settings:

```toml
"$schema" = "../../.local/share/hyde/schema/config_toml.json"

[wallpaper]
    custom_paths = [
        "$XDG_VIDEOS_DIR/Wallpapers",
    ] # List of paths to search for wallpapers
    filetypes = [ "mp4", "mkv" ] # explicit file types
    multi_display = true # Set to true if you want to use different wallpapers on different displays

[mediaplayer]
    max_length     = 20
    prefix_paused  = "⏸️"
    prefix_playing = "▶️"
    standby_text   = "📻"

[wallbash]
    skip_template = [
        # "waybar",    
    ]

[rofi]
    scale = 9

[rofi.launch]
    scale = 10
    drun_style = "launchpad"

[rofi.theme]
    scale = 5

[rofi.cliphist]
    scale = 8

[rofi.wallpaper]
    scale = 5
```