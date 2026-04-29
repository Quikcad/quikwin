package window

import "github.com/Quikcad/quikwin/internal/common"

// Option configures a new window.
type Option = common.Option

func WithTitle(title string) Option    { return common.WithTitle(title) }
func WithSize(w, h uint32) Option      { return common.WithSize(w, h) }
func WithMinSize(w, h uint32) Option   { return common.WithMinSize(w, h) }

// New creates a native window using the current platform's backend.
// The returned value also satisfies vkwin.Window.
func New(opts ...Option) (Window, error) {
	w, err := common.New(opts...)
	if err != nil {
		return nil, err
	}
	return w.(Window), nil
}
