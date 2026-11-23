package maccoord

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Read existing native exports without taking ownership or changing their ACL.
// SYSTEM/Administrators are privileged OS principals; no other account may be
// granted access. Unrecognized allowed ACE forms fail closed.
func privateOwnerFile(path string, _ os.FileInfo) bool {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return false
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return false
	}
	trusted := func(sid *windows.SID) bool {
		return sid != nil && (windows.EqualSid(sid, u.User.Sid) || sid.String() == "S-1-5-18" || sid.String() == "S-1-5-32-544")
	}
	owner, _, err := sd.Owner()
	if err != nil || !trusted(owner) {
		return false
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		return false
	}
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(acl, i, &ace) != nil {
			return false
		}
		if ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || !trusted((*windows.SID)(unsafe.Pointer(&ace.SidStart))) {
			return false
		}
	}
	return true
}
