//go:build windows

package common

import "github.com/Quikcad/quikwin/internal/win32"

func newPlatform(cfg *Config) (any, error) {
	return win32.New(cfg.Title, cfg.Width, cfg.Height, cfg.MinWidth, cfg.MinHeight)
}
