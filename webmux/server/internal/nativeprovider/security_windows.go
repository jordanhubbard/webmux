package nativeprovider

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

func accountDescriptor() (*windows.SECURITY_DESCRIPTOR, *windows.SID, error) {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, nil, err
	}
	sid := u.User.Sid
	sd, err := windows.SecurityDescriptorFromString("O:" + sid.String() + "D:P(A;OICI;FA;;;" + sid.String() + ")")
	return sd, sid, err
}

func secureCreatedDirectory(dir string) error {
	sd, sid, err := accountDescriptor()
	if err != nil {
		return err
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, sid, nil, acl, nil)
}

func privateDirectory(dir string) error {
	descriptor, _, err := accountDescriptor()
	if err != nil {
		return err
	}
	name, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	attrs := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: descriptor}
	err = windows.CreateDirectory(name, &attrs)
	runtime.KeepAlive(descriptor)
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("native provider directory is not an owned directory")
	}
	_, sid, err := accountDescriptor()
	if err != nil {
		return err
	}
	sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil || !windows.EqualSid(owner, sid) {
		return errors.New("native provider directory belongs to another account")
	}
	// Existing state is verified rather than taking ownership or widening it.
	control, _, err := sd.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		return errors.New("native provider directory access is not protected")
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil || acl.AceCount != 1 {
		return errors.New("native provider directory has unexpected access grants")
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(acl, 0, &ace); err != nil {
		return err
	}
	if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || !windows.EqualSid((*windows.SID)(unsafe.Pointer(&ace.SidStart)), sid) {
		return errors.New("native provider directory grants another account access")
	}
	return nil
}

func prepareRoot(root string) error {
	parent := filepath.Dir(root)
	if err := os.MkdirAll(filepath.Dir(parent), 0700); err != nil {
		return err
	}
	if err := privateDirectory(parent); err != nil {
		return err
	}
	return privateDirectory(root)
}

// The mutex remains authoritative even if identity.json is lost. Ownership is
// scoped to this OS account and stable terminal ID, across Windows sessions.
// Windows mutex ownership is thread-affine: keep its owning goroutine pinned.
func ownSession(session string) (func(), error) {
	sd, sid, err := accountDescriptor()
	if err != nil {
		return nil, err
	}
	name, err := windows.UTF16PtrFromString(`Global\WebMux.NativeProvider.` + secretDigest(sid.String()+":"+session))
	if err != nil {
		return nil, err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	handle, err := windows.CreateMutex(&sa, false, name)
	runtime.KeepAlive(sd)
	if err != nil && err != windows.ERROR_ALREADY_EXISTS {
		return nil, err
	}
	runtime.LockOSThread()
	status, err := windows.WaitForSingleObject(handle, 0)
	if err != nil || status != windows.WAIT_OBJECT_0 && status != windows.WAIT_ABANDONED {
		_ = windows.CloseHandle(handle)
		runtime.UnlockOSThread()
		return nil, errors.New("a native provider already owns this terminal; no second job was started")
	}
	return func() { _ = windows.ReleaseMutex(handle); _ = windows.CloseHandle(handle); runtime.UnlockOSThread() }, nil
}
