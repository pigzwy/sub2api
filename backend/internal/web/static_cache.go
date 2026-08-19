//go:build embed || unit

package web

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"path"
	"strings"
)

// Vite emits content-hashed filenames under assets/, so the backend can apply
// immutable caching without relying on a reverse proxy to classify paths.
const staticAssetsCacheControl = "public, max-age=31536000, immutable"

// The HTML shell contains only the immutable asset references. Runtime
// settings are loaded by the frontend, so this short TTL is safe for edge
// caching while allowing a new build to propagate quickly.
const staticShellCacheControl = "public, max-age=60, must-revalidate"

// StaticHTMLETag returns a stable validator for an embedded HTML shell.
// Unlike the settings-injected cache ETag, it changes only when the built
// frontend shell changes, which makes it safe to reuse at an edge cache.
func StaticHTMLETag(html []byte) string {
	hash := sha256.Sum256(html)
	return `"` + hex.EncodeToString(hash[:8]) + `"`
}

// isFingerprintedEmbeddedAssetPath reports whether a cleaned URL path refers to
// a Vite asset whose filename contains the default eight-character build hash.
func isFingerprintedEmbeddedAssetPath(cleanPath string) bool {
	cleanPath = strings.TrimPrefix(cleanPath, "/")
	if !strings.HasPrefix(cleanPath, "assets/") {
		return false
	}

	filename := path.Base(cleanPath)
	extension := path.Ext(filename)
	stem := strings.TrimSuffix(filename, extension)
	const fingerprintLength = 8
	delimiterIndex := len(stem) - fingerprintLength - 1
	if extension == "" || delimiterIndex < 1 || stem[delimiterIndex] != '-' {
		return false
	}

	// Vite hashes use URL-safe characters and are stable for immutable caching.
	fingerprint := stem[delimiterIndex+1:]
	for _, char := range fingerprint {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
}

// applyStaticAssetCacheHeaders sets Cache-Control for long-cacheable static paths.
// index.html / SPA routes must keep no-cache and are not handled here.
func applyStaticAssetCacheHeaders(header http.Header, cleanPath string) {
	if header == nil || !isFingerprintedEmbeddedAssetPath(cleanPath) {
		return
	}
	header.Set("Cache-Control", staticAssetsCacheControl)
}

func isFrontendNavigationMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead
}
