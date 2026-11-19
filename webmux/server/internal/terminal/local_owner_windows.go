package terminal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"

	"github.com/jordanhubbard/webmux/server/internal/nativeprovider"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func LocalExecutionScope() (string, error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Cryptography`, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return "", err
	}
	defer key.Close()
	machine, _, err := key.GetStringValue("MachineGuid")
	if err != nil || machine == "" {
		return "", errors.New("Windows execution host identity is unavailable")
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(machine + ":" + user.User.Sid.String()))
	return hex.EncodeToString(digest[:]), nil
}

func captureLocalOwner(i nativeprovider.Identity) (LocalOwnerTarget, error) {
	scope, err := LocalExecutionScope()
	if err != nil {
		return LocalOwnerTarget{}, err
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(i.OwnerPID))
	if err != nil {
		return LocalOwnerTarget{}, err
	}
	defer windows.CloseHandle(h)
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return LocalOwnerTarget{}, err
	}
	return LocalOwnerTarget{Version: 1, Scope: scope, Session: i.Session, Instance: i.Instance, Endpoint: i.Endpoint,
		OwnerPID: i.OwnerPID, RootPID: i.RootPID, BinarySHA256: i.BinarySHA256, Replay: i.Replay,
		OwnerCreation: uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime)}, nil
}

// A nil error means this exact original process is gone. Neither failure to
// connect nor an unobservable process is proof of exit.
func InspectLocalOwner(ctx context.Context, t LocalOwnerTarget, _ string) error {
	h, gone, err := openLocalOwner(ctx, t)
	if h != 0 {
		defer windows.CloseHandle(h)
	}
	if err != nil || gone {
		return err
	}
	return errors.New("the original local provider is still running; its stop outcome is unconfirmed")
}

func openLocalOwner(ctx context.Context, t LocalOwnerTarget) (windows.Handle, bool, error) {
	if err := ctx.Err(); err != nil {
		return 0, false, err
	}
	scope, err := LocalExecutionScope()
	if err != nil || scope != t.Scope {
		return 0, false, errors.New("original local provider execution host/account cannot be verified")
	}
	if t.Version != 1 || t.Session == "" || len(t.Instance) != 32 || t.OwnerPID <= 0 || uint64(t.OwnerPID) > 1<<32-1 || t.OwnerCreation == 0 || !filepath.IsAbs(t.Endpoint) || len(t.BinarySHA256) != 64 {
		return 0, false, errors.New("original local provider identity is unavailable; retain the session for manual recovery")
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(t.OwnerPID))
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return 0, true, nil
	}
	if err != nil {
		return 0, false, errors.New("original local provider process cannot be inspected")
	}
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		windows.CloseHandle(h)
		return 0, false, errors.New("original local provider creation time cannot be inspected")
	}
	stamp := uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime)
	status, err := windows.WaitForSingleObject(h, 0)
	if err != nil {
		windows.CloseHandle(h)
		return 0, false, errors.New("original local provider exit cannot be inspected")
	}
	return h, stamp != t.OwnerCreation || status == windows.WAIT_OBJECT_0, nil
}

func StopLocalOwner(ctx context.Context, t LocalOwnerTarget, key string) error {
	h, gone, err := openLocalOwner(ctx, t)
	if h != 0 {
		defer windows.CloseHandle(h)
	}
	if err != nil || gone {
		return err
	}
	var size uint32 = 32768
	path := make([]uint16, size)
	if err := windows.QueryFullProcessImageName(h, 0, &path[0], &size); err != nil {
		return errors.New("original local provider image cannot be verified")
	}
	image, err := os.ReadFile(windows.UTF16ToString(path[:size]))
	if err != nil {
		return errors.New("original local provider image cannot be verified")
	}
	digest := sha256.Sum256(image)
	if hex.EncodeToString(digest[:]) != t.BinarySHA256 {
		return errors.New("original local provider image changed; stop was refused")
	}
	return nativeprovider.Stop(ctx, nativeprovider.Identity{Session: t.Session, Instance: t.Instance, Endpoint: t.Endpoint,
		OwnerPID: t.OwnerPID, RootPID: t.RootPID, BinarySHA256: t.BinarySHA256, Replay: t.Replay}, key)
}
