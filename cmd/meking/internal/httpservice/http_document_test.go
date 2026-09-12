package httpservice

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"

	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/document"
	"github.com/memoria-space/meking/corpus/text"
)

func TestHTTPDocumentUploadReturnsCommittedReceipt(t *testing.T) {
	dependencies := defaultHTTPDependencies()
	var submitted document.UploadCommand
	dependencies.submitDocument = func(
		_ context.Context,
		input document.UploadCommand,
	) (corpus.DocumentReceipt, error) {
		submitted = input
		content, err := io.ReadAll(input.Content)
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "first\nsecond" {
			t.Fatalf("submitted content = %q", content)
		}
		return corpus.DocumentReceipt{
			DocumentID: document.ID(
				"doc_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			),
			ContentDigest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		}, nil
	}
	handler := mustHTTPHandler(t, httpTestConfig(t), dependencies)
	request := newDocumentUploadRequest(t, "source.txt", "text/plain; charset=utf-8", "first\nsecond")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	var receipt httpDocumentReceipt
	decodeHTTPTestResponse(t, response, &receipt)
	if response.Code != http.StatusCreated || receipt.ZoneID != string(httpTestZoneID) ||
		receipt.DocumentID == "" || receipt.ContentDigest == "" ||
		receipt.Status != documentUploadedStatus {
		t.Fatalf("status/receipt = %d/%#v", response.Code, receipt)
	}
	if submitted.Name != "source.txt" || submitted.MediaType != "text/plain" {
		t.Fatalf("Document upload = %#v", submitted)
	}
}

func TestHTTPDocumentCatalogListsProjectFilesForCurrentZone(t *testing.T) {
	dependencies := defaultHTTPDependencies()
	dependencies.browseDocuments = func(
		_ context.Context,
		page document.Page,
	) (corpus.DocumentCatalogPage, error) {
		if page.Offset != 10 || page.Limit != 20 {
			t.Fatalf("Document page = %#v", page)
		}
		value, err := document.Restore(document.Document{
			ID:   "doc_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Name: "source.md", MediaType: "text/markdown", Size: 42,
			Digest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		})
		if err != nil {
			t.Fatal(err)
		}
		return corpus.DocumentCatalogPage{
			Documents: []corpus.DocumentCatalogEntry{{Document: value, Selected: true}},
			Next:      &document.Page{Offset: 30, Limit: 20},
		}, nil
	}
	handler := mustHTTPHandler(t, httpTestConfig(t), dependencies)
	response := serveHTTPRequest(
		t, handler, http.MethodGet,
		"/api/v1/zones/10000000-0000-4000-8000-000000000001/documents?offset=10&limit=20", nil,
	)
	var page httpDocumentCatalogPage
	decodeHTTPTestResponse(t, response, &page)
	if response.Code != http.StatusOK || page.ZoneID != string(httpTestZoneID) ||
		page.NextOffset == nil || *page.NextOffset != 30 || !page.HasMore ||
		len(page.Documents) != 1 || !page.Documents[0].Selected || page.Documents[0].Name != "source.md" {
		t.Fatalf("status/Document page = %d/%#v", response.Code, page)
	}
}

func TestHTTPDocumentUploadRejectsUnprocessableMediaType(t *testing.T) {
	dependencies := defaultHTTPDependencies()
	called := false
	dependencies.submitDocument = func(
		context.Context,
		document.UploadCommand,
	) (corpus.DocumentReceipt, error) {
		called = true
		return corpus.DocumentReceipt{}, text.ErrUnsupported
	}
	handler := mustHTTPHandler(t, httpTestConfig(t), dependencies)
	request := newDocumentUploadRequest(t, "source.pdf", "application/pdf", "%PDF")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	var failure httpErrorEnvelope
	decodeHTTPTestResponse(t, response, &failure)
	if response.Code != http.StatusUnsupportedMediaType ||
		failure.Error.Code != "unsupported_document" || !called {
		t.Fatalf("status/failure/called = %d/%#v/%t", response.Code, failure, called)
	}
}

func newDocumentUploadRequest(
	t *testing.T,
	name string,
	mediaType string,
	content string,
) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="`+name+`"`)
	header.Set("Content-Type", mediaType)
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/zones/10000000-0000-4000-8000-000000000001/documents", &body)
	request.Host = "example.com"
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}
