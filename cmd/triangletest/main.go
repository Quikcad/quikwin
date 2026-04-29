package main

import (
	_ "embed"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"unsafe"

	vk "github.com/lukem570/vulkan-go/pkg/raw"

	"github.com/Quikcad/quikwin/pkg/window"
)

//go:embed shader/vert.spv
var vertSPV []byte

//go:embed shader/frag.spv
var fragSPV []byte

type surfaceWindow interface {
	NewSurface(instance vk.Instance) (*vk.SurfaceKHR, error)
}

func main() {
	if err := vk.Initialize(); err != nil {
		log.Fatal("vulkan init:", err)
	}

	win, err := window.New(
		window.WithTitle("Triangle Test"),
		window.WithSize(800, 600),
	)
	if err != nil {
		log.Fatal("create window:", err)
	}
	defer win.Destroy()

	app := &triApp{win: win}
	if err := app.init(); err != nil {
		log.Fatal(err)
	}
	defer app.destroy()

	fmt.Println("rendering triangle — close the window to exit")
	for !win.ShouldClose() {
		win.PollEvents()
		if err := app.drawFrame(); err != nil {
			log.Fatal("draw frame:", err)
		}
	}
	app.device.WaitIdle()
}

// ---------------------------------------------------------------------------
// triApp holds all Vulkan objects for the triangle demo
// ---------------------------------------------------------------------------

type triApp struct {
	win window.Window

	inst          *vk.Instance
	surf          *vk.SurfaceKHR
	physDev       *vk.PhysicalDevice
	graphicsFamily uint32
	device        *vk.Device
	queue         *vk.Queue

	swapchain    *vk.SwapchainKHR
	swapFormat   vk.Format
	swapExtent   vk.Extent2D
	swapImages   []*vk.Image
	swapViews    []*vk.ImageView
	renderPass   *vk.RenderPass
	vertShader   *vk.ShaderModule
	fragShader   *vk.ShaderModule
	pipeLayout   *vk.PipelineLayout
	pipeline     *vk.Pipeline
	framebuffers []*vk.Framebuffer
	cmdPool      *vk.CommandPool
	cmdBufs      []*vk.CommandBuffer

	imageAvail  [maxFrames]*vk.Semaphore
	renderDone  [maxFrames]*vk.Semaphore
	inFlight    [maxFrames]*vk.Fence
	currentFrame int
}

const maxFrames = 2

func (a *triApp) init() error {
	if err := a.createInstance(); err != nil {
		return fmt.Errorf("instance: %w", err)
	}
	if err := a.createSurface(); err != nil {
		return fmt.Errorf("surface: %w", err)
	}
	if err := a.pickPhysicalDevice(); err != nil {
		return fmt.Errorf("physical device: %w", err)
	}
	if err := a.createDevice(); err != nil {
		return fmt.Errorf("device: %w", err)
	}
	if err := a.createSwapchain(); err != nil {
		return fmt.Errorf("swapchain: %w", err)
	}
	if err := a.createRenderPass(); err != nil {
		return fmt.Errorf("render pass: %w", err)
	}
	if err := a.createPipeline(); err != nil {
		return fmt.Errorf("pipeline: %w", err)
	}
	if err := a.createFramebuffers(); err != nil {
		return fmt.Errorf("framebuffers: %w", err)
	}
	if err := a.createCommandBuffers(); err != nil {
		return fmt.Errorf("command buffers: %w", err)
	}
	if err := a.createSyncObjects(); err != nil {
		return fmt.Errorf("sync objects: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Instance
// ---------------------------------------------------------------------------

func (a *triApp) createInstance() error {
	inst, err := vk.CreateInstance(&vk.InstanceCreateInfo{
		ApplicationInfo: &vk.ApplicationInfo{
			ApplicationName: "triangle-test",
			ApiVersion:      apiVersion(1, 2, 0),
		},
		EnabledExtensionNames: platformExts(a.win),
	}, nil)
	if err != nil {
		return err
	}
	a.inst = inst
	return nil
}

// ---------------------------------------------------------------------------
// Surface
// ---------------------------------------------------------------------------

func (a *triApp) createSurface() error {
	sw, ok := a.win.(surfaceWindow)
	if !ok {
		return fmt.Errorf("window does not support Vulkan surface")
	}
	surf, err := sw.NewSurface(*a.inst)
	if err != nil {
		return err
	}
	a.surf = surf
	return nil
}

// ---------------------------------------------------------------------------
// Physical device
// ---------------------------------------------------------------------------

func (a *triApp) pickPhysicalDevice() error {
	devs, err := a.inst.EnumeratePhysicalDevices()
	if err != nil {
		return err
	}
	for _, dev := range devs {
		qf, ok := findGraphicsFamily(dev, a.surf)
		if !ok {
			continue
		}
		a.physDev = dev
		a.graphicsFamily = qf
		props := dev.GetProperties()
		fmt.Printf("using GPU: %s\n", props.DeviceName)
		return nil
	}
	return fmt.Errorf("no suitable GPU found")
}

func findGraphicsFamily(dev *vk.PhysicalDevice, surf *vk.SurfaceKHR) (uint32, bool) {
	props := dev.GetQueueFamilyProperties()
	for i, p := range props {
		if p.QueueFlags&vk.QueueFlags(vk.QueueGraphicsBit) == 0 {
			continue
		}
		support, err := dev.GetSurfaceSupportKHR(uint32(i), surf)
		if err != nil || !support {
			continue
		}
		return uint32(i), true
	}
	return 0, false
}

// ---------------------------------------------------------------------------
// Logical device
// ---------------------------------------------------------------------------

func (a *triApp) createDevice() error {
	priority := float32(1.0)
	dev, err := a.physDev.CreateDevice(&vk.DeviceCreateInfo{
		QueueCreateInfos: []vk.DeviceQueueCreateInfo{{
			QueueFamilyIndex: a.graphicsFamily,
			QueuePriorities:  []float32{priority},
		}},
		EnabledExtensionNames: []string{"VK_KHR_swapchain"},
	}, nil)
	if err != nil {
		return err
	}
	a.device = dev
	a.queue = dev.GetQueue(a.graphicsFamily, 0)
	return nil
}

// ---------------------------------------------------------------------------
// Swapchain
// ---------------------------------------------------------------------------

func (a *triApp) createSwapchain() error {
	caps, err := a.physDev.GetSurfaceCapabilitiesKHR(a.surf)
	if err != nil {
		return err
	}
	formats, err := a.physDev.GetSurfaceFormatsKHR(a.surf)
	if err != nil {
		return err
	}
	modes, err := a.physDev.GetSurfacePresentModesKHR(a.surf)
	if err != nil {
		return err
	}

	format := chooseSurfaceFormat(formats)
	mode := choosePresentMode(modes)
	extent := chooseExtent(caps, a.win)

	imageCount := caps.MinImageCount + 1
	if caps.MaxImageCount > 0 && imageCount > caps.MaxImageCount {
		imageCount = caps.MaxImageCount
	}

	sc, err := a.device.CreateSwapchainKHR(&vk.SwapchainCreateInfoKHR{
		Surface:          a.surf,
		MinImageCount:    imageCount,
		ImageFormat:      format.Format,
		ImageColorSpace:  format.ColorSpace,
		ImageExtent:      extent,
		ImageArrayLayers: 1,
		ImageUsage:       vk.ImageUsageFlags(vk.ImageUsageColorAttachmentBit),
		ImageSharingMode: vk.SharingModeExclusive,
		PreTransform:     caps.CurrentTransform,
		CompositeAlpha:   vk.CompositeAlphaOpaqueBitKHR,
		PresentMode:      mode,
		Clipped:          true,
	}, nil)
	if err != nil {
		return err
	}
	a.swapchain = sc
	a.swapFormat = format.Format
	a.swapExtent = extent

	imgs, err := a.device.GetSwapchainImagesKHR(sc)
	if err != nil {
		return err
	}
	a.swapImages = imgs

	views := make([]*vk.ImageView, len(imgs))
	for i, img := range imgs {
		v, err := a.device.CreateImageView(&vk.ImageViewCreateInfo{
			Image:    img,
			ViewType: vk.ImageViewType2d,
			Format:   format.Format,
			Components: vk.ComponentMapping{
				R: vk.ComponentSwizzleIdentity,
				G: vk.ComponentSwizzleIdentity,
				B: vk.ComponentSwizzleIdentity,
				A: vk.ComponentSwizzleIdentity,
			},
			SubresourceRange: vk.ImageSubresourceRange{
				AspectMask:     vk.ImageAspectFlags(vk.ImageAspectColorBit),
				BaseMipLevel:   0,
				LevelCount:     1,
				BaseArrayLayer: 0,
				LayerCount:     1,
			},
		}, nil)
		if err != nil {
			return fmt.Errorf("image view %d: %w", i, err)
		}
		views[i] = v
	}
	a.swapViews = views
	return nil
}

func chooseSurfaceFormat(formats []vk.SurfaceFormatKHR) vk.SurfaceFormatKHR {
	for _, f := range formats {
		if f.Format == vk.FormatB8g8r8a8Srgb && f.ColorSpace == vk.ColorSpaceSrgbNonlinearKHR {
			return f
		}
	}
	if len(formats) > 0 {
		return formats[0]
	}
	return vk.SurfaceFormatKHR{Format: vk.FormatB8g8r8a8Unorm, ColorSpace: vk.ColorSpaceSrgbNonlinearKHR}
}

func choosePresentMode(modes []vk.PresentModeKHR) vk.PresentModeKHR {
	for _, m := range modes {
		if m == vk.PresentModeMailboxKHR {
			return m
		}
	}
	return vk.PresentModeFifoKHR
}

func chooseExtent(caps vk.SurfaceCapabilitiesKHR, win window.Window) vk.Extent2D {
	if caps.CurrentExtent.Width != math.MaxUint32 {
		return caps.CurrentExtent
	}
	w, h := win.Size()
	return vk.Extent2D{
		Width:  clampU32(w, caps.MinImageExtent.Width, caps.MaxImageExtent.Width),
		Height: clampU32(h, caps.MinImageExtent.Height, caps.MaxImageExtent.Height),
	}
}

// ---------------------------------------------------------------------------
// Render pass
// ---------------------------------------------------------------------------

func (a *triApp) createRenderPass() error {
	rp, err := a.device.CreateRenderPass(&vk.RenderPassCreateInfo{
		Attachments: []vk.AttachmentDescription{{
			Format:         a.swapFormat,
			Samples:        vk.SampleCount1Bit,
			LoadOp:         vk.AttachmentLoadOpClear,
			StoreOp:        vk.AttachmentStoreOpStore,
			StencilLoadOp:  vk.AttachmentLoadOpDontCare,
			StencilStoreOp: vk.AttachmentStoreOpDontCare,
			InitialLayout:  vk.ImageLayoutUndefined,
			FinalLayout:    vk.ImageLayoutPresentSrcKHR,
		}},
		Subpasses: []vk.SubpassDescription{{
			PipelineBindPoint:    vk.PipelineBindPointGraphics,
			ColorAttachmentCount: 1,
			ColorAttachments: []vk.AttachmentReference{{
				Attachment: 0,
				Layout:     vk.ImageLayoutColorAttachmentOptimal,
			}},
		}},
		Dependencies: []vk.SubpassDependency{{
			SrcSubpass:    ^uint32(0), // VK_SUBPASS_EXTERNAL
			DstSubpass:    0,
			SrcStageMask:  vk.PipelineStageFlags(vk.PipelineStageColorAttachmentOutputBit),
			DstStageMask:  vk.PipelineStageFlags(vk.PipelineStageColorAttachmentOutputBit),
			SrcAccessMask: 0,
			DstAccessMask: vk.AccessFlags(vk.AccessColorAttachmentWriteBit),
		}},
	}, nil)
	if err != nil {
		return err
	}
	a.renderPass = rp
	return nil
}

// ---------------------------------------------------------------------------
// Pipeline
// ---------------------------------------------------------------------------

func (a *triApp) createPipeline() error {
	vs, err := a.device.CreateShaderModule(&vk.ShaderModuleCreateInfo{Code: toUint32Slice(vertSPV)}, nil)
	if err != nil {
		return fmt.Errorf("vertex shader: %w", err)
	}
	a.vertShader = vs

	fs, err := a.device.CreateShaderModule(&vk.ShaderModuleCreateInfo{Code: toUint32Slice(fragSPV)}, nil)
	if err != nil {
		return fmt.Errorf("fragment shader: %w", err)
	}
	a.fragShader = fs

	layout, err := a.device.CreatePipelineLayout(&vk.PipelineLayoutCreateInfo{}, nil)
	if err != nil {
		return fmt.Errorf("pipeline layout: %w", err)
	}
	a.pipeLayout = layout

	allWriteMask := vk.ColorComponentFlags(
		vk.ColorComponentRBit | vk.ColorComponentGBit |
			vk.ColorComponentBBit | vk.ColorComponentABit)

	pipelines, err := a.device.CreateGraphicsPipelines(nil, []vk.GraphicsPipelineCreateInfo{{
		StageCount: 2,
		Stages: []vk.PipelineShaderStageCreateInfo{
			{Stage: vk.ShaderStageVertexBit, Module: vs, Name: "main"},
			{Stage: vk.ShaderStageFragmentBit, Module: fs, Name: "main"},
		},
		VertexInputState: &vk.PipelineVertexInputStateCreateInfo{},
		InputAssemblyState: &vk.PipelineInputAssemblyStateCreateInfo{
			Topology: vk.PrimitiveTopologyTriangleList,
		},
		ViewportState: &vk.PipelineViewportStateCreateInfo{
			ViewportCount: 1,
			ScissorCount:  1,
		},
		RasterizationState: &vk.PipelineRasterizationStateCreateInfo{
			PolygonMode: vk.PolygonModeFill,
			CullMode:    vk.CullModeFlags(vk.CullModeNone),
			FrontFace:   vk.FrontFaceCounterClockwise,
			LineWidth:   1.0,
		},
		MultisampleState: &vk.PipelineMultisampleStateCreateInfo{
			RasterizationSamples: vk.SampleCount1Bit,
		},
		ColorBlendState: &vk.PipelineColorBlendStateCreateInfo{
			AttachmentCount: 1,
			Attachments: []vk.PipelineColorBlendAttachmentState{{
				ColorWriteMask: allWriteMask,
			}},
		},
		DynamicState: &vk.PipelineDynamicStateCreateInfo{
			DynamicStates: []vk.DynamicState{vk.DynamicStateViewport, vk.DynamicStateScissor},
		},
		Layout:     layout,
		RenderPass: a.renderPass,
		Subpass:    0,
	}}, nil)
	if err != nil {
		return fmt.Errorf("graphics pipeline: %w", err)
	}
	a.pipeline = pipelines[0]
	return nil
}

// ---------------------------------------------------------------------------
// Framebuffers
// ---------------------------------------------------------------------------

func (a *triApp) createFramebuffers() error {
	fbs := make([]*vk.Framebuffer, len(a.swapViews))
	for i, view := range a.swapViews {
		fb, err := a.device.CreateFramebuffer(&vk.FramebufferCreateInfo{
			RenderPass:  a.renderPass,
			Attachments: []*vk.ImageView{view},
			Width:       a.swapExtent.Width,
			Height:      a.swapExtent.Height,
			Layers:      1,
		}, nil)
		if err != nil {
			return fmt.Errorf("framebuffer %d: %w", i, err)
		}
		fbs[i] = fb
	}
	a.framebuffers = fbs
	return nil
}

// ---------------------------------------------------------------------------
// Command buffers
// ---------------------------------------------------------------------------

func (a *triApp) createCommandBuffers() error {
	pool, err := a.device.CreateCommandPool(&vk.CommandPoolCreateInfo{
		QueueFamilyIndex: a.graphicsFamily,
	}, nil)
	if err != nil {
		return err
	}
	a.cmdPool = pool

	bufs, err := a.device.AllocateCommandBuffers(&vk.CommandBufferAllocateInfo{
		CommandPool:        pool,
		Level:              vk.CommandBufferLevelPrimary,
		CommandBufferCount: uint32(len(a.framebuffers)),
	})
	if err != nil {
		return err
	}
	a.cmdBufs = bufs

	for i, cb := range bufs {
		if err := cb.Begin(&vk.CommandBufferBeginInfo{}); err != nil {
			return fmt.Errorf("begin command buffer %d: %w", i, err)
		}

		cb.BeginRenderPass(&vk.RenderPassBeginInfo{
			RenderPass:  a.renderPass,
			Framebuffer: a.framebuffers[i],
			RenderArea: vk.Rect2D{
				Offset: vk.Offset2D{X: 0, Y: 0},
				Extent: a.swapExtent,
			},
			ClearValues: []vk.ClearValue{clearColor(0.0, 0.0, 0.0, 1.0)},
		}, vk.SubpassContentsInline)

		cb.SetViewport(0, []vk.Viewport{{
			X:        0, Y: 0,
			Width:    float32(a.swapExtent.Width),
			Height:   float32(a.swapExtent.Height),
			MinDepth: 0, MaxDepth: 1,
		}})

		cb.SetScissor(0, []vk.Rect2D{{
			Offset: vk.Offset2D{},
			Extent: a.swapExtent,
		}})

		cb.BindPipeline(vk.PipelineBindPointGraphics, a.pipeline)
		cb.Draw(3, 1, 0, 0)
		cb.EndRenderPass()

		if err := cb.End(); err != nil {
			return fmt.Errorf("end command buffer %d: %w", i, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Sync objects
// ---------------------------------------------------------------------------

func (a *triApp) createSyncObjects() error {
	for i := range maxFrames {
		sem1, err := a.device.CreateSemaphore(&vk.SemaphoreCreateInfo{}, nil)
		if err != nil {
			return err
		}
		a.imageAvail[i] = sem1

		sem2, err := a.device.CreateSemaphore(&vk.SemaphoreCreateInfo{}, nil)
		if err != nil {
			return err
		}
		a.renderDone[i] = sem2

		fence, err := a.device.CreateFence(&vk.FenceCreateInfo{
			Flags: vk.FenceCreateFlags(vk.FenceCreateSignaledBit),
		}, nil)
		if err != nil {
			return err
		}
		a.inFlight[i] = fence
	}
	return nil
}

// ---------------------------------------------------------------------------
// Draw frame
// ---------------------------------------------------------------------------

func (a *triApp) drawFrame() error {
	f := a.currentFrame

	if err := a.device.WaitForFences([]*vk.Fence{a.inFlight[f]}, true, ^uint64(0)); err != nil {
		return err
	}
	if err := a.device.ResetFences([]*vk.Fence{a.inFlight[f]}); err != nil {
		return err
	}

	imgIdx, err := a.device.AcquireNextImageKHR(a.swapchain, ^uint64(0), a.imageAvail[f], nil)
	if err != nil {
		return fmt.Errorf("acquire image: %w", err)
	}

	if err := a.queue.Submit([]vk.SubmitInfo{{
		WaitSemaphoreCount: 1,
		WaitSemaphores:     []*vk.Semaphore{a.imageAvail[f]},
		WaitDstStageMask:   []vk.PipelineStageFlags{vk.PipelineStageFlags(vk.PipelineStageColorAttachmentOutputBit)},
		CommandBuffers:     []*vk.CommandBuffer{a.cmdBufs[imgIdx]},
		SignalSemaphores:   []*vk.Semaphore{a.renderDone[f]},
	}}, a.inFlight[f]); err != nil {
		return fmt.Errorf("queue submit: %w", err)
	}

	if err := a.queue.PresentKHR(&vk.PresentInfoKHR{
		WaitSemaphores: []*vk.Semaphore{a.renderDone[f]},
		SwapchainCount: 1,
		Swapchains:     []*vk.SwapchainKHR{a.swapchain},
		ImageIndices:   []uint32{imgIdx},
	}); err != nil {
		return fmt.Errorf("present: %w", err)
	}

	a.currentFrame = (a.currentFrame + 1) % maxFrames
	return nil
}

// ---------------------------------------------------------------------------
// Destroy
// ---------------------------------------------------------------------------

func (a *triApp) destroy() {
	if a.device != nil {
		a.device.WaitIdle()
	}
	for i := range maxFrames {
		if a.imageAvail[i] != nil {
			a.device.DestroySemaphore(a.imageAvail[i], nil)
		}
		if a.renderDone[i] != nil {
			a.device.DestroySemaphore(a.renderDone[i], nil)
		}
		if a.inFlight[i] != nil {
			a.device.DestroyFence(a.inFlight[i], nil)
		}
	}
	if a.cmdPool != nil {
		a.device.DestroyCommandPool(a.cmdPool, nil)
	}
	for _, fb := range a.framebuffers {
		a.device.DestroyFramebuffer(fb, nil)
	}
	if a.pipeline != nil {
		a.device.DestroyPipeline(a.pipeline, nil)
	}
	if a.pipeLayout != nil {
		a.device.DestroyPipelineLayout(a.pipeLayout, nil)
	}
	if a.vertShader != nil {
		a.device.DestroyShaderModule(a.vertShader, nil)
	}
	if a.fragShader != nil {
		a.device.DestroyShaderModule(a.fragShader, nil)
	}
	if a.renderPass != nil {
		a.device.DestroyRenderPass(a.renderPass, nil)
	}
	for _, v := range a.swapViews {
		a.device.DestroyImageView(v, nil)
	}
	if a.swapchain != nil {
		a.device.DestroySwapchainKHR(a.swapchain, nil)
	}
	if a.device != nil {
		a.device.Destroy(nil)
	}
	if a.surf != nil && a.inst != nil {
		a.inst.DestroySurfaceKHR(a.surf, nil)
	}
	if a.inst != nil {
		a.inst.Destroy(nil)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func platformExts(win window.Window) []string {
	exts := []string{"VK_KHR_surface"}
	switch {
	case hasMethod[interface{ WlDisplay() (uintptr, error) }](win):
		exts = append(exts, "VK_KHR_wayland_surface")
	case hasMethod[interface{ Display() (uintptr, error) }](win):
		exts = append(exts, "VK_KHR_xlib_surface")
	case hasMethod[interface{ HWND() (uintptr, error) }](win):
		exts = append(exts, "VK_KHR_win32_surface")
	}
	return exts
}

func hasMethod[T any](v any) bool { _, ok := v.(T); return ok }

func apiVersion(major, minor, patch uint32) uint32 {
	return (major << 22) | (minor << 12) | patch
}

func clampU32(v, lo, hi uint32) uint32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// toUint32Slice reinterprets a SPIR-V byte slice as []uint32.
func toUint32Slice(b []byte) []uint32 {
	if len(b)%4 != 0 {
		panic("SPIR-V size not a multiple of 4")
	}
	out := make([]uint32, len(b)/4)
	for i := range out {
		out[i] = binary.LittleEndian.Uint32(b[i*4:])
	}
	return out
}

// clearColor packs RGBA floats into a VkClearValue.
func clearColor(r, g, b, a float32) vk.ClearValue {
	var cv vk.ClearValue
	*(*float32)(unsafe.Pointer(&cv[0])) = r
	*(*float32)(unsafe.Pointer(&cv[4])) = g
	*(*float32)(unsafe.Pointer(&cv[8])) = b
	*(*float32)(unsafe.Pointer(&cv[12])) = a
	return cv
}
