//go:build darwin

package cocoa

import (
	"sync"
	"unsafe"

	"github.com/Quikcad/quikwin/internal/platform/cocoa/objc"
)

// The menu bar is application-wide: NSApp owns one mainMenu, not one per
// window. SetMenuBar is on the window because that is where an application
// already holds a handle to the platform, and calling it replaces whatever the
// previous call installed.

// NSControlStateValue.
const (
	stateOff = int64(0)
	stateOn  = int64(1)
)

var (
	selSetMainMenu                  unsafe.Pointer
	selSetServicesMenu              unsafe.Pointer
	selInitWithTitle                unsafe.Pointer
	selInitMenuItem                 unsafe.Pointer
	selSeparatorItem                unsafe.Pointer
	selAddItem                      unsafe.Pointer
	selSetSubmenu                   unsafe.Pointer
	selSetTarget                    unsafe.Pointer
	selSetAction                    unsafe.Pointer
	selSetTag                       unsafe.Pointer
	selTag                          unsafe.Pointer
	selSetKeyEquivalentModifierMask unsafe.Pointer
	selSetState                     unsafe.Pointer
	selSetEnabled                   unsafe.Pointer
	selSetAutoenablesItems          unsafe.Pointer
	selProcessInfo                  unsafe.Pointer
	selProcessName                  unsafe.Pointer
	selMenuAction                   unsafe.Pointer

	menuSelsOnce sync.Once
)

func initMenuSels() {
	menuSelsOnce.Do(func() {
		reg := objc.SelRegister
		selSetMainMenu = reg("setMainMenu:")
		selSetServicesMenu = reg("setServicesMenu:")
		selInitWithTitle = reg("initWithTitle:")
		selInitMenuItem = reg("initWithTitle:action:keyEquivalent:")
		selSeparatorItem = reg("separatorItem")
		selAddItem = reg("addItem:")
		selSetSubmenu = reg("setSubmenu:")
		selSetTarget = reg("setTarget:")
		selSetAction = reg("setAction:")
		selSetTag = reg("setTag:")
		selTag = reg("tag")
		selSetKeyEquivalentModifierMask = reg("setKeyEquivalentModifierMask:")
		selSetState = reg("setState:")
		selSetEnabled = reg("setEnabled:")
		selSetAutoenablesItems = reg("setAutoenablesItems:")
		selProcessInfo = reg("processInfo")
		selProcessName = reg("processName")
		selMenuAction = reg("quikwinMenuAction:")
	})
}

// ---------------------------------------------------------------------------
// Action target
// ---------------------------------------------------------------------------

// menuTarget is the single object every item with an Action points at. An
// NSMenuItem does not retain its target, so it is retained here and kept for
// the process's lifetime rather than per menu bar.
var (
	menuTarget     unsafe.Pointer
	menuTargetOnce sync.Once

	menuMu      sync.Mutex
	menuActions = map[int64]func(){}
	menuNextTag int64
)

func initMenuTarget() {
	menuTargetOnce.Do(func() {
		cls := objc.AllocateClassPair(objc.GetClass("NSObject"), "QuikwinMenuTarget")
		objc.AddMethod(cls, "quikwinMenuAction:", "v@:@", func(_self, _cmd, sender uintptr) {
			if fn := menuAction(objc.MsgSend0i(unsafe.Pointer(sender), selTag)); fn != nil {
				fn()
			}
		})
		objc.RegisterClassPair(cls)
		menuTarget = objc.MsgSend0(objc.MsgSend0(cls, selAlloc), selInit)
	})
}

func menuAction(tag int64) func() {
	menuMu.Lock()
	defer menuMu.Unlock()
	return menuActions[tag]
}

// bindAction files fn under a fresh tag and returns it. Tags keep climbing
// across menu bars so an item left over from a replaced bar cannot fire the
// action that happens to have taken its place.
func bindAction(fn func()) int64 {
	menuMu.Lock()
	defer menuMu.Unlock()
	menuNextTag++
	menuActions[menuNextTag] = fn
	return menuNextTag
}

func resetActions() {
	menuMu.Lock()
	defer menuMu.Unlock()
	menuActions = map[int64]func(){}
}

// ---------------------------------------------------------------------------
// Building
// ---------------------------------------------------------------------------

func (w *window) SetMenuBar(items []MenuItem) {
	initMenuSels()
	initMenuTarget()

	pool := objc.PoolPush()
	defer objc.PoolPop(pool)

	// The previous bar's actions go before the new one is built, so a rebuild
	// driven by a menu item firing does not keep the old closures alive.
	resetActions()

	main := newMenu("MainMenu")
	defer objc.MsgSend0(main, selRelease)

	addAppMenu(main)
	for _, it := range items {
		if it.Separator {
			continue // a separator between menu-bar titles has nowhere to draw
		}
		item := newMenuItem(it.Label, nil, "")
		if len(it.Children) > 0 {
			sub := buildMenu(it.Label, it.Children)
			objc.MsgSend1pVoid(item, selSetSubmenu, sub)
			objc.MsgSend0(sub, selRelease)
		}
		objc.MsgSend1pVoid(main, selAddItem, item)
		objc.MsgSend0(item, selRelease)
	}

	objc.MsgSend1pVoid(nsApp, selSetMainMenu, main)
}

// buildMenu returns a retained NSMenu; the caller owns it.
func buildMenu(title string, items []MenuItem) unsafe.Pointer {
	menu := newMenu(title)
	// Without this AppKit decides each item's enabled state by asking the
	// responder chain, which for a target of ours answers "enabled" and would
	// override Disabled.
	objc.MsgSend1bVoid(menu, selSetAutoenablesItems, 0)

	for _, it := range items {
		if it.Separator {
			// separatorItem is a shared autoreleased instance, so it is not
			// released here the way an allocated item is.
			objc.MsgSend1pVoid(menu, selAddItem,
				objc.MsgSend0(objc.GetClass("NSMenuItem"), selSeparatorItem))
			continue
		}

		key, mods := parseShortcut(it.Shortcut)
		item := newMenuItem(it.Label, nil, key)
		if key != "" {
			objc.MsgSend1iVoid(item, selSetKeyEquivalentModifierMask, int64(mods))
		}
		if it.Action != nil {
			objc.MsgSend1iVoid(item, selSetTag, bindAction(it.Action))
			objc.MsgSend1pVoid(item, selSetTarget, menuTarget)
			objc.MsgSend1pVoid(item, selSetAction, selMenuAction)
		}
		if it.Checked {
			objc.MsgSend1iVoid(item, selSetState, stateOn)
		} else {
			objc.MsgSend1iVoid(item, selSetState, stateOff)
		}
		if it.Disabled {
			objc.MsgSend1bVoid(item, selSetEnabled, 0)
		} else {
			objc.MsgSend1bVoid(item, selSetEnabled, 1)
		}
		if len(it.Children) > 0 {
			sub := buildMenu(it.Label, it.Children)
			objc.MsgSend1pVoid(item, selSetSubmenu, sub)
			objc.MsgSend0(sub, selRelease)
		}

		objc.MsgSend1pVoid(menu, selAddItem, item)
		objc.MsgSend0(item, selRelease)
	}
	return menu
}

// addAppMenu prepends the menu macOS expects every application to have. Its
// entries are the system's own selectors, left targetless so the responder
// chain resolves them on NSApp, and Services needs AppKit to fill it in — none
// of which a caller could express through MenuItem.
func addAppMenu(main unsafe.Pointer) {
	name := processName()

	item := newMenuItem(name, nil, "")
	menu := newMenu(name)

	addSystemItem(menu, "About "+name, "orderFrontStandardAboutPanel:", "", 0)
	addSeparator(menu)

	services := newMenu("Services")
	servicesItem := newMenuItem("Services", nil, "")
	objc.MsgSend1pVoid(servicesItem, selSetSubmenu, services)
	objc.MsgSend1pVoid(menu, selAddItem, servicesItem)
	objc.MsgSend1pVoid(nsApp, selSetServicesMenu, services)
	objc.MsgSend0(servicesItem, selRelease)
	objc.MsgSend0(services, selRelease)

	addSeparator(menu)
	addSystemItem(menu, "Hide "+name, "hide:", "h", modCommand)
	addSystemItem(menu, "Hide Others", "hideOtherApplications:", "h", modCommand|modOption)
	addSystemItem(menu, "Show All", "unhideAllApplications:", "", 0)
	addSeparator(menu)
	addSystemItem(menu, "Quit "+name, "terminate:", "q", modCommand)

	objc.MsgSend1pVoid(item, selSetSubmenu, menu)
	objc.MsgSend1pVoid(main, selAddItem, item)
	objc.MsgSend0(menu, selRelease)
	objc.MsgSend0(item, selRelease)
}

func addSystemItem(menu unsafe.Pointer, label, selName, key string, mods uint64) {
	item := newMenuItem(label, objc.SelRegister(selName), key)
	if mods != 0 {
		objc.MsgSend1iVoid(item, selSetKeyEquivalentModifierMask, int64(mods))
	}
	objc.MsgSend1pVoid(menu, selAddItem, item)
	objc.MsgSend0(item, selRelease)
}

func addSeparator(menu unsafe.Pointer) {
	objc.MsgSend1pVoid(menu, selAddItem,
		objc.MsgSend0(objc.GetClass("NSMenuItem"), selSeparatorItem))
}

// newMenu returns a retained NSMenu; the caller owns it.
func newMenu(title string) unsafe.Pointer {
	raw := objc.MsgSend0(objc.GetClass("NSMenu"), selAlloc)
	return objc.MsgSend1p(raw, selInitWithTitle, nsString(title))
}

// newMenuItem returns a retained NSMenuItem; the caller owns it.
func newMenuItem(label string, action unsafe.Pointer, key string) unsafe.Pointer {
	raw := objc.MsgSend0(objc.GetClass("NSMenuItem"), selAlloc)
	return objc.MsgSend3p(raw, selInitMenuItem, nsString(label), action, nsString(key))
}

func processName() string {
	info := objc.MsgSend0(objc.GetClass("NSProcessInfo"), selProcessInfo)
	name := objc.MsgSend0(info, selProcessName)
	if name == nil {
		return "App"
	}
	utf8 := objc.MsgSend0(name, selUTF8String)
	if utf8 == nil {
		return "App"
	}
	return goString(utf8)
}
