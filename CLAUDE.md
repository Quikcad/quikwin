# quikwin

Cross-platform windowing library in pure Go. Companion to `github.com/lukem570/vulkan-go`.

## Module

`github.com/Quikcad/quikwin`

## Purpose

Provides idiomatic Go windowing for Vulkan applications. Exposes `VkSurface()` so callers can hand the surface directly to `vulkan-go`. No CGo — all platform calls go through `goffi`.

## Design rules

- Pure Go. No CGo. FFI via `goffi`.
- Idiomatic Go: interfaces, zero-value safety, error returns (not panics), functional options for config.
- `pkg/window` holds the core `Window` interface and shared types (`Key`, `Action`, `Mod`, `Button`, `CursorShape`).
- Platform backends live under `pkg/window/<platform>` (e.g. `pkg/window/win32`, `pkg/window/x11`, `pkg/window/wayland`, `pkg/window/cocoa`).
- Each backend implements `window.Window`. No backend symbol leaks into the public API.
- Constructor `New(opts ...Option) (window.Window, error)` selects the right backend at runtime.

## Vulkan integration

- `VkSurface() *vk.SurfaceKHR` returns a surface the caller owns; caller destroys it.
- Import `vk` from `github.com/lukem570/vulkan-go`.

## Conventions

- API boundaries use `float64`. `float32` only for GPU uploads.
- Angles are radians.
- Value receivers for small fixed-size types; pointer receivers for large/mutable state.
- Place uncommented compile-time interface checks directly above the type definition. Use a `var` block for multiple checks, a standalone `var` for a single check.
- Always expand struct definitions across multiple lines, even with a single field.
- Factories that make value types are called `Make*`; factories that return pointers are called `New*`.
- No global state.
- Callbacks registered with `OnX` replace any previous callback (no multi-listener lists).
- `PollEvents` is non-blocking; the caller drives the loop.
- `Destroy` is idempotent.
- Unexported fields only; expose state through methods.
- No comments that restate the method name.

## Key types (pkg/window)

```
Window      — core interface
Key         — keyboard key constant
Action      — Press / Release / Repeat
Mod         — modifier bitmask
Button      — mouse button constant
CursorShape — standard cursor shapes
```

## Agent identity

You are the smartest artificial intelegence that is incapable
of writing sloppy or incomplete code. you are so smart that 
you remove all of the words not reqired for your thinking. 
