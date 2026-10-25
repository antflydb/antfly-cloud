package sdk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const liteTransferChunk int64 = 8 << 20

// TransferOptions controls the byte transport separately from management API
// authentication. HTTPClient must not inject an Authorization header or log URLs.
type TransferOptions struct {
	HTTPClient *http.Client
	Progress   func(done, total int64)
}

func (o TransferOptions) client() *http.Client {
	client := &http.Client{Timeout: 2 * time.Minute}
	if o.HTTPClient != nil {
		copy := *o.HTTPClient
		client = &copy
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return client
}
func transferURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "storage.googleapis.com" || u.User != nil || u.Fragment != "" {
		return nil, fmt.Errorf("invalid scoped storage URL")
	}
	return u, nil
}
func transferSend(client *http.Client, request *http.Request) (*http.Response, error) {
	response, err := client.Do(request)
	if err != nil {
		if request.Context().Err() != nil {
			return nil, request.Context().Err()
		}
		return nil, fmt.Errorf("storage transfer interrupted; retry the same operation")
	}
	return response, nil
}
func uploadOffset(ctx context.Context, client *http.Client, session string, size int64) (int64, error) {
	u, err := transferURL(session)
	if err != nil {
		return 0, err
	}
	if u.Query().Get("upload_id") == "" {
		return 0, fmt.Errorf("upload session is missing its capability")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, u.String(), http.NoBody)
	if err != nil {
		return 0, fmt.Errorf("invalid upload status request")
	}
	request.Header.Set("Content-Range", fmt.Sprintf("bytes */%d", size))
	response, err := transferSend(client, request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode == 200 || response.StatusCode == 201 {
		return size, nil
	}
	if response.StatusCode != 308 {
		return 0, fmt.Errorf("storage upload status returned HTTP %d", response.StatusCode)
	}
	rangeValue := response.Header.Get("Range")
	if rangeValue == "" {
		return 0, nil
	}
	if !strings.HasPrefix(rangeValue, "bytes=0-") {
		return 0, fmt.Errorf("invalid storage upload offset")
	}
	last, err := strconv.ParseInt(strings.TrimPrefix(rangeValue, "bytes=0-"), 10, 64)
	if err != nil || last < 0 || last >= size {
		return 0, fmt.Errorf("invalid storage upload offset")
	}
	return last + 1, nil
}

// UploadLiteSnapshot accepts a closed, independent snapshot produced by
// `antfly lite snapshot`. It never copies a live database or changes models.
// Keep idempotencyKey stable when resuming. Only the operation ID is safe to log.
func (c *Client) UploadLiteSnapshot(ctx context.Context, org, path, idempotencyKey string, options TransferOptions) (*LiteOperation, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !stat.Mode().IsRegular() || stat.Size() <= 0 {
		return nil, fmt.Errorf("snapshot must be a nonempty regular file")
	}
	hash := sha256.New()
	n, err := io.CopyBuffer(hash, contextInput{ctx, file}, make([]byte, 1<<20))
	if err != nil {
		return nil, err
	}
	if n != stat.Size() {
		return nil, fmt.Errorf("snapshot changed while hashing")
	}
	op, err := c.StartLiteImport(ctx, org, StartLiteImportRequest{IdempotencyKey: idempotencyKey, SizeBytes: n, Sha256: hex.EncodeToString(hash.Sum(nil))})
	if err != nil {
		return nil, err
	}
	if string(op.Phase) != "uploading" {
		return op, nil
	}
	session, err := c.LiteUploadSession(ctx, org, op.Id.String())
	if err != nil {
		return op, err
	}
	client := options.client()
	offset, err := uploadOffset(ctx, client, session.UploadUrl, n)
	if err != nil {
		return op, err
	}
	failures := 0
	for offset < n {
		chunk := min(liteTransferChunk, n-offset)
		request, err := http.NewRequestWithContext(ctx, http.MethodPut, session.UploadUrl, io.NewSectionReader(file, offset, chunk))
		if err != nil {
			return op, fmt.Errorf("invalid upload request")
		}
		request.ContentLength = chunk
		request.Header.Set("Content-Type", "application/octet-stream")
		request.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, offset+chunk-1, n))
		response, sendErr := transferSend(client, request)
		if response != nil {
			response.Body.Close()
			if response.StatusCode != 308 && response.StatusCode != 200 && response.StatusCode != 201 {
				sendErr = fmt.Errorf("storage upload returned HTTP %d", response.StatusCode)
			}
		}
		if sendErr != nil {
			failures++
			if failures >= 5 {
				return op, sendErr
			}
			if err := transferDelay(ctx, failures); err != nil {
				return op, err
			}
		}
		// Even an ambiguous response is resolved using the committed offset;
		// never assume a failed request committed zero bytes or a full chunk.
		next, err := uploadOffset(ctx, client, session.UploadUrl, n)
		if err != nil {
			return op, err
		}
		if next < offset {
			return op, fmt.Errorf("storage upload offset moved backwards")
		}
		if next == offset && sendErr == nil {
			failures++
			if failures >= 5 {
				return op, fmt.Errorf("storage upload made no progress")
			}
		}
		if next > offset {
			failures = 0
		}
		offset = next
		if options.Progress != nil {
			options.Progress(offset, n)
		}
	}
	return c.SealLiteUpload(ctx, org, op.Id.String())
}

type contextInput struct {
	ctx context.Context
	r   io.Reader
}

func (r contextInput) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
func transferDelay(ctx context.Context, attempt int) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Duration(attempt) * time.Second):
		return nil
	}
}

// WaitLiteOperation returns at a user decision boundary, success, or failure.
// It never automatically confirms creation or switches an application's client.
func (c *Client) WaitLiteOperation(ctx context.Context, org, id string) (*LiteOperation, error) {
	for {
		op, err := c.LiteOperation(ctx, org, id)
		if err != nil {
			return nil, err
		}
		switch string(op.Phase) {
		case "awaiting_confirmation", "ready", "download_ready":
			return op, nil
		case "failed", "cancelled", "expired":
			return op, fmt.Errorf("Lite operation %s is %s; inspect its dependency report and error", id, op.Phase)
		}
		if err := transferDelay(ctx, 2); err != nil {
			return op, err
		}
	}
}
