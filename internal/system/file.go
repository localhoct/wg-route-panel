package system

import (
	"fmt"
	"os"
	"path/filepath"
)

func AtomicWrite(path string, data []byte, mode os.FileMode) error {
	if e := os.MkdirAll(filepath.Dir(path), 0750); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".wgpanel-*")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if e = f.Chmod(mode); e == nil {
		_, e = f.Write(data)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return fmt.Errorf("atomic write: %w", e)
	}
	return os.Rename(tmp, path)
}
