package common

// Config holds window creation parameters.
type Config struct {
	Title               string
	Width, Height       uint32
	MinWidth, MinHeight uint32
}

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

// New creates a platform window and returns it as any.
// The concrete value satisfies window.Window (and vkwin.Window).
func New(opts ...Option) (any, error) {
	cfg := &Config{Title: "Window", Width: 800, Height: 600}
	for _, o := range opts {
		o(cfg)
	}
	return newPlatform(cfg)
}
