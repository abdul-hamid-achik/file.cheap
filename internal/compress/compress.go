// Package compress provides tar+zstd archiving for stash content.
package compress

import (
	"archive/tar"
	"compress/gzip"
	"context"
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

// maxExtractedBytes caps both total bytes written during Extract and logical
// bytes traversed during archive inspection, as defense-in-depth against a
// decompression bomb. It is far above any realistic agent-workflow stash. A var
// (not const) so tests can lower it.
var maxExtractedBytes int64 = 20 << 30 // 20 GiB

// maxArchiveInspectionBytes caps the decompressed stream consumed while
// inspecting tar metadata. It includes file bodies skipped by archive/tar,
// padding, and PAX/GNU extension records that Next consumes internally. The
// extra GiB above maxExtractedBytes leaves ample room for valid tar overhead.
// A var lets tests exercise the failure path.
var maxArchiveInspectionBytes int64 = 21 << 30 // 21 GiB

// maxArchiveScanHeaders bounds metadata-only archive inspection. The manifest
// itself is capped at 64 MiB, so a valid stash cannot approach this number of
// entries. A var lets tests exercise the failure path.
var maxArchiveScanHeaders = 1_000_000

// Archive creates a compressed tar archive from a directory.
// The archive is written to outputPath. If algo is None, creates an uncompressed tar.
//
// The compression chain (tar writer -> compressor -> file) is flushed and closed
// explicitly before success is reported: a failure on the final flush (e.g.
// ENOSPC) surfaces as a non-nil error rather than a silently truncated archive.
func Archive(srcDir, outputPath string, algo Algorithm) (totalSize int64, err error) {
	return ArchiveContext(context.Background(), srcDir, outputPath, algo)
}

// ArchiveContext is Archive with cancellation support during traversal and
// file copying.
func ArchiveContext(ctx context.Context, srcDir, outputPath string, algo Algorithm) (totalSize int64, err error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
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
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if werr != nil {
			return werr
		}
		rel, rerr := filepath.Rel(srcDir, path)
		if rerr != nil {
			return rerr
		}
		if rel == "." {
			return nil
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
		hdr.Name = filepath.ToSlash(rel)
		if info.IsDir() {
			// Directory headers preserve empty directories and their permissions;
			// omitting them made a compressed round-trip lose empty trees.
			hdr.Name += "/"
			return tw.WriteHeader(hdr)
		}
		if werr := tw.WriteHeader(hdr); werr != nil {
			return fmt.Errorf("write header for %s: %w", rel, werr)
		}
		if isSymlink {
			return nil // symlink tar entries have no body
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported file type at %q: %s", path, info.Mode().Type())
		}
		f, oerr := os.Open(path)
		if oerr != nil {
			return oerr
		}
		defer f.Close() //nolint:errcheck
		n, cerr := io.Copy(tw, &contextReader{ctx: ctx, reader: f})
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
	return ExtractContext(context.Background(), archivePath, targetDir)
}

// HasRegularFileContext reports whether an archive contains name as a regular
// file. It streams tar headers without extracting files, so callers can verify
// metadata that names a file in a compressed stash without materializing it.
func HasRegularFileContext(ctx context.Context, archivePath, name string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return false, fmt.Errorf("open archive: %w", err)
	}
	defer f.Close() //nolint:errcheck

	ext := filepath.Ext(archivePath)
	var reader io.Reader = f
	var gzReader *gzip.Reader
	var zstDecoder *zstd.Decoder
	switch ext {
	case ".zst":
		dec, err := zstd.NewReader(f)
		if err != nil {
			return false, fmt.Errorf("init zstd decoder: %w", err)
		}
		zstDecoder = dec
		reader = dec
		defer zstDecoder.Close()
	case ".gz", ".tgz":
		gz, err := gzip.NewReader(f)
		if err != nil {
			return false, fmt.Errorf("init gzip: %w", err)
		}
		gzReader = gz
		reader = gz
		defer gzReader.Close() //nolint:errcheck
	}

	tr := tar.NewReader(&archiveInspectionReader{
		ctx:       ctx,
		reader:    reader,
		remaining: maxArchiveInspectionBytes,
		limit:     maxArchiveInspectionBytes,
	})
	var scannedBytes int64
	var scannedHeaders int
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		hdr, err := tr.Next()
		if err == io.EOF {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("read tar: %w", err)
		}
		scannedHeaders++
		if scannedHeaders > maxArchiveScanHeaders {
			return false, fmt.Errorf("archive exceeds %d-header inspection cap", maxArchiveScanHeaders)
		}
		if hdr.Size < 0 || hdr.Size > maxExtractedBytes-scannedBytes {
			return false, fmt.Errorf("archive exceeds %d-byte inspection cap (possible decompression bomb)", maxExtractedBytes)
		}
		scannedBytes += hdr.Size
		if filepath.ToSlash(hdr.Name) == name && hdr.FileInfo().Mode().IsRegular() {
			return true, nil
		}
	}
}

// ExtractContext is Extract with cancellation support between entries and
// during decompression writes.
func ExtractContext(ctx context.Context, archivePath, targetDir string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
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
	var extracted int64 // running total, bounded by maxExtractedBytes
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
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
			if err := ensureNoSymlinkPath(targetDir, target); err != nil {
				return err
			}
			if err := os.MkdirAll(target, hdr.FileInfo().Mode()); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := ensureNoSymlinkPath(targetDir, filepath.Dir(target)); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			out, err := os.CreateTemp(filepath.Dir(target), ".fcheap-extract-*")
			if err != nil {
				return err
			}
			tmpPath := out.Name()
			// Bound each entry by the remaining budget so a bomb can't write past
			// the cap. CopyN(remaining+1) returns >remaining only when over budget.
			remaining := maxExtractedBytes - extracted
			n, cerr := io.CopyN(out, &contextReader{ctx: ctx, reader: tr}, remaining+1)
			extracted += n
			if n > remaining {
				_ = out.Close()
				_ = os.Remove(tmpPath)
				return fmt.Errorf("archive exceeds %d-byte extraction cap (possible decompression bomb)", maxExtractedBytes)
			}
			if cerr != nil && cerr != io.EOF {
				_ = out.Close()
				_ = os.Remove(tmpPath)
				return cerr
			}
			if err := out.Chmod(hdr.FileInfo().Mode()); err != nil {
				_ = out.Close()
				_ = os.Remove(tmpPath)
				return err
			}
			if err := out.Close(); err != nil {
				_ = os.Remove(tmpPath)
				return err
			}
			// Rename replaces a planted leaf symlink without following it and
			// ensures a failed extraction never exposes a partial file.
			if err := os.Rename(tmpPath, target); err != nil {
				_ = os.Remove(tmpPath)
				return err
			}
		case tar.TypeSymlink:
			// Recreate the link, but refuse any that would resolve outside the
			// extraction root (classic tar symlink-escape attack). Unsafe links
			// are skipped rather than failing the whole extract.
			if !isSafeSymlink(targetDir, target, hdr.Linkname) {
				continue
			}
			if err := ensureNoSymlinkPath(targetDir, filepath.Dir(target)); err != nil {
				return err
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

// ensureNoSymlinkPath rejects an existing symlink in any path component below
// base. Lexical traversal checks alone are insufficient: target/sub/file is
// lexically inside target even when target/sub is a planted link to elsewhere.
func ensureNoSymlinkPath(base, target string) error {
	rel, err := filepath.Rel(base, target)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("unsafe extraction path: %s", target)
	}
	if rel == "." {
		return nil
	}
	current := base
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect extraction path %q: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to extract through symlink path component %q", current)
		}
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

// archiveInspectionReader bounds every decompressed byte consumed by
// archive/tar, including metadata records and implicit entry-body skips that
// are not visible to callers of Reader.Next.
type archiveInspectionReader struct {
	ctx       context.Context
	reader    io.Reader
	remaining int64
	limit     int64
}

func (r *archiveInspectionReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if r.remaining <= 0 {
		var probe [1]byte
		for {
			n, err := r.reader.Read(probe[:])
			if n > 0 {
				return 0, fmt.Errorf(
					"archive exceeds %d-byte decompressed inspection cap (possible decompression bomb)",
					r.limit,
				)
			}
			if err != nil {
				return 0, err
			}
			if err := r.ctx.Err(); err != nil {
				return 0, err
			}
		}
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.reader.Read(p)
	r.remaining -= int64(n)
	return n, err
}
