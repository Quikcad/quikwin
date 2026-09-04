package common

import "github.com/Quikcad/quikwin/internal/platform/wtypes"

// Config is an alias for wtypes.Config so callers can use common.Config.
type Config = wtypes.Config

// Option mutates a Config.
type Option func(*Config)

func WithTitle(title string) Option {
	return func(c *Config) { c.Title = title }
}

func WithSize(w, h uint32) Option {
	return func(c *Config) { c.Width = w; c.Height = h }
}

func WithMinSize(w, h uint32) Option {
	return func(c *Config) { c.MinWidth = w; c.MinHeight = h }
}

func WithResizable(resizable bool) Option {
	return func(c *Config) { c.Resizable = resizable }
}

func WithBorder(border bool) Option {
	return func(c *Config) { c.Border = border }
}

func WithTitlebar(titlebar bool) Option {
	return func(c *Config) { c.Titlebar = titlebar }
}

func WithCentered(centered bool) Option {
	return func(c *Config) { c.Centered = centered }
}

// New creates a platform window and returns it as any.
// The concrete value satisfies window.Window (and vkwin.Window).
func New(opts ...Option) (any, error) {
	cfg := &Config{
		Title:     "Window",
		Width:     800,
		Height:    600,
		Resizable: true,
		Border:    true,
		Titlebar:  true,
	}
	for _, o := range opts {
		o(cfg)
	}
	return newPlatform(cfg)
}
