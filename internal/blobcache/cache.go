package blobcache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/septi0/dockvmap/internal/model"
)

type Store interface {
	DeleteCachedBlob(ctx context.Context, digest string) error
	DeleteCachedBlobIfExpired(ctx context.Context, digest string, before time.Time) (bool, error)
	GetCachedBlob(ctx context.Context, digest string) (*model.CachedBlob, error)
	ListExpiredCachedBlobs(ctx context.Context, before time.Time) ([]string, error)
	ListCachedBlobDigestsWithPrefix(ctx context.Context, prefix string) ([]string, error)
	ListCachedBlobsByAccessAsc(ctx context.Context, limit int) ([]model.CachedBlob, error)
	SumCachedBlobSize(ctx context.Context) (int64, error)
	SaveCachedBlob(ctx context.Context, blob model.CachedBlob) error
	TouchCachedBlob(ctx context.Context, digest string, accessedAt time.Time) error
}

type Cache struct {
	dir      string
	lifetime time.Duration
	maxSize  int64
	store    Store
	mu       sync.Mutex
}

const metadataWriteTimeout = 5 * time.Second
const orphanGracePeriod = 15 * time.Minute
const maxSizeEvictionBatch = 1000

func New(dir string, lifetime string, maxSize int64, store Store) (*Cache, error) {
	lifetimeDuration, err := time.ParseDuration(lifetime)

	if err != nil {
		return nil, fmt.Errorf("invalid cache lifetime: %w", err)
	}

	if lifetimeDuration <= 0 {
		return nil, fmt.Errorf("cache lifetime must be positive")
	}

	if maxSize < 0 {
		return nil, fmt.Errorf("cache max size must not be negative")
	}

	if err := os.MkdirAll(filepath.Join(dir, "blobs", "sha256"), 0o755); err != nil {
		return nil, fmt.Errorf("creating cache directory: %w", err)
	}

	return &Cache{dir: dir, lifetime: lifetimeDuration, maxSize: maxSize, store: store}, nil
}

func (c *Cache) Serve(ctx context.Context, rw http.ResponseWriter, r *http.Request, digest string) (bool, error) {
	if !cacheableDigest(digest) {
		return false, nil
	}

	blob, err := c.store.GetCachedBlob(ctx, digest)

	if err != nil || blob == nil {
		return false, err
	}

	path := c.path(digest)
	file, err := os.Open(path)

	if err != nil {
		if os.IsNotExist(err) {
			return false, c.store.DeleteCachedBlob(ctx, digest)
		}

		return false, fmt.Errorf("opening cached blob %q: %w", digest, err)
	}

	defer file.Close()

	now := time.Now().UTC()

	if !blob.AccessedAt.After(now.Add(-c.lifetime)) {
		_ = os.Remove(path)

		return false, c.store.DeleteCachedBlob(ctx, digest)
	}

	if err := c.store.TouchCachedBlob(ctx, digest, now); err != nil {
		return false, err
	}

	rw.Header().Set("Docker-Content-Digest", blob.Digest)

	if blob.ContentType != "" {
		rw.Header().Set("Content-Type", blob.ContentType)
	}

	http.ServeContent(rw, r, digest, blob.CreatedAt, file)

	return true, nil
}

type StreamOutcome struct {
	CacheErr error
	CopyErr  error
}

func (c *Cache) StreamAndStore(ctx context.Context, destination io.Writer, digest, contentType string, source io.Reader) StreamOutcome {
	writer, err := c.newWriter(digest)

	if err != nil {
		return StreamOutcome{CacheErr: err, CopyErr: copyStream(destination, source)}
	}

	var cacheErr error
	buffer := make([]byte, 32*1024)

	for {
		count, readErr := source.Read(buffer)

		if count > 0 {
			if _, err := destination.Write(buffer[:count]); err != nil {
				if writer != nil {
					writer.abort()
				}

				return StreamOutcome{CacheErr: cacheErr, CopyErr: err}
			}

			if writer != nil {
				if _, err := writer.Write(buffer[:count]); err != nil {
					cacheErr = err
					writer.abort()
					writer = nil
				}
			}
		}

		if readErr == io.EOF {
			break
		}

		if readErr != nil {
			if writer != nil {
				writer.abort()
			}

			return StreamOutcome{CacheErr: cacheErr, CopyErr: readErr}
		}
	}

	if writer != nil {
		if err := writer.commit(ctx, contentType); err != nil {
			cacheErr = err
		}
	}

	return StreamOutcome{CacheErr: cacheErr}
}

func (c *Cache) Cleanup(ctx context.Context) (int, error) {
	c.mu.Lock()

	defer c.mu.Unlock()

	before := time.Now().UTC().Add(-c.lifetime)
	digests, err := c.store.ListExpiredCachedBlobs(ctx, before)

	if err != nil {
		return 0, err
	}

	deleted := 0

	for _, digest := range digests {
		removed, err := c.store.DeleteCachedBlobIfExpired(ctx, digest, before)

		if err != nil {
			return deleted, err
		}

		if !removed {
			continue
		}

		if err := os.Remove(c.path(digest)); err != nil && !os.IsNotExist(err) {
			return deleted, fmt.Errorf("removing cached blob %q: %w", digest, err)
		}

		deleted++
	}

	if c.maxSize > 0 {
		evicted, err := c.evictToMaxSize(ctx)

		deleted += evicted

		if err != nil {
			return deleted, err
		}
	}

	return deleted, nil
}

// decrementing total locally is safe here: Cleanup holds c.mu, so no concurrent commit changes sizes
func (c *Cache) evictToMaxSize(ctx context.Context) (int, error) {
	total, err := c.store.SumCachedBlobSize(ctx)

	if err != nil {
		return 0, err
	}

	target := c.maxSize - c.maxSize/10
	evicted := 0

	for total > target {
		if err := ctx.Err(); err != nil {
			return evicted, err
		}

		blobs, err := c.store.ListCachedBlobsByAccessAsc(ctx, maxSizeEvictionBatch)

		if err != nil {
			return evicted, err
		}

		if len(blobs) == 0 {
			break
		}

		for _, blob := range blobs {
			if err := c.store.DeleteCachedBlob(ctx, blob.Digest); err != nil {
				return evicted, err
			}

			if err := os.Remove(c.path(blob.Digest)); err != nil && !os.IsNotExist(err) {
				return evicted, fmt.Errorf("removing cached blob %q: %w", blob.Digest, err)
			}

			total -= blob.Size
			evicted++

			if total <= target {
				break
			}
		}
	}

	return evicted, nil
}

func (c *Cache) Usage(ctx context.Context) (used int64, max int64, err error) {
	used, err = c.store.SumCachedBlobSize(ctx)

	if err != nil {
		return 0, 0, err
	}

	return used, c.maxSize, nil
}

func (c *Cache) ScanOrphans(ctx context.Context) (int, error) {
	root := filepath.Join(c.dir, "blobs", "sha256")

	shardEntries, err := os.ReadDir(root)

	if err != nil {
		return 0, fmt.Errorf("reading blob cache directory: %w", err)
	}

	cutoff := time.Now().Add(-orphanGracePeriod)
	removed := 0

	for _, shard := range shardEntries {
		if !shard.IsDir() {
			continue
		}

		n, err := c.scanShardOrphans(ctx, shard.Name(), cutoff)

		if err != nil {
			return removed, err
		}

		removed += n
	}

	return removed, nil
}

func (c *Cache) scanShardOrphans(ctx context.Context, shard string, cutoff time.Time) (int, error) {
	shardPath := filepath.Join(c.dir, "blobs", "sha256", shard)

	fileEntries, err := os.ReadDir(shardPath)

	if err != nil {
		return 0, fmt.Errorf("reading blob cache shard %q: %w", shard, err)
	}

	if len(fileEntries) == 0 {
		return 0, nil
	}

	known, err := c.store.ListCachedBlobDigestsWithPrefix(ctx, "sha256:"+shard)

	if err != nil {
		return 0, err
	}

	knownSet := make(map[string]struct{}, len(known))

	for _, digest := range known {
		knownSet[digest] = struct{}{}
	}

	removed := 0

	for _, f := range fileEntries {
		if f.IsDir() {
			continue
		}

		digest := "sha256:" + f.Name()

		if !IsDigest(digest) {
			continue
		}

		if _, ok := knownSet[digest]; ok {
			continue
		}

		info, err := f.Info()

		if err != nil {
			return removed, fmt.Errorf("stat cached blob %q: %w", digest, err)
		}

		if info.ModTime().After(cutoff) {
			continue
		}

		if err := os.Remove(filepath.Join(shardPath, f.Name())); err != nil && !os.IsNotExist(err) {
			return removed, fmt.Errorf("removing orphaned cached blob %q: %w", digest, err)
		}

		removed++
	}

	return removed, nil
}

type writer struct {
	cache  *Cache
	digest string
	temp   *os.File
	hash   hash.Hash
	size   int64
}

func (c *Cache) newWriter(digest string) (*writer, error) {
	if !cacheableDigest(digest) {
		return nil, fmt.Errorf("unsupported cache digest %q", digest)
	}

	dir := filepath.Dir(c.path(digest))

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating cached blob directory: %w", err)
	}

	temp, err := os.CreateTemp(dir, ".blob-*")

	if err != nil {
		return nil, fmt.Errorf("creating cached blob: %w", err)
	}

	return &writer{cache: c, digest: digest, temp: temp, hash: sha256.New()}, nil
}

func (w *writer) Write(data []byte) (int, error) {
	count, err := w.temp.Write(data)

	if count > 0 {
		_, _ = w.hash.Write(data[:count])
		w.size += int64(count)
	}

	return count, err
}

func (w *writer) commit(ctx context.Context, contentType string) error {
	if err := w.temp.Close(); err != nil {
		w.abort()

		return fmt.Errorf("closing cached blob: %w", err)
	}

	expected := strings.TrimPrefix(w.digest, "sha256:")

	if actual := hex.EncodeToString(w.hash.Sum(nil)); actual != expected {
		w.abort()

		return fmt.Errorf("cached blob digest mismatch")
	}

	w.cache.mu.Lock()
	defer w.cache.mu.Unlock()

	if err := os.Rename(w.temp.Name(), w.cache.path(w.digest)); err != nil {
		w.abort()

		return fmt.Errorf("publishing cached blob: %w", err)
	}

	now := time.Now().UTC()
	metadataCtx, cancel := context.WithTimeout(context.Background(), metadataWriteTimeout)

	defer cancel()

	if err := w.cache.store.SaveCachedBlob(metadataCtx, model.CachedBlob{
		Digest:      w.digest,
		Size:        w.size,
		ContentType: contentType,
		CreatedAt:   now,
		AccessedAt:  now,
	}); err != nil {
		_ = os.Remove(w.cache.path(w.digest))

		return err
	}

	return nil
}

func (w *writer) abort() {
	if w.temp == nil {
		return
	}

	_ = w.temp.Close()
	_ = os.Remove(w.temp.Name())

	w.temp = nil
}

func (c *Cache) path(digest string) string {
	hexDigest := strings.TrimPrefix(digest, "sha256:")

	return filepath.Join(c.dir, "blobs", "sha256", hexDigest[:2], hexDigest)
}

func IsDigest(digest string) bool {
	algorithm, value, found := strings.Cut(digest, ":")

	if !found || algorithm != "sha256" || len(value) != sha256.Size*2 {
		return false
	}

	_, err := hex.DecodeString(value)

	return err == nil && value == strings.ToLower(value)
}

func cacheableDigest(digest string) bool {
	return IsDigest(digest)
}

func copyStream(destination io.Writer, source io.Reader) error {
	_, err := io.Copy(destination, source)

	return err
}

func (c *Cache) Lifetime() time.Duration {
	return c.lifetime
}
