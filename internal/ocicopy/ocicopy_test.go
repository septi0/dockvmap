package ocicopy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
)

const (
	indexType    = "application/vnd.oci.image.index.v1+json"
	manifestType = "application/vnd.oci.image.manifest.v1+json"
)

type fakeRegistry struct {
	mu             sync.Mutex
	blobs          map[string][]byte
	manifests      map[string][]byte
	mediaTypes     map[string]string
	tags           map[string]map[string]string
	uploads        int
	deletesOff     bool
	corruptDigests bool
}

func newFakeRegistry() *fakeRegistry {
	return &fakeRegistry{
		blobs:      map[string][]byte{},
		manifests:  map[string][]byte{},
		mediaTypes: map[string]string{},
		tags:       map[string]map[string]string{},
	}
}

func digestOf(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func (f *fakeRegistry) addBlob(data string) string {
	digest := digestOf([]byte(data))
	f.blobs[digest] = []byte(data)

	return digest
}

func (f *fakeRegistry) addManifest(repository, tag, mediaType string, content any) string {
	body, _ := json.Marshal(content)
	digest := digestOf(body)
	f.manifests[digest] = body
	f.mediaTypes[digest] = mediaType

	if tag != "" {
		if f.tags[repository] == nil {
			f.tags[repository] = map[string]string{}
		}

		f.tags[repository][tag] = digest
	}

	return digest
}

func (f *fakeRegistry) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	path := strings.TrimPrefix(r.URL.Path, "/prefix")

	switch {
	case strings.HasSuffix(path, "/tags/list"):
		repository := strings.TrimSuffix(strings.TrimPrefix(path, "/v2/"), "/tags/list")
		tags := slices.Sorted(maps.Keys(f.tags[repository]))

		if tags == nil {
			http.NotFound(w, r)
			return
		}

		start := 0

		if last := r.URL.Query().Get("last"); last != "" {
			start = slices.Index(tags, last) + 1
		}

		end := min(start+2, len(tags))

		if end < len(tags) {
			w.Header().Set("Link", fmt.Sprintf(`</v2/%s/tags/list?last=%s&n=2>; rel="next"`, repository, tags[end-1]))
		}

		json.NewEncoder(w).Encode(map[string]any{"name": repository, "tags": tags[start:end]})

	case strings.Contains(path, "/manifests/"):
		repository, reference, _ := strings.Cut(strings.TrimPrefix(path, "/v2/"), "/manifests/")
		digest := reference

		if !strings.HasPrefix(reference, "sha256:") {
			digest = f.tags[repository][reference]
		}

		switch r.Method {
		case http.MethodGet, http.MethodHead:
			body, ok := f.manifests[digest]

			if !ok {
				http.NotFound(w, r)
				return
			}

			w.Header().Set("Content-Type", f.mediaTypes[digest])
			w.Header().Set("Docker-Content-Digest", digest)

			if f.corruptDigests {
				body = append(slices.Clone(body), ' ')
			}

			w.Write(body)

		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			stored := digestOf(body)

			if strings.HasPrefix(reference, "sha256:") && reference != stored {
				http.Error(w, "digest invalid", http.StatusBadRequest)
				return
			}

			f.manifests[stored] = body
			f.mediaTypes[stored] = r.Header.Get("Content-Type")

			if !strings.HasPrefix(reference, "sha256:") {
				if f.tags[repository] == nil {
					f.tags[repository] = map[string]string{}
				}

				f.tags[repository][reference] = stored
			}

			w.WriteHeader(http.StatusCreated)

		case http.MethodDelete:
			if f.deletesOff {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}

			if _, ok := f.tags[repository][reference]; ok {
				delete(f.tags[repository], reference)
				w.WriteHeader(http.StatusAccepted)
				return
			}

			http.NotFound(w, r)
		}

	case strings.HasSuffix(path, "/blobs/uploads/") && r.Method == http.MethodPost:
		w.Header().Set("Location", "/prefix"+path+"upload-1")
		w.WriteHeader(http.StatusAccepted)

	case strings.Contains(path, "/blobs/uploads/") && r.Method == http.MethodPut:
		body, _ := io.ReadAll(r.Body)
		digest := r.URL.Query().Get("digest")

		if digestOf(body) != digest {
			http.Error(w, "digest invalid", http.StatusBadRequest)
			return
		}

		f.blobs[digest] = body
		f.uploads++
		w.WriteHeader(http.StatusCreated)

	case strings.Contains(path, "/blobs/"):
		digest := path[strings.LastIndex(path, "/")+1:]
		body, ok := f.blobs[digest]

		if !ok {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Length", fmt.Sprint(len(body)))

		if r.Method == http.MethodGet {
			w.Write(body)
		}

	default:
		http.NotFound(w, r)
	}
}

func serve(t *testing.T, f *fakeRegistry) Registry {
	t.Helper()

	server := httptest.NewServer(f)
	t.Cleanup(server.Close)

	return Registry{BaseURL: server.URL + "/prefix", Client: server.Client()}
}

func image(f *fakeRegistry, base, layer, config string) map[string]any {
	return map[string]any{
		"schemaVersion": 2,
		"mediaType":     manifestType,
		"config":        map[string]any{"digest": f.addBlob(config)},
		"layers":        []map[string]any{{"digest": f.addBlob(base)}, {"digest": f.addBlob(layer)}},
	}
}

func multiArch(f *fakeRegistry, repository, tag string) string {
	amd64 := f.addManifest(repository, "", manifestType, image(f, "base", "amd64 layer", "amd64 config"))
	arm64 := f.addManifest(repository, "", manifestType, image(f, "base", "arm64 layer", "arm64 config"))

	return f.addManifest(repository, tag, indexType, map[string]any{
		"schemaVersion": 2,
		"mediaType":     indexType,
		"manifests":     []map[string]any{{"digest": amd64}, {"digest": arm64}},
	})
}

func TestCopyMultiArchImage(t *testing.T) {
	ctx := context.Background()
	source, target := newFakeRegistry(), newFakeRegistry()
	index := multiArch(source, "team/web", "v1")
	src, dst := serve(t, source), serve(t, target)

	copied, err := Copy(ctx, src, dst, "team/web", "v1")

	if err != nil || !copied {
		t.Fatalf("Copy() = %v, %v, want copied", copied, err)
	}

	if got, _ := dst.ManifestDigest(ctx, "team/web", "v1"); got != index {
		t.Errorf("target v1 = %q, want the index %q", got, index)
	}

	if target.uploads != 5 {
		t.Errorf("uploaded %d blobs, want 5 (the shared base layer once)", target.uploads)
	}

	if len(target.manifests) != 3 {
		t.Errorf("target has %d manifests, want the index and both platforms", len(target.manifests))
	}

	copied, err = Copy(ctx, src, dst, "team/web", "v1")

	if err != nil || copied || target.uploads != 5 {
		t.Errorf("second Copy() = %v, %v with %d uploads, want nothing copied", copied, err, target.uploads)
	}
}

func TestCopySkipsBlobsTheTargetHas(t *testing.T) {
	ctx := context.Background()
	source, target := newFakeRegistry(), newFakeRegistry()
	source.addManifest("team/api", "v2", manifestType, image(source, "base", "api layer", "api config"))
	target.addBlob("base")

	if _, err := Copy(ctx, serve(t, source), serve(t, target), "team/api", "v2"); err != nil {
		t.Fatalf("Copy() error = %v", err)
	}

	if target.uploads != 2 {
		t.Errorf("uploaded %d blobs, want 2 (the base layer was already there)", target.uploads)
	}
}

func TestCopyRefusesAManifestThatDoesNotMatchItsDigest(t *testing.T) {
	source, target := newFakeRegistry(), newFakeRegistry()
	multiArch(source, "team/web", "v1")
	source.corruptDigests = true

	_, err := Copy(context.Background(), serve(t, source), serve(t, target), "team/web", "v1")

	if err == nil || !strings.Contains(err.Error(), "doesn't match its digest") {
		t.Errorf("Copy() error = %v, want a digest mismatch", err)
	}

	if len(target.manifests) != 0 || target.uploads != 0 {
		t.Errorf("target got %d manifests and %d blobs, want nothing", len(target.manifests), target.uploads)
	}
}

func TestTagsFollowsPages(t *testing.T) {
	source := newFakeRegistry()

	for _, tag := range []string{"a", "b", "c", "d", "e"} {
		source.addManifest("team/web", tag, manifestType, image(source, "base", tag, tag))
	}

	tags, err := serve(t, source).Tags(context.Background(), "team/web")

	if err != nil || !slices.Equal(tags, []string{"a", "b", "c", "d", "e"}) {
		t.Errorf("Tags() = %v, %v, want a to e across three pages", tags, err)
	}

	if tags, err := serve(t, source).Tags(context.Background(), "missing"); err != nil || tags != nil {
		t.Errorf("Tags() of a missing repository = %v, %v, want none", tags, err)
	}
}

func TestDeletes(t *testing.T) {
	ctx := context.Background()
	registry := newFakeRegistry()
	registry.addManifest("team/web", "v1", manifestType, image(registry, "base", "layer", "config"))
	r := serve(t, registry)

	if allowed, err := r.DeletesAllowed(ctx); err != nil || !allowed {
		t.Errorf("DeletesAllowed() = %v, %v, want true", allowed, err)
	}

	if err := r.Delete(ctx, "team/web", "v1"); err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	if err := r.Delete(ctx, "team/web", "v1"); err != nil {
		t.Errorf("deleting an already deleted tag: error = %v, want none", err)
	}

	registry.deletesOff = true

	if allowed, err := r.DeletesAllowed(ctx); err != nil || allowed {
		t.Errorf("DeletesAllowed() with deletes off = %v, %v, want false", allowed, err)
	}

	if err := r.Delete(ctx, "team/web", "v1"); !errors.Is(err, ErrDeletesDisabled) {
		t.Errorf("Delete() with deletes off error = %v, want ErrDeletesDisabled", err)
	}
}
