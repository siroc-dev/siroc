package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEncodeJSONNilSliceIsEmptyArray(t *testing.T) {
	var list []string
	b, err := json.Marshal(encodeJSON(list))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "[]" {
		t.Fatalf("got %s", b)
	}
}

func TestWriteJSONNilSlice(t *testing.T) {
	var list []string
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, list)
	if rec.Body.String() != "[]\n" && rec.Body.String() != "[]" {
		got := bytes.TrimSpace(rec.Body.Bytes())
		if string(got) != "[]" {
			t.Fatalf("body=%q", rec.Body.String())
		}
	}
}
