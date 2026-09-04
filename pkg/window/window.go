package window

import "github.com/Quikcad/quikwin/internal/platform/wtypes"

// Window is the core window interface. Its canonical declaration lives in
// internal/platform/wtypes so the backends can assert against it at compile
// time — importing this package from them would cycle through
// internal/platform/common.
type Window = wtypes.Window
