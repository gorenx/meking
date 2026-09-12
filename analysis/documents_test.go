package analysis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientConvertDocumentStreamsVersionedMultipart(t *testing.T) {
	t.Parallel()
	content := []byte("<html><title>Atlas</title><body>Beacon</body></html>")
	digest := sha256.Sum256(content)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/documents/convert" {
			http.NotFound(writer, request)
			return
		}
		if request.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatal("missing analysis authorization")
		}
		if err := request.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm() error = %v", err)
		}
		if request.FormValue("contract_version") != "2" || request.FormValue("extension") != ".html" ||
			request.FormValue("media_type") != "text/html" ||
			request.FormValue("content_sha256") != hex.EncodeToString(digest[:]) {
			t.Fatalf("multipart fields = %#v", request.MultipartForm.Value)
		}
		file, header, err := request.FormFile("file")
		if err != nil {
			t.Fatalf("FormFile() error = %v", err)
		}
		defer file.Close()
		actual, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		if header.Filename != "atlas.html" || header.Header.Get("Content-Type") != "text/html" || string(actual) != string(content) {
			t.Fatalf("uploaded file = name=%q type=%q content=%q", header.Filename, header.Header.Get("Content-Type"), actual)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"contract_version":2,"markdown":"# Atlas","title":"Atlas","warnings":[]}`))
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		BaseURL: server.URL, BearerToken: "test-token", HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	response, err := client.ConvertDocument(t.Context(), DocumentConversionRequest{
		Filename: "atlas.html", Extension: ".html", MediaType: "text/html",
		ContentSHA256: hex.EncodeToString(digest[:]), ContentLength: int64(len(content)),
		Content: strings.NewReader(string(content)),
	})
	if err != nil {
		t.Fatalf("ConvertDocument() error = %v", err)
	}
	if response.Markdown != "# Atlas" || response.Title == nil || *response.Title != "Atlas" || response.Warnings == nil {
		t.Fatalf("ConvertDocument() = %#v", response)
	}
}

func TestSupportsDocumentConversionUsesFilenameAndCanonicalMediaType(t *testing.T) {
	t.Parallel()

	for _, supported := range []struct{ name, mediaType string }{
		{name: "source.pdf", mediaType: "application/pdf"},
		{name: "source.DOCX", mediaType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
		{name: "source.html", mediaType: "text/html"},
	} {
		if !SupportsDocumentConversion(supported.name, supported.mediaType) {
			t.Errorf("SupportsDocumentConversion(%q, %q) = false", supported.name, supported.mediaType)
		}
	}
	for _, unsupported := range []struct{ name, mediaType string }{
		{name: "source.pdf", mediaType: "application/octet-stream"},
		{name: "source.zip", mediaType: "application/zip"},
		{name: "../source.pdf", mediaType: "application/pdf"},
		{name: "source.html", mediaType: "text/html; charset=utf-8"},
	} {
		if SupportsDocumentConversion(unsupported.name, unsupported.mediaType) {
			t.Errorf("SupportsDocumentConversion(%q, %q) = true", unsupported.name, unsupported.mediaType)
		}
	}
	if !IsDocumentConversionCandidate("source.txt", "text/html") ||
		!IsDocumentConversionCandidate("source.pdf", "application/octet-stream") ||
		IsDocumentConversionCandidate("source.txt", "text/plain") {
		t.Fatal("Document conversion candidate classification is invalid")
	}
}

func TestClientConvertDocumentValidatesRequestAndResponse(t *testing.T) {
	t.Parallel()
	valid := DocumentConversionRequest{
		Filename: "atlas.pdf", Extension: ".pdf", MediaType: "application/pdf",
		ContentSHA256: strings.Repeat("a", 64), ContentLength: 1, Content: strings.NewReader("x"),
	}
	tests := []struct {
		name    string
		request DocumentConversionRequest
	}{
		{name: "path filename", request: withDocumentFilename(valid, "../atlas.pdf")},
		{name: "unsupported extension", request: withDocumentExtension(valid, ".zip")},
		{name: "mismatched media", request: withDocumentMediaType(valid, "application/zip")},
		{name: "invalid digest", request: withDocumentDigest(valid, "ABC")},
		{name: "empty content", request: withDocumentLength(valid, 0)},
		{name: "oversized content", request: withDocumentLength(valid, MaxDocumentContentBytes+1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &Client{}
			_, err := client.ConvertDocument(t.Context(), test.request)
			var failure *Failure
			if !errors.As(err, &failure) || failure.Operation != "document_conversion_request" {
				t.Fatalf("ConvertDocument() error = %v", err)
			}
		})
	}

	badTitle := " title "
	for _, response := range []DocumentConversionResponse{
		{ContractVersion: 1, Warnings: []string{}},
		{ContractVersion: 2, Title: &badTitle, Warnings: []string{}},
		{ContractVersion: 2, Warnings: []string{"line\nsecret"}},
	} {
		if err := response.Validate(); err == nil {
			t.Fatalf("Validate(%#v) expected error", response)
		}
	}
}

func TestClientConvertDocumentPropagatesCancellationAndLengthDrift(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body)
		_, _ = writer.Write([]byte(`{"contract_version":2,"markdown":"ok","title":null,"warnings":[]}`))
	}))
	defer server.Close()
	client, err := NewClient(ClientConfig{
		BaseURL: server.URL, BearerToken: "token", HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	request := DocumentConversionRequest{
		Filename: "atlas.html", Extension: ".html", MediaType: "text/html",
		ContentSHA256: strings.Repeat("a", 64), ContentLength: 2, Content: strings.NewReader("one"),
	}
	_, err = client.ConvertDocument(t.Context(), request)
	var failure *Failure
	if !errors.As(err, &failure) || failure.Kind != FailureUnavailable {
		t.Fatalf("ConvertDocument(length drift) error = %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	request.ContentLength = 3
	request.Content = strings.NewReader("one")
	_, err = client.ConvertDocument(ctx, request)
	if !errors.As(err, &failure) || !errors.Is(err, context.Canceled) {
		t.Fatalf("ConvertDocument(cancelled) error = %v", err)
	}
}

func withDocumentFilename(request DocumentConversionRequest, value string) DocumentConversionRequest {
	request.Filename = value
	return request
}

func withDocumentExtension(request DocumentConversionRequest, value string) DocumentConversionRequest {
	request.Extension = value
	return request
}

func withDocumentMediaType(request DocumentConversionRequest, value string) DocumentConversionRequest {
	request.MediaType = value
	return request
}

func withDocumentDigest(request DocumentConversionRequest, value string) DocumentConversionRequest {
	request.ContentSHA256 = value
	return request
}

func withDocumentLength(request DocumentConversionRequest, value int64) DocumentConversionRequest {
	request.ContentLength = value
	return request
}
