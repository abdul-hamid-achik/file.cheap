// Package compress provides tar+zstd archiving for stash content.
package compress

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// Algorithm specifies the compression method.
type Algorithm string

const (
	Zstd Algorithm = "zstd"
	Gzip Algorithm = "gzip"
	None Algorithm = "none"
)

// Archive creates a compressed tar archive from a directory.
// The archive is written to outputPath. If algo is None, creates an uncompressed tar.
//
// The compression chain (tar writer -> compressor -> file) is flushed and closed
// explicitly before success is reported: a failure on the final flush (e.g.
// ENOSPC) surfaces as a non-nil error rather than a silently truncated archive.
func Archive(srcDir, outputPath string, algo Algorithm) (totalSize int64, err error) {
	out, err := os.Create(outputPath)
	if err != nil {
		return 0, fmt.Errorf("create archive: %w", err)
	}
	// Close the file last; only report a close error if nothing earlier failed.
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close archive: %w", cerr)
		}
	}()

	var writer io.Writer = out
	var gzWriter *gzip.Writer
	var zstEncoder *zstd.Encoder

	switch algo {
	case Zstd:
		enc, zerr := zstd.NewWriter(out)
		if zerr != nil {
			return 0, fmt.Errorf("init zstd: %w", zerr)
		}
		zstEncoder = enc
		writer = enc
	case Gzip:
		gzWriter = gzip.NewWriter(out)
		writer = gzWriter
	case None:
		// no compression
	default:
		return 0, fmt.Errorf("unknown algorithm: %s", algo)
	}

	tw := tar.NewWriter(writer)
	// closeChain flushes the tar trailer and the compressor's buffered output.
	// These are exactly the writes that can fail on the final flush, so their
	// errors must be checked, not discarded. Idempotent via the closed guard.
	closed := false
	closeChain := func() error {
		if closed {
			return nil
		}
		closed = true
		if cerr := tw.Close(); cerr != nil {
			return fmt.Errorf("close tar: %w", cerr)
		}
		if gzWriter != nil {
			if cerr := gzWriter.Close(); cerr != nil {
				return fmt.Errorf("flush gzip: %w", cerr)
			}
		}
		if zstEncoder != nil {
			if cerr := zstEncoder.Close(); cerr != nil {
				return fmt.Errorf("flush zstd: %w", cerr)
			}
		}
		return nil
	}
	// Safety net for early-error paths (no-op once closeChain ran on success).
	defer func() {
		if cerr := closeChain(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	walkErr := filepath.Walk(srcDir, func(path string, info os.FileInfo, werr error) error {
		if werr != nil {
			return werr
		}
		if info.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(srcDir, path)
		if rerr != nil {
			return rerr
		}
		// Symlinks: record the link itself (no body) rather than dereferencing,
		// which would fail on dangling links and corrupt the entry otherwise.
		var linkTarget string
		isSymlink := info.Mode()&os.ModeSymlink != 0
		if isSymlink {
			linkTarget, rerr = os.Readlink(path)
			if rerr != nil {
				return fmt.Errorf("readlink %s: %w", rel, rerr)
			}
		}
		hdr, herr := tar.FileInfoHeader(info, linkTarget)
		if herr != nil {
			return herr
		}
		hdr.Name = rel
		if werr := tw.WriteHeader(hdr); werr != nil {
			return fmt.Errorf("write header for %s: %w", rel, werr)
		}
		if isSymlink {
			return nil // symlink tar entries have no body
		}
		f, oerr := os.Open(path)
		if oerr != nil {
			return oerr
		}
		defer f.Close() //nolint:errcheck
		n, cerr := io.Copy(tw, f)
		if cerr != nil {
			return fmt.Errorf("write %s: %w", rel, cerr)
		}
		totalSize += n
		return nil
	})
	if walkErr != nil {
		return 0, walkErr
	}
	// Flush + close the chain and surface any final-flush error BEFORE reporting
	// success, so a truncated archive is never reported as ok.
	if cerr := closeChain(); cerr != nil {
		return 0, cerr
	}
	return totalSize, nil
}

// Extract decompresses a tar archive to a target directory.
func Extract(archivePath, targetDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer f.Close() //nolint:errcheck

	// Detect format by extension
	ext := filepath.Ext(archivePath)
	var reader io.Reader = f
	var gzReader *gzip.Reader
	var zstDecoder *zstd.Decoder

	switch ext {
	case ".zst":
		dec, err := zstd.NewReader(f)
		if err != nil {
			return fmt.Errorf("init zstd decoder: %w", err)
		}
		zstDecoder = dec
		reader = dec
		defer zstDecoder.Close()
	case ".gz", ".tgz":
		gz, err := gzip.NewReader(f)
		if err != nil {
			return fmt.Errorf("init gzip: %w", err)
		}
		gzReader = gz
		reader = gz
		defer gzReader.Close() //nolint:errcheck
	default:
		// try to detect
	}

	tr := tar.NewReader(reader)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}
		target := filepath.Join(targetDir, hdr.Name)
		// Prevent directory traversal
		if !isSafePath(targetDir, target) {
			return fmt.Errorf("unsafe path in archive: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, hdr.FileInfo().Mode()); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, hdr.FileInfo().Mode())
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close() //nolint:errcheck
				return err
			}
			out.Close() //nolint:errcheck
		case tar.TypeSymlink:
			// Recreate the link, but refuse any that would resolve outside the
			// extraction root (classic tar symlink-escape attack). Unsafe links
			// are skipped rather than failing the whole extract.
			if !isSafeSymlink(targetDir, target, hdr.Linkname) {
				continue
			}
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			_ = os.Remove(target) // overwrite if it already exists
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		}
	}
	return nil
}

// isSafeSymlink reports whether a symlink at linkPath pointing to linkname stays
// within base. Absolute targets, and relative targets that resolve outside base,
// are rejected.
func isSafeSymlink(base, linkPath, linkname string) bool {
	if filepath.IsAbs(linkname) {
		return false
	}
	resolved := filepath.Join(filepath.Dir(linkPath), linkname)
	return isSafePath(base, resolved)
}

func isSafePath(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	if filepath.IsAbs(rel) {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
