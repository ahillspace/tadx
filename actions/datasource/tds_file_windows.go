package datasource

import (
	"errors"
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

func editDirectory(info os.FileInfo) bool {
	data, ok := info.Sys().(*syscall.Win32FileAttributeData)
	return ok && info.IsDir() && data.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT == 0
}

func openEditInput(dir *os.Root, name string, expected os.FileInfo) (*os.File, error) {
	f, err := dir.Open(name)
	if err != nil {
		return nil, err
	}
	var attrs windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &attrs); err != nil {
		f.Close()
		return nil, err
	}
	actual, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if attrs.NumberOfLinks != 1 || attrs.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || !actual.Mode().IsRegular() || !os.SameFile(expected, actual) {
		f.Close()
		return nil, errors.New("changes input must be one unaliased regular file")
	}
	return f, nil
}
