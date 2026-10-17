package requestmodel

import (
	"bytes"
	"mime/multipart"
	"testing"
)

func TestHasAmbiguousJSON(t *testing.T) {
	for _, body := range []string{
		`{"model":"astra","model":"sol"}`,
		`{"model":"sol","model":"astra","messages":[]}`,
		`{"model":"sol","model":"sol"}`,
		`{"model":"sol","Model":"astra"}`,
		`{"MODEL":null,"mOdEl":"astra"}`,
		`{"\u006dodel":"sol","mod\u0065l":"astra"}`,
		`{"model":{},"model":false}`,
		`{"session":{"model":"sol","Model":"astra"}}`,
		`{"session":{},"SESSION":{"model":"astra"}}`,
		`{"response":{"model":"sol","model":"astra"}}`,
		`{"response":{},"Response":{}}`,
	} {
		t.Run(body, func(t *testing.T) {
			if !HasAmbiguousJSON([]byte(body)) {
				t.Fatal("ambiguous routing fields accepted")
			}
		})
	}
	for _, body := range []string{
		`{"model":"sol","input":"hello"}`,
		`{"\u006dodel":"sol"}`,
		`{"model":"sol","session":{"model":"astra"}}`,
		`{"type":"session.update","session":{"model":"sol"}}`,
		`{"type":"response.create"}`,
		`{"model":"sol","tools":[{"parameters":{"properties":{"model":{},"Model":{}}}}]}`,
		`{"model":"sol","input":"\"model\":\"astra\",\"model\":null"}`,
		`{"model":`, `[]`, `null`, ``,
	} {
		t.Run(body, func(t *testing.T) {
			if HasAmbiguousJSON([]byte(body)) {
				t.Fatal("unambiguous request rejected")
			}
		})
	}
}

func TestHasAmbiguousMultipart(t *testing.T) {
	for _, tc := range []struct {
		fields [][2]string
		want   bool
	}{
		{[][2]string{{"model", "sol"}, {"model", "astra"}}, true},
		{[][2]string{{"model", "sol"}, {"MODEL", "sol"}}, true},
		{[][2]string{{"session", `{"model":"sol","\u006dodel":"astra"}`}}, true},
		{[][2]string{{"session", `{}`}, {"Session", `{}`}}, true},
		{[][2]string{{"model", "sol"}, {"prompt", "model model"}}, false},
		{[][2]string{{"session", `{"model":"sol"}`}}, false},
	} {
		var body bytes.Buffer
		w := multipart.NewWriter(&body)
		for _, field := range tc.fields {
			if err := w.WriteField(field[0], field[1]); err != nil {
				t.Fatal(err)
			}
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		if got := HasAmbiguousBody(w.FormDataContentType(), body.Bytes()); got != tc.want {
			t.Fatalf("fields=%v got=%v", tc.fields, got)
		}
	}
}
