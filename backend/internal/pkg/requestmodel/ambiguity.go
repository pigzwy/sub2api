package requestmodel

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"strings"

	"github.com/tidwall/gjson"
)

// AmbiguousModelMessage deliberately omits client-controlled model names.
const AmbiguousModelMessage = "duplicate model or session fields are not allowed"

// HasAmbiguousJSON rejects repeated routing fields before a decoder can collapse
// them. Count occurrences, including null/non-string values and identical values;
// gjson decodes escaped key names, and EqualFold covers struct-binding aliases.
// Only routing envelopes are inspected, not tool schemas or user content. Invalid
// JSON remains the caller's normal syntax-validation responsibility.
func HasAmbiguousJSON(body []byte) bool {
	if !gjson.ValidBytes(body) {
		return false
	}
	return ambiguousObject(gjson.ParseBytes(body), true)
}

func ambiguousObject(object gjson.Result, envelopes bool) bool {
	if !object.IsObject() {
		return false
	}
	models, sessions, responses := 0, 0, 0
	ambiguous := false
	object.ForEach(func(key, value gjson.Result) bool {
		switch {
		case strings.EqualFold(key.String(), "model"):
			models++
		case envelopes && strings.EqualFold(key.String(), "session"):
			sessions++
			ambiguous = ambiguous || ambiguousObject(value, false)
		case envelopes && strings.EqualFold(key.String(), "response"):
			responses++
			ambiguous = ambiguous || ambiguousObject(value, false)
		}
		ambiguous = ambiguous || models > 1 || sessions > 1 || responses > 1
		return !ambiguous
	})
	return ambiguous
}

// HasAmbiguousBody also covers multipart model/session fields used by image,
// audio and Live endpoints. It never rewrites the original request bytes.
func HasAmbiguousBody(contentType string, body []byte) bool {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.EqualFold(mediaType, "multipart/form-data") {
		return HasAmbiguousJSON(body)
	}
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	models, sessions := 0, 0
	for {
		part, err := reader.NextPart()
		if err != nil {
			return false // malformed multipart is rejected by its owning handler
		}
		name := part.FormName()
		if strings.EqualFold(name, "model") {
			models++
		}
		if strings.EqualFold(name, "session") {
			sessions++
			data, readErr := io.ReadAll(part)
			if readErr == nil && HasAmbiguousJSON(data) {
				return true
			}
		}
		if models > 1 || sessions > 1 {
			return true
		}
	}
}
