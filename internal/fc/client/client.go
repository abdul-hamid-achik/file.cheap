package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/fc/version"
)

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

func (c *Client) SetAPIKey(apiKey string) {
	c.apiKey = apiKey
}

func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader, contentType string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}

	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("User-Agent", "fc-cli/"+version.Short())

	return c.httpClient.Do(req)
}

func (c *Client) doJSON(ctx context.Context, method, path string, reqBody, respBody interface{}) error {
	var body io.Reader
	if reqBody != nil {
		data, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}

	resp, err := c.doRequest(ctx, method, path, body, "application/json")
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return c.parseError(resp)
	}

	if respBody != nil && resp.StatusCode != http.StatusNoContent {
		return json.NewDecoder(resp.Body).Decode(respBody)
	}
	return nil
}

func (c *Client) parseError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	var errResp ErrorResponse
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error.Message != "" {
		return fmt.Errorf("%s: %s", errResp.Error.Code, errResp.Error.Message)
	}
	return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
}

func (c *Client) Upload(ctx context.Context, filePath string, transforms []string, wait bool) (*UploadResponse, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer func() { _ = file.Close() }()

	// Use io.Pipe for streaming - avoids buffering entire file in memory
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	// Channel to collect any errors from the writing goroutine
	errCh := make(chan error, 1)

	// Write the multipart form in a separate goroutine
	go func() {
		defer func() { _ = pw.Close() }()

		part, err := writer.CreateFormFile("file", filepath.Base(filePath))
		if err != nil {
			errCh <- err
			return
		}

		if _, err := io.Copy(part, file); err != nil {
			errCh <- err
			return
		}

		for _, t := range transforms {
			if err := writer.WriteField("transforms", t); err != nil {
				errCh <- err
				return
			}
		}

		if wait {
			if err := writer.WriteField("wait", "true"); err != nil {
				errCh <- err
				return
			}
		}

		errCh <- writer.Close()
	}()

	resp, err := c.doRequest(ctx, http.MethodPost, "/api/v1/upload", pr, writer.FormDataContentType())
	if err != nil {
		return nil, fmt.Errorf("upload failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Check for write errors from the goroutine
	if writeErr := <-errCh; writeErr != nil {
		return nil, fmt.Errorf("failed to write multipart form: %w", writeErr)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
		return nil, c.parseError(resp)
	}

	var result UploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.URL == "" {
		result.URL = fmt.Sprintf("%s/cdn/%s/_/%s", c.baseURL, result.ID, filepath.Base(filePath))
	}

	return &result, nil
}

func (c *Client) UploadReader(ctx context.Context, r io.Reader, filename string, size int64, transforms []string, wait bool) (*UploadResponse, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, err
	}

	if _, err := io.Copy(part, r); err != nil {
		return nil, err
	}

	for _, t := range transforms {
		if err := writer.WriteField("transforms", t); err != nil {
			return nil, err
		}
	}

	if wait {
		if err := writer.WriteField("wait", "true"); err != nil {
			return nil, err
		}
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	resp, err := c.doRequest(ctx, http.MethodPost, "/api/v1/upload", &buf, writer.FormDataContentType())
	if err != nil {
		return nil, fmt.Errorf("upload failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
		return nil, c.parseError(resp)
	}

	var result UploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.URL == "" {
		result.URL = fmt.Sprintf("%s/cdn/%s/_/%s", c.baseURL, result.ID, filename)
	}

	return &result, nil
}

func (c *Client) ListFiles(ctx context.Context, limit, offset int, status, search string) (*ListFilesResponse, error) {
	params := url.Values{}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		params.Set("offset", strconv.Itoa(offset))
	}
	if status != "" {
		params.Set("status", status)
	}
	if search != "" {
		params.Set("search", search)
	}

	path := "/api/v1/files"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var result ListFilesResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetFile(ctx context.Context, fileID string) (*File, error) {
	var result File
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/files/"+fileID, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) DeleteFile(ctx context.Context, fileID string) error {
	return c.doJSON(ctx, http.MethodDelete, "/api/v1/files/"+fileID, nil, nil)
}

func (c *Client) Download(ctx context.Context, fileID, variant string) (io.ReadCloser, string, error) {
	path := "/api/v1/files/" + fileID + "/download"
	if variant != "" {
		path += "?variant=" + url.QueryEscape(variant)
	}

	resp, err := c.doRequest(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, "", err
	}

	if resp.StatusCode >= 400 {
		defer func() { _ = resp.Body.Close() }()
		return nil, "", c.parseError(resp)
	}

	filename := ""
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if strings.Contains(cd, "filename=") {
			parts := strings.Split(cd, "filename=")
			if len(parts) > 1 {
				filename = strings.Trim(parts[1], `"`)
			}
		}
	}

	return resp.Body, filename, nil
}

func (c *Client) Transform(ctx context.Context, fileID string, req *TransformRequest) (*TransformResponse, error) {
	var result TransformResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/files/"+fileID+"/transform", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) BatchTransform(ctx context.Context, req *BatchTransformRequest) (*BatchTransformResponse, error) {
	var result BatchTransformResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/batch/transform", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetBatchStatus(ctx context.Context, batchID string, includeItems bool) (*BatchStatusResponse, error) {
	path := "/api/v1/batch/" + batchID
	if includeItems {
		path += "?include_items=true"
	}

	var result BatchStatusResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetJobStatus(ctx context.Context, jobID string) (*JobStatus, error) {
	var result JobStatus
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/jobs/"+jobID, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) CreateShare(ctx context.Context, fileID string, expires string) (*ShareResponse, error) {
	path := "/api/v1/files/" + fileID + "/share"
	if expires != "" {
		path += "?expires=" + url.QueryEscape(expires)
	}

	var result ShareResponse
	if err := c.doJSON(ctx, http.MethodPost, path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) DeviceAuth(ctx context.Context) (*DeviceAuthResponse, error) {
	var result DeviceAuthResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/auth/device", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) DeviceToken(ctx context.Context, deviceCode string) (*DeviceTokenResponse, error) {
	reqBody := map[string]string{"device_code": deviceCode}
	var result DeviceTokenResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/auth/device/token", reqBody, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) WaitForFile(ctx context.Context, fileID string, pollInterval time.Duration, timeout time.Duration) (*File, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			file, err := c.GetFile(ctx, fileID)
			if err != nil {
				return nil, err
			}
			if file.Status == "completed" || file.Status == "failed" {
				return file, nil
			}
		}
	}
}

func (c *Client) WaitForBatch(ctx context.Context, batchID string, pollInterval time.Duration, timeout time.Duration) (*BatchStatusResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			status, err := c.GetBatchStatus(ctx, batchID, true)
			if err != nil {
				return nil, err
			}
			if status.Status == "completed" || status.Status == "failed" || status.Status == "partial" {
				return status, nil
			}
		}
	}
}

// Video methods

func (c *Client) VideoTranscode(ctx context.Context, fileID string, req *VideoTranscodeRequest) (*VideoTranscodeResponse, error) {
	var result VideoTranscodeResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/files/"+fileID+"/video/transcode", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Chunked upload methods for large files

func (c *Client) InitChunkedUpload(ctx context.Context, req *ChunkedUploadInitRequest) (*ChunkedUploadInitResponse, error) {
	var result ChunkedUploadInitResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/upload/chunked", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) UploadChunk(ctx context.Context, uploadID string, chunkIndex int, data []byte) (*ChunkedUploadChunkResponse, error) {
	path := fmt.Sprintf("/api/v1/upload/chunked/%s?chunk=%d", uploadID, chunkIndex)
	resp, err := c.doRequest(ctx, http.MethodPut, path, bytes.NewReader(data), "application/octet-stream")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return nil, c.parseError(resp)
	}

	var result ChunkedUploadChunkResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetChunkedUploadStatus(ctx context.Context, uploadID string) (*ChunkedUploadStatusResponse, error) {
	var result ChunkedUploadStatusResponse
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/upload/chunked/"+uploadID, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) CancelChunkedUpload(ctx context.Context, uploadID string) error {
	return c.doJSON(ctx, http.MethodDelete, "/api/v1/upload/chunked/"+uploadID, nil, nil)
}

// UploadLargeFile uploads a file using chunked upload for large files
func (c *Client) UploadLargeFile(ctx context.Context, filePath string, onProgress func(uploaded, total int64)) (*UploadResponse, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer func() { _ = file.Close() }()

	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	// Detect content type from extension
	contentType := "application/octet-stream"
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".mp4":
		contentType = "video/mp4"
	case ".mov":
		contentType = "video/quicktime"
	case ".avi":
		contentType = "video/x-msvideo"
	case ".mkv":
		contentType = "video/x-matroska"
	case ".webm":
		contentType = "video/webm"
	case ".wmv":
		contentType = "video/x-ms-wmv"
	case ".flv":
		contentType = "video/x-flv"
	}

	// Initialize chunked upload
	initResp, err := c.InitChunkedUpload(ctx, &ChunkedUploadInitRequest{
		Filename:    filepath.Base(filePath),
		ContentType: contentType,
		TotalSize:   stat.Size(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize chunked upload: %w", err)
	}

	// Upload chunks
	chunkSize := initResp.ChunkSize
	buffer := make([]byte, chunkSize)
	var uploaded int64

	for i := 0; i < initResp.ChunksTotal; i++ {
		n, err := file.Read(buffer)
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("failed to read chunk %d: %w", i, err)
		}
		if n == 0 {
			break
		}

		chunkResp, err := c.UploadChunk(ctx, initResp.UploadID, i, buffer[:n])
		if err != nil {
			return nil, fmt.Errorf("failed to upload chunk %d: %w", i, err)
		}

		uploaded += int64(n)
		if onProgress != nil {
			onProgress(uploaded, stat.Size())
		}

		if chunkResp.Complete {
			return &UploadResponse{
				ID:       chunkResp.FileID,
				Filename: filepath.Base(filePath),
				Status:   "pending",
			}, nil
		}
	}

	return nil, fmt.Errorf("upload completed but no file ID returned")
}
