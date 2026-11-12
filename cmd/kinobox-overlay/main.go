// Command kinobox-overlay shows the riders' status in a borderless strip
// across the top (or bottom) of the screen, on top of a video player. It reads
// the data from a running kinobox server.
package main

import (
	"context"
	"errors"
	"flag"
	"image/png"
	"io/fs"
	"log/slog"
	"os"
	"time"

	"kinobox/internal/config"
	"kinobox/internal/overlay"
)

const defaultConfig = "kinobox.toml"

// version is set at release build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	configPath := flag.String("config", defaultConfig, "path to the TOML configuration file ([overlay] section)")
	debug := flag.Bool("debug", false, "log at debug level")
	preview := flag.String("preview", "", "render one frame to this PNG file instead of opening a window")
	previewWidth := flag.Int("preview-width", 1920, "width of the -preview image in pixels")
	flag.Parse()

	if *debug {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}
	cfg, err := loadConfig(*configPath)
	if err != nil {
		slog.Error("configuration", "err", err)
		os.Exit(2)
	}
	oc := cfg.Overlay

	renderer, err := overlay.NewRenderer(oc)
	if err != nil {
		slog.Error("renderer", "err", err)
		os.Exit(2)
	}
	client := overlay.NewClient(oc.Server, oc.PollInterval)

	if *preview != "" {
		if err := renderPreview(client, renderer, oc, *preview, *previewWidth); err != nil {
			slog.Error("preview", "err", err)
			os.Exit(1)
		}
		return
	}

	slog.Info("starting overlay", "version", version, "server", oc.Server, "position", oc.Position, "height", oc.Height)
	if err := runWindow(oc, client, renderer); err != nil {
		slog.Error("window", "err", err)
		os.Exit(1)
	}
}

// renderPreview fetches the current state once and writes the strip as PNG,
// which is handy for trying out colours without a display.
func renderPreview(client *overlay.Client, r *overlay.Renderer, oc config.Overlay, path string, width int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go client.Run(ctx)
	var st overlay.State
	for v := uint64(0); v == 0 && ctx.Err() == nil; {
		time.Sleep(20 * time.Millisecond)
		st, v = client.State()
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := png.Encode(f, r.Render(st, width, oc.Height)); err != nil {
		return err
	}
	slog.Info("preview written", "path", path, "server_error", st.Err)
	return f.Close()
}

// loadConfig reads the configuration file. A missing file is only an error if
// it was named explicitly; otherwise the defaults are used.
func loadConfig(path string) (config.Config, error) {
	explicit := false
	flag.Visit(func(f *flag.Flag) { explicit = explicit || f.Name == "config" })

	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) && !explicit {
		slog.Info("no configuration file, using defaults", "path", path)
		return config.Default(), nil
	}
	cfg, err := config.Load(path)
	if err == nil {
		slog.Info("configuration loaded", "path", path)
	}
	return cfg, err
}
