package managedpolicy

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const locatorKey = `SOFTWARE\TADX\ManagedPolicy`
const protectedSDDL = "O:BAG:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;GRGX;;;BU)"
const registrySDDL = "O:BAG:BAD:P(A;CI;KA;;;SY)(A;CI;KA;;;BA)(A;CI;KR;;;BU)"

func windowsRegistryRoot() registry.Key { return registry.LOCAL_MACHINE }

func SystemPath() (string, error) { path, _, err := systemPolicyLocation(); return path, err }
func systemPolicyLocation() (string, bool, error) {
	return readLocation(registry.LOCAL_MACHINE, locatorKey)
}

func readLocation(root registry.Key, keyPath string) (string, bool, error) {
	key, err := registry.OpenKey(root, keyPath, registry.READ|registry.WOW64_64KEY)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		path, err := defaultSystemPath()
		return path, false, err
	}
	if err != nil {
		return "", true, fmt.Errorf("cannot read the protected policy locator: %w", err)
	}
	defer key.Close()
	if err := checkRegistryKey(key); err != nil {
		return "", true, err
	}
	parent, err := registry.OpenKey(root, filepath.Dir(keyPath), registry.READ|registry.WOW64_64KEY)
	if err != nil {
		return "", true, err
	}
	defer parent.Close()
	if err := checkRegistryKey(parent); err != nil {
		return "", true, err
	}
	dir, kind, err := key.GetStringValue("Directory")
	if err != nil || kind != registry.SZ {
		return "", true, errors.New("the protected policy locator requires a Directory REG_SZ value")
	}
	if err := validateLocalDirectory(dir); err != nil {
		return "", true, fmt.Errorf("invalid policy locator: %w", err)
	}
	return filepath.Join(dir, "managed-policy.json"), true, nil
}

func checkRegistryKey(key registry.Key) error {
	sd, err := windows.GetSecurityInfo(windows.Handle(key), windows.SE_REGISTRY_KEY, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return errors.New("cannot inspect policy locator protection")
	}
	owner, _, err := sd.Owner()
	if err != nil || !trustedSID(owner) {
		return errors.New("policy locator owner must be SYSTEM, Administrators, or TrustedInstaller")
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		return errors.New("policy locator requires a non-null DACL")
	}
	const unsafeMask = windows.KEY_SET_VALUE | windows.KEY_CREATE_SUB_KEY | windows.DELETE | windows.WRITE_DAC | windows.WRITE_OWNER | windows.GENERIC_ALL | windows.GENERIC_WRITE
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			return err
		}
		if ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 || ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return errors.New("unsupported policy locator DACL entry")
		}
		if ace.Mask&unsafeMask != 0 && !trustedSID((*windows.SID)(unsafe.Pointer(&ace.SidStart))) {
			return errors.New("policy locator permits non-administrator changes")
		}
	}
	return nil
}

func protectHandle(handle windows.Handle, kind windows.SE_OBJECT_TYPE, sddl string) error {
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetSecurityInfo(handle, kind, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, owner, nil, dacl, nil)
}

func publishLocation(root registry.Key, keyPath, dir string) (bool, error) {
	parentPath := filepath.Dir(keyPath)
	parent, created, err := registry.CreateKey(root, parentPath, registry.ALL_ACCESS|registry.WOW64_64KEY)
	if err != nil {
		return false, err
	}
	defer parent.Close()
	if created {
		if err := protectHandle(windows.Handle(parent), windows.SE_REGISTRY_KEY, registrySDDL); err != nil {
			return false, err
		}
	}
	if err := checkRegistryKey(parent); err != nil {
		return false, err
	}
	key, _, err := registry.CreateKey(parent, filepath.Base(keyPath), registry.ALL_ACCESS|registry.WOW64_64KEY)
	if err != nil {
		return false, err
	}
	defer key.Close()
	if err := protectHandle(windows.Handle(key), windows.SE_REGISTRY_KEY, registrySDDL); err != nil {
		return false, err
	}
	if err := checkRegistryKey(key); err != nil {
		return false, err
	}
	if err := key.SetStringValue("Directory", dir); err != nil {
		return false, err
	}
	path, _, err := readLocation(root, keyPath)
	if err == nil && !sameInstallPath(path, filepath.Join(dir, "managed-policy.json")) {
		err = errors.New("policy locator verification returned a different directory")
	}
	return true, err
}

func validateLocalDirectory(dir string) error {
	if !filepath.IsAbs(dir) || strings.HasPrefix(dir, `\\`) || filepath.Clean(dir) != dir || len(filepath.VolumeName(dir)) != 2 || strings.ContainsAny(strings.TrimPrefix(dir, filepath.VolumeName(dir)), ":\x00") {
		return errors.New("policy directory must be a clean local absolute path")
	}
	for _, part := range strings.Split(strings.TrimPrefix(dir, filepath.VolumeName(dir)), `\`) {
		if strings.TrimRight(part, " .") != part {
			return errors.New("policy directory cannot use trailing spaces or dots")
		}
	}
	return nil
}

func sameInstallPath(a, b string) bool {
	return a != "" && strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}
