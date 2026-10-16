package sdk

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// DownloadLiteFile resumes into owned scratch storage beside destination. Only
// a fully verified file is atomically linked into place, without replacement.
// It deliberately leaves a small receipt/lock directory for retry provenance.
func (c *Client) DownloadLiteFile(ctx context.Context, org, operation, destination string, options TransferOptions) error {
	if _, err := os.Lstat(destination); err == nil {
		return fmt.Errorf("destination exists; choose a new file")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	link, err := c.LiteDownload(ctx, org, operation)
	if err != nil {
		return err
	}
	if link.SizeBytes <= 0 || len(link.Sha256) != 64 {
		return fmt.Errorf("invalid download identity")
	}
	identity, _ := json.Marshal(struct {
		Org, Operation, SHA256 string
		Size                   int64
	}{org, operation, link.Sha256, link.SizeBytes})
	dir := destination + ".antfly-download"
	created := false
	if err := os.Mkdir(dir, 0700); err == nil {
		created = true
	} else if !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("download scratch path is not an owned directory")
	}
	receipt := filepath.Join(dir, "receipt.json")
	if created {
		f, err := os.OpenFile(receipt, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, writeErr := f.Write(identity)
		syncErr := f.Sync()
		closeErr := f.Close()
		if writeErr != nil {
			return writeErr
		}
		if syncErr != nil {
			return syncErr
		}
		if closeErr != nil {
			return closeErr
		}
	} else {
		if info, err := os.Lstat(receipt); err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("download scratch receipt must be a regular file")
		}
		f, err := os.Open(receipt)
		if err != nil {
			return fmt.Errorf("download scratch directory has no valid receipt")
		}
		raw, readErr := io.ReadAll(io.LimitReader(f, 4096))
		f.Close()
		if readErr != nil || !bytes.Equal(raw, identity) {
			return fmt.Errorf("download scratch belongs to a different immutable source; choose a new destination")
		}
	}
	lockPath := filepath.Join(dir, "lock")
	if info, err := os.Lstat(lockPath); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("download scratch lock must be a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	unlock, err := lockDownload(ctx, lockPath)
	if err != nil {
		return err
	}
	defer unlock()
	if _, err := os.Lstat(destination); err == nil {
		return fmt.Errorf("destination exists; choose a new file")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	partPath := filepath.Join(dir, "content.part")
	if info, err := os.Lstat(partPath); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("download staging must be a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	part, err := os.OpenFile(partPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer part.Close()
	info, err = part.Stat()
	if err != nil {
		return err
	}
	offset := info.Size()
	if offset > link.SizeBytes {
		return fmt.Errorf("download staging exceeds admitted size")
	}
	client := options.client()
	failures := 0
	for offset < link.SizeBytes {
		if _, err := transferURL(link.DownloadUrl); err != nil {
			return err
		}
		end := min(offset+liteTransferChunk, link.SizeBytes) - 1
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, link.DownloadUrl, nil)
		if err != nil {
			return fmt.Errorf("invalid download request")
		}
		request.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, end))
		response, sendErr := transferSend(client, request)
		if sendErr == nil {
			if response.StatusCode != http.StatusPartialContent || response.Header.Get("Content-Range") != fmt.Sprintf("bytes %d-%d/%d", offset, end, link.SizeBytes) {
				response.Body.Close()
				sendErr = fmt.Errorf("storage download did not honor the requested byte range")
			} else {
				if _, err := part.Seek(offset, io.SeekStart); err != nil {
					response.Body.Close()
					return err
				}
				count, copyErr := io.CopyBuffer(part, contextInput{ctx, io.LimitReader(response.Body, end-offset+2)}, make([]byte, 1<<20))
				response.Body.Close()
				if count > end-offset+1 {
					return fmt.Errorf("storage download exceeded requested range")
				}
				if err := part.Sync(); err != nil {
					return err
				}
				offset += count
				if copyErr != nil {
					sendErr = fmt.Errorf("storage download interrupted; resume the same operation")
				} else if offset != end+1 {
					sendErr = fmt.Errorf("storage download ended early")
				}
			}
		}
		if sendErr != nil {
			failures++
			if failures >= 5 {
				return sendErr
			}
			if err := transferDelay(ctx, failures); err != nil {
				return err
			}
			fresh, err := c.LiteDownload(ctx, org, operation)
			if err != nil {
				return err
			}
			if fresh.SizeBytes != link.SizeBytes || fresh.Sha256 != link.Sha256 {
				return fmt.Errorf("download identity changed")
			}
			link = fresh
		} else {
			failures = 0
		}
		if options.Progress != nil {
			options.Progress(offset, link.SizeBytes)
		}
	}
	if _, err := part.Seek(0, io.SeekStart); err != nil {
		return err
	}
	hash := sha256.New()
	count, err := io.CopyBuffer(hash, contextInput{ctx, part}, make([]byte, 1<<20))
	if err != nil {
		return err
	}
	if count != link.SizeBytes || hex.EncodeToString(hash.Sum(nil)) != link.Sha256 {
		return fmt.Errorf("download checksum mismatch; destination was not created")
	}
	if err := part.Sync(); err != nil {
		return err
	}
	if err := part.Close(); err != nil {
		return err
	}
	if err := os.Link(partPath, destination); err != nil {
		return fmt.Errorf("publish verified file without replacement: %w", err)
	}
	parent, err := os.Open(filepath.Dir(destination))
	if err == nil {
		err = syncDownloadDirectory(parent)
		parent.Close()
	}
	if err != nil {
		return err
	}
	return os.Remove(partPath)
}
