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
func Archive(srcDir, outputPath string, algo Algorithm) (int64, error) {
	out, err := os.Create(outputPath)
	if err != nil {
		return 0, fmt.Errorf("create archive: %w", err)
	}
	defer out.Close() //nolint:errcheck

	var writer io.Writer = out
	var gzWriter *gzip.Writer
	var zstEncoder *zstd.Encoder

	switch algo {
	case Zstd:
		enc, err := zstd.NewWriter(out)
		if err != nil {
			return 0, fmt.Errorf("init zstd: %w", err)
		}
		zstEncoder = enc
		writer = enc
	case Gzip:
		gz := gzip.NewWriter(out)
		gzWriter = gz
		writer = gz
	case None:
		// no compression
	default:
		return 0, fmt.Errorf("unknown algorithm: %s", algo)
	}

	tw := tar.NewWriter(writer)
	defer func() {
		tw.Close() //nolint:errcheck
		if gzWriter != nil {
			gzWriter.Close() //nolint:errcheck
		}
		if zstEncoder != nil {
			zstEncoder.Close() //nolint:errcheck
		}
	}()

	var totalSize int64
	err = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = rel
		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("write header for %s: %w", rel, err)
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close() //nolint:errcheck
		n, err := io.Copy(tw, f)
		if err != nil {
			return fmt.Errorf("write %s: %w", rel, err)
		}
		totalSize += n
		return nil
	})
	if err != nil {
		return 0, err
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
			// Skip symlinks for security
			continue
		}
	}
	return nil
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
