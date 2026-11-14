package ocicopy

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
	"net/url"
	"strconv"
	"strings"

	"github.com/septi0/dockvmap/internal/oci"
)

const maxManifestSize = 4 << 20

const pageSize = 1000

var ErrDeletesDisabled = errors.New("the registry doesn't allow deletes; set REGISTRY_STORAGE_DELETE_ENABLED=true on it")

type Registry struct {
	BaseURL string
	Client  *http.Client
	Header  http.Header
}

type statusError struct {
	Status int
	Op     string
}

func (e *statusError) Error() string {
	return fmt.Sprintf("%s: registry answered %d %s", e.Op, e.Status, http.StatusText(e.Status))
}

func (r Registry) do(ctx context.Context, method, path string, body io.Reader, header http.Header) (*http.Response, error) {
	req, err := r.newRequest(ctx, method, strings.TrimSuffix(r.BaseURL, "/")+path, body, header)

	if err != nil {
		return nil, err
	}

	return r.Client.Do(req)
}

func (r Registry) newRequest(ctx context.Context, method, target string, body io.Reader, header http.Header) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, target, body)

	if err != nil {
		return nil, err
	}

	for key, values := range r.Header {
		req.Header[key] = values
	}

	for key, values := range header {
		req.Header[key] = values
	}

	return req, nil
}

func (r Registry) Repositories(ctx context.Context) ([]string, error) {
	var page struct {
		Repositories []string `json:"repositories"`
	}

	return list(ctx, r, "/v2/_catalog", "listing repositories", &page, func() []string { return page.Repositories })
}

func (r Registry) Tags(ctx context.Context, repository string) ([]string, error) {
	var page struct {
		Tags []string `json:"tags"`
	}

	tags, err := list(ctx, r, "/v2/"+repository+"/tags/list", "listing tags of "+repository, &page, func() []string { return page.Tags })

	var status *statusError

	if errors.As(err, &status) && status.Status == http.StatusNotFound {
		return nil, nil
	}

	return tags, err
}

// next pages differ from the first only in their query, so the base path (which may carry a prefix) is kept
func list(ctx context.Context, r Registry, path, op string, page any, items func() []string) ([]string, error) {
	var all []string
	query := "n=" + strconv.Itoa(pageSize)

	for query != "" {
		resp, err := r.do(ctx, http.MethodGet, path+"?"+query, nil, nil)

		if err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}

		if resp.StatusCode != http.StatusOK {
			oci.DrainAndClose(resp)
			return nil, &statusError{Status: resp.StatusCode, Op: op}
		}

		err = json.NewDecoder(io.LimitReader(resp.Body, maxManifestSize)).Decode(page)
		query = nextQuery(resp)
		oci.DrainAndClose(resp)

		if err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}

		all = append(all, items()...)
	}

	return all, nil
}

func nextQuery(resp *http.Response) string {
	next, err := url.Parse(oci.NextPageURL(resp.Request.URL, resp.Header.Get("Link")))

	if err != nil {
		return ""
	}

	return next.RawQuery
}

// ManifestDigest returns "" when the reference doesn't exist
func (r Registry) ManifestDigest(ctx context.Context, repository, reference string) (string, error) {
	resp, err := r.do(ctx, http.MethodHead, "/v2/"+repository+"/manifests/"+reference, nil, acceptManifests())

	if err != nil {
		return "", fmt.Errorf("checking %s:%s: %w", repository, reference, err)
	}

	oci.DrainAndClose(resp)

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return "", nil
	case resp.StatusCode != http.StatusOK:
		return "", &statusError{Status: resp.StatusCode, Op: "checking " + repository + ":" + reference}
	}

	return resp.Header.Get("Docker-Content-Digest"), nil
}

func acceptManifests() http.Header {
	return http.Header{"Accept": oci.ManifestMediaTypes}
}

type manifest struct {
	body      []byte
	mediaType string
	digest    string
}

type manifestContent struct {
	MediaType string       `json:"mediaType"`
	Config    *descriptor  `json:"config"`
	Layers    []descriptor `json:"layers"`
	Manifests []descriptor `json:"manifests"`
}

type descriptor struct {
	Digest string `json:"digest"`
}

func (r Registry) fetchManifest(ctx context.Context, repository, reference string) (*manifest, error) {
	op := "reading " + repository + ":" + reference
	resp, err := r.do(ctx, http.MethodGet, "/v2/"+repository+"/manifests/"+reference, nil, acceptManifests())

	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	defer oci.DrainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, &statusError{Status: resp.StatusCode, Op: op}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxManifestSize+1))

	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	if len(body) > maxManifestSize {
		return nil, fmt.Errorf("%s: the manifest is larger than 4 MiB", op)
	}

	sum := sha256.Sum256(body)
	digest := "sha256:" + hex.EncodeToString(sum[:])

	expected := resp.Header.Get("Docker-Content-Digest")

	if oci.IsDigest(reference) {
		expected = reference
	}

	if expected != "" && expected != digest {
		return nil, fmt.Errorf("%s: the manifest doesn't match its digest %s", op, expected)
	}

	mediaType := resp.Header.Get("Content-Type")

	if mediaType == "" {
		var content manifestContent
		json.Unmarshal(body, &content)
		mediaType = content.MediaType
	}

	return &manifest{body: body, mediaType: mediaType, digest: digest}, nil
}

// Copy returns false when dst already had the tag at the same digest, or src no longer has it
func Copy(ctx context.Context, src, dst Registry, repository, tag string) (bool, error) {
	wanted, err := src.ManifestDigest(ctx, repository, tag)

	if err != nil || wanted == "" {
		return false, err
	}

	current, err := dst.ManifestDigest(ctx, repository, tag)

	if err != nil {
		return false, err
	}

	if current == wanted {
		return false, nil
	}

	m, err := src.fetchManifest(ctx, repository, tag)

	if err != nil {
		return false, err
	}

	if current == m.digest {
		return false, nil
	}

	if err := copyContent(ctx, src, dst, repository, m); err != nil {
		return false, err
	}

	return true, dst.putManifest(ctx, repository, tag, m)
}

func copyContent(ctx context.Context, src, dst Registry, repository string, m *manifest) error {
	var content manifestContent

	if err := json.Unmarshal(m.body, &content); err != nil {
		return fmt.Errorf("reading manifest %s of %s: %w", m.digest, repository, err)
	}

	for _, child := range content.Manifests {
		if existing, err := dst.ManifestDigest(ctx, repository, child.Digest); err != nil {
			return err
		} else if existing != "" {
			continue
		}

		childManifest, err := src.fetchManifest(ctx, repository, child.Digest)

		if err != nil {
			return err
		}

		if err := copyContent(ctx, src, dst, repository, childManifest); err != nil {
			return err
		}

		if err := dst.putManifest(ctx, repository, child.Digest, childManifest); err != nil {
			return err
		}
	}

	blobs := content.Layers

	if content.Config != nil {
		blobs = append([]descriptor{*content.Config}, blobs...)
	}

	for _, blob := range blobs {
		if err := copyBlob(ctx, src, dst, repository, blob); err != nil {
			return err
		}
	}

	return nil
}

func (r Registry) blobExists(ctx context.Context, repository, digest string) (bool, error) {
	resp, err := r.do(ctx, http.MethodHead, "/v2/"+repository+"/blobs/"+digest, nil, nil)

	if err != nil {
		return false, fmt.Errorf("checking blob %s: %w", digest, err)
	}

	oci.DrainAndClose(resp)

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	}

	return false, &statusError{Status: resp.StatusCode, Op: "checking blob " + digest}
}

// the destination registry checks the digest when the upload completes, as the OCI spec requires
func copyBlob(ctx context.Context, src, dst Registry, repository string, blob descriptor) error {
	if exists, err := dst.blobExists(ctx, repository, blob.Digest); err != nil || exists {
		return err
	}

	op := "copying blob " + blob.Digest + " of " + repository
	resp, err := src.do(ctx, http.MethodGet, "/v2/"+repository+"/blobs/"+blob.Digest, nil, nil)

	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	defer oci.DrainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		return &statusError{Status: resp.StatusCode, Op: op}
	}

	location, err := dst.startUpload(ctx, repository)

	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	upload, err := url.Parse(location)

	if err != nil {
		return fmt.Errorf("%s: invalid upload location: %w", op, err)
	}

	query := upload.Query()
	query.Set("digest", blob.Digest)
	upload.RawQuery = query.Encode()

	req, err := dst.newRequest(ctx, http.MethodPut, dst.resolve(upload).String(), resp.Body, http.Header{"Content-Type": {"application/octet-stream"}})

	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	req.ContentLength = resp.ContentLength
	put, err := dst.Client.Do(req)

	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	oci.DrainAndClose(put)

	if put.StatusCode != http.StatusCreated {
		return &statusError{Status: put.StatusCode, Op: op}
	}

	return nil
}

func (r Registry) startUpload(ctx context.Context, repository string) (string, error) {
	resp, err := r.do(ctx, http.MethodPost, "/v2/"+repository+"/blobs/uploads/", nil, nil)

	if err != nil {
		return "", err
	}

	oci.DrainAndClose(resp)

	if resp.StatusCode != http.StatusAccepted {
		return "", &statusError{Status: resp.StatusCode, Op: "starting an upload"}
	}

	location := resp.Header.Get("Location")

	if location == "" {
		return "", errors.New("the registry started an upload without a location")
	}

	return location, nil
}

func (r Registry) resolve(location *url.URL) *url.URL {
	if location.IsAbs() {
		return location
	}

	base, err := url.Parse(r.BaseURL)

	if err != nil {
		return location
	}

	return base.ResolveReference(location)
}

func (r Registry) putManifest(ctx context.Context, repository, reference string, m *manifest) error {
	resp, err := r.do(ctx, http.MethodPut, "/v2/"+repository+"/manifests/"+reference, bytes.NewReader(m.body), http.Header{"Content-Type": {m.mediaType}})

	if err != nil {
		return fmt.Errorf("pushing %s:%s: %w", repository, reference, err)
	}

	oci.DrainAndClose(resp)

	if resp.StatusCode != http.StatusCreated {
		return &statusError{Status: resp.StatusCode, Op: "pushing " + repository + ":" + reference}
	}

	return nil
}

// Delete removes a tag or manifest the way the primary did; one already gone counts as deleted
func (r Registry) Delete(ctx context.Context, repository, reference string) error {
	resp, err := r.do(ctx, http.MethodDelete, "/v2/"+repository+"/manifests/"+reference, nil, nil)

	if err != nil {
		return fmt.Errorf("deleting %s:%s: %w", repository, reference, err)
	}

	oci.DrainAndClose(resp)

	switch resp.StatusCode {
	case http.StatusAccepted, http.StatusNotFound:
		return nil
	case http.StatusMethodNotAllowed:
		return ErrDeletesDisabled
	}

	return &statusError{Status: resp.StatusCode, Op: "deleting " + repository + ":" + reference}
}

// DeletesAllowed deletes a manifest that can't exist: a registry with deletes off refuses before looking it up
func (r Registry) DeletesAllowed(ctx context.Context) (bool, error) {
	err := r.Delete(ctx, "dockvmap-probe", "sha256:"+strings.Repeat("0", 64))

	if errors.Is(err, ErrDeletesDisabled) {
		return false, nil
	}

	return err == nil, err
}
