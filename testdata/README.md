# Test Data

This directory contains test fixtures for integration tests.

## Required Files

Before running processor tests, add a test image:

### Option 1: Download a Sample Image

```bash
# Download a sample JPEG (small, CC0 licensed)
curl -L -o testdata/test.jpg "https://picsum.photos/800/600.jpg"

# Or use ImageMagick to create one
convert -size 800x600 xc:blue testdata/test.jpg
```

### Option 2: Use Your Own Image

Copy any JPEG image to `testdata/test.jpg`. Recommended size: 800x600 or larger.

### Option 3: Create a Minimal Test Image

```bash
# Using ImageMagick
convert -size 100x100 xc:red testdata/test.jpg

# Using FFmpeg
ffmpeg -f lavfi -i color=c=blue:s=100x100 -frames:v 1 testdata/test.jpg
```

## File Descriptions

| File | Purpose |
|------|---------|
| `test.jpg` | Standard test image for thumbnail/resize tests |
| `test.png` | PNG test image (optional) |
| `test.gif` | GIF test image (optional) |

## Note

Test images are gitignored to keep the repository small. Each developer needs to create their own test fixtures using the instructions above.

## Verification

After creating test images, verify they work:

```bash
# Check file exists and is valid
file testdata/test.jpg
# Should output: testdata/test.jpg: JPEG image data, ...

# Run processor tests
go test -v ./internal/processor/image/...
```
