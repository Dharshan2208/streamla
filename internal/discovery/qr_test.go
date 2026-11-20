package discovery

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestQRHandler(t *testing.T) {
	handler := QRHandler("6969")
	req := httptest.NewRequest(http.MethodGet, "/api/qr", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "image/png" {
		t.Fatalf("expected image/png content type, got %s", ct)
	}

	body := rec.Body.Bytes()
	// PNG signature is 89 50 4E 47 0D 0A 1A 0A
	pngSig := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	if !bytes.HasPrefix(body, pngSig) {
		t.Fatalf("expected valid PNG signature, got %x", body[:8])
	}
}
