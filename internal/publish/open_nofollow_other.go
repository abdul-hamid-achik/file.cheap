//go:build !darwin && !linux

package publish

import "os"

// The descriptor identity check in readBoundedRegularFileWithHooks prevents
// this fallback from reading a symlink target or replacement file.
func openPublishFileNoFollow(filePath string) (*os.File, error) {
	return os.Open(filePath)
}
