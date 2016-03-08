// +build windows

package rolling_file_appender

import (
	"os"
	"syscall"
	"unsafe"
)

func createHidden(name string) (*os.File, error) {
	var sa syscall.SecurityAttributes
	sa.Length = uint32(unsafe.Sizeof(sa))
	sa.InheritHandle = 1
	handle, err := syscall.CreateFile(
		syscall.StringToUTF16Ptr(name),
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE,
		&sa,
		syscall.CREATE_ALWAYS,
		syscall.FILE_ATTRIBUTE_HIDDEN,
		0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), name), nil
}
