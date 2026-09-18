package managedpolicy

import (
	"encoding/binary"
	"errors"
)

// Darwin's supported amd64 and arm64 architectures use little-endian native
// attribute buffers: a length, an attrreference, then a kauth_filesec value.
func checkDarwinACLBuffer(buffer []byte) error {
	invalid := errors.New("invalid extended ACL data")
	if len(buffer) < 12 {
		return invalid
	}
	length := uint64(binary.LittleEndian.Uint32(buffer))
	if length < 12 || length > uint64(len(buffer)) {
		return invalid
	}
	offset := int64(int32(binary.LittleEndian.Uint32(buffer[4:]))) + 4
	size := uint64(binary.LittleEndian.Uint32(buffer[8:]))
	if size == 0 {
		return nil
	}
	if offset < 12 || uint64(offset)+size > length || size < 44 {
		return invalid
	}
	acl := buffer[offset : uint64(offset)+size]
	if binary.LittleEndian.Uint32(acl) != 0x012cc16d {
		return invalid
	}
	count := binary.LittleEndian.Uint32(acl[36:])
	if count == 0xffffffff {
		if size != 44 {
			return invalid
		}
		return nil
	}
	if count > 128 || size != 44+uint64(count)*24 {
		return invalid
	}
	const unsafeRights = 1<<2 | 1<<4 | 1<<5 | 1<<6 | 1<<8 | 1<<10 | 1<<12 | 1<<13 | 1<<21 | 1<<23
	for index := uint32(0); index < count; index++ {
		entry := acl[44+index*24:]
		flags := binary.LittleEndian.Uint32(entry[16:])
		rights := binary.LittleEndian.Uint32(entry[20:])
		if flags&(1<<8) != 0 || flags&0xf == 2 {
			continue
		}
		if flags&0xf != 1 {
			return errors.New("unsupported extended ACL entry")
		}
		if rights&unsafeRights != 0 {
			return errors.New("extended ACL modification grant cannot be verified as root-only; use root ownership and POSIX mode protection")
		}
	}
	return nil
}
