package main

import (
	"fmt"
	"log"
	"time"

	vk "github.com/lukem570/vulkan-go/pkg/raw"

	"github.com/Quikcad/quikwin/pkg/window"
)

type surfaceWindow interface {
	NewSurface(instance vk.Instance) (*vk.SurfaceKHR, error)
}

func main() {
	if err := vk.Initialize(); err != nil {
		log.Fatal("vulkan init:", err)
	}

	win, err := window.New(
		window.WithTitle("Surface Test"),
		window.WithSize(800, 600),
	)
	if err != nil {
		log.Fatal("create window:", err)
	}
	defer win.Destroy()

	inst, err := vk.CreateInstance(&vk.InstanceCreateInfo{
		ApplicationInfo: &vk.ApplicationInfo{
			ApplicationName: "surface-test",
			ApiVersion:      apiVersion(1, 2, 0),
		},
		EnabledExtensionNames: platformExts(win),
	}, nil)
	if err != nil {
		log.Fatal("create instance:", err)
	}
	defer inst.Destroy(nil)

	sw, ok := win.(surfaceWindow)
	if !ok {
		log.Fatal("window does not support Vulkan surface")
	}
	surf, err := sw.NewSurface(*inst)
	if err != nil {
		log.Fatal("create surface:", err)
	}
	defer inst.DestroySurfaceKHR(surf, nil)

	fmt.Println("surface created OK")

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) && !win.ShouldClose() {
		win.PollEvents()
	}

	fmt.Println("surface test passed")
}

// platformExts returns the instance extensions required for the current display server.
func platformExts(win window.Window) []string {
	exts := []string{"VK_KHR_surface"}
	switch {
	case implementsWayland(win):
		exts = append(exts, "VK_KHR_wayland_surface")
	case implementsX11(win):
		exts = append(exts, "VK_KHR_xlib_surface")
	case implementsWin32(win):
		exts = append(exts, "VK_KHR_win32_surface")
	case implementsCocoa(win):
		exts = append(exts, "VK_MVK_macos_surface")
	}
	return exts
}

func implementsWayland(w any) bool {
	_, ok := w.(interface{ WlDisplay() (uintptr, error) })
	return ok
}
func implementsX11(w any) bool {
	_, ok := w.(interface{ Display() (uintptr, error) })
	return ok
}
func implementsWin32(w any) bool {
	_, ok := w.(interface{ HWND() (uintptr, error) })
	return ok
}
func implementsCocoa(w any) bool {
	_, ok := w.(interface{ SetTitlebarStyle(uint8) })
	return ok
}

func apiVersion(major, minor, patch uint32) uint32 {
	return (major << 22) | (minor << 12) | patch
}
