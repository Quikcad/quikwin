package vkwin

import (
	"github.com/Quikcad/quikwin/pkg/window"
	vk "github.com/lukem570/vulkan-go/pkg/raw"
)

// Window is a window that can produce a Vulkan surface.
// Callers own the returned surface and must destroy it.
type Window interface {
	window.Window
	NewSurface(instance vk.Instance) (*vk.SurfaceKHR, error)
}
