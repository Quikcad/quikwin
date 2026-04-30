# quikwin

Cross-platform windowing library in pure Go. No CGo — all platform calls go through [goffi](https://github.com/go-webgpu/goffi). Companion to [vulkan-go](https://github.com/lukem570/vulkan-go).

## Platforms

| Platform | Backend | Status |
|---|---|---|
| Linux (Wayland) | `pkg/window/wayland` | ✓ |
| Linux (X11) | `pkg/window/x11` | ✓ |
| Windows | `pkg/window/win32` | ✓ |
| macOS | `pkg/window/cocoa` | ✓ |

## Install

```sh
go get github.com/Quikcad/quikwin
```

Requires `CGO_ENABLED=0` — the library uses goffi for all platform calls.

## Usage

```go
import "github.com/Quikcad/quikwin/pkg/window"

win, err := window.New(
    window.WithTitle("My App"),
    window.WithSize(1280, 720),
)
if err != nil {
    log.Fatal(err)
}
defer win.Destroy()

for !win.ShouldClose() {
    win.PollEvents()
    // render here
}
```

## Vulkan surface

```go
import vk "github.com/lukem570/vulkan-go/pkg/raw"

type surfaceWindow interface {
    NewSurface(instance vk.Instance) (*vk.SurfaceKHR, error)
}

sw := win.(surfaceWindow)
surf, err := sw.NewSurface(instance)
```

Enable the right instance extensions based on the display server:

```go
exts := []string{"VK_KHR_surface"}
if _, ok := win.(interface{ WlDisplay() (uintptr, error) }); ok {
    exts = append(exts, "VK_KHR_wayland_surface")
}
// VK_KHR_xlib_surface, VK_KHR_win32_surface, VK_MVK_macos_surface for other platforms
```

## Window interface

```go
type Window interface {
    Size() (width, height uint32)
    Scale() float32

    ShouldClose() bool
    PollEvents()

    SetTitle(title string)
    SetCursor(shape CursorShape)
    SetMinSize(w, h uint32)
    SetSize(w, h uint32)
    Destroy()

    BeginDrag()

    OnResize(fn func(width, height uint32))
    OnLiveResize(fn func())
    OnClose(fn func())
    OnFocus(fn func(focused bool))
    OnKey(fn func(key Key, action Action, mods Mod))
    OnChar(fn func(ch rune))
    OnMouseButton(fn func(button Button, action Action, mods Mod))
    OnMouseMove(fn func(x, y float64))
    OnScroll(fn func(dx, dy float64))
    OnDragBegin(fn func(x, y float64))
    OnDragMove(fn func(x, y float64))
    OnDragEnd(fn func(x, y float64))
    OnDrop(fn func(paths []string))
}
```

## Decorations

Wayland: server-side decorations requested automatically via `zxdg_decoration_manager_v1` when the compositor supports it. Win32 and X11 use native decorations by default.

## Build

```sh
CGO_ENABLED=0 go build ./...
CGO_ENABLED=0 go vet ./...
```

Or with [Task](https://taskfile.dev):

```sh
task build
task vet
```

## License

MIT
