//go:build darwin || linux

package publish

import (
	"os"

	"golang.org/x/sys/unix"
)

func openPublishFileNoFollow(filePath string) (*os.File, error) {
	fd, err := unix.Open(
		filePath,
		unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		0,
	)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), filePath)
	if file == nil {
		_ = unix.Close(fd)
		return nil, os.ErrInvalid
	}
	return file, nil
}
