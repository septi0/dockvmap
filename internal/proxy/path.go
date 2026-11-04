package proxy

import "strings"

type pathKind int

const (
	pathManifest pathKind = iota + 1
	pathBlob
	pathUploads
	pathUpload
	pathTags
	pathReferrers
	pathCatalog
)

type registryPath struct {
	name string
	kind pathKind
	ref  string
}

func parseRegistryPath(path string) (registryPath, bool) {
	rest, ok := strings.CutPrefix(path, "/v2/")

	if !ok {
		return registryPath{}, false
	}

	if rest == "_catalog" {
		return registryPath{kind: pathCatalog}, true
	}

	parts := strings.Split(rest, "/")
	n := len(parts)

	var parsed registryPath

	switch {
	case n >= 4 && parts[n-3] == "blobs" && parts[n-2] == "uploads":
		parsed = registryPath{name: strings.Join(parts[:n-3], "/"), kind: pathUpload, ref: parts[n-1]}

		if parsed.ref == "" {
			parsed.kind = pathUploads
		}

	case n >= 3 && parts[n-2] == "manifests":
		parsed = registryPath{name: strings.Join(parts[:n-2], "/"), kind: pathManifest, ref: parts[n-1]}

	case n >= 3 && parts[n-2] == "blobs":
		parsed = registryPath{name: strings.Join(parts[:n-2], "/"), kind: pathBlob, ref: parts[n-1]}

	case n >= 3 && parts[n-2] == "tags" && parts[n-1] == "list":
		parsed = registryPath{name: strings.Join(parts[:n-2], "/"), kind: pathTags}

	case n >= 3 && parts[n-2] == "referrers":
		parsed = registryPath{name: strings.Join(parts[:n-2], "/"), kind: pathReferrers, ref: parts[n-1]}

	default:
		return registryPath{}, false
	}

	if parsed.name == "" || (parsed.ref == "" && parsed.kind != pathUploads && parsed.kind != pathTags) {
		return registryPath{}, false
	}

	return parsed, true
}
