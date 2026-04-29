//go:build darwin

package common

import "github.com/Quikcad/quikwin/internal/cocoa"

func newPlatform(cfg *Config) (any, error) {
	return cocoa.New(cfg.Title, cfg.Width, cfg.Height, cfg.MinWidth, cfg.MinHeight)
}
