//go:build linux

package common

import (
	"os"

	"github.com/Quikcad/quikwin/internal/platform/wayland"
	"github.com/Quikcad/quikwin/internal/platform/x11"
)

func newPlatform(cfg *Config) (any, error) {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return wayland.New(cfg)
	}
	return x11.New(cfg)
}
