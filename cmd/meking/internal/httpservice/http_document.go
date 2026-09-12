package httpservice

import (
	"errors"
	"fmt"
	"mime"
	"net/http"

	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/document"
	"github.com/memoria-space/meking/corpus/text"
)

const (
	documentMultipartMemory   = 1 << 20
	documentMultipartOverhead = 1 << 20
	documentUploadedStatus    = "uploaded"
)

func (a *httpApplication) documents(writer http.ResponseWriter, request *http.Request) {
	zoneID, err := requestZoneID(request)
	if err != nil {
		writeHTTPErrorResponse(writer, err, false)
		return
	}
	offset, err := nonNegativeHTTPQueryInteger(request, "offset", 0)
	if err != nil {
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{Code: "invalid_input", Message: err.Error()})
		return
	}
	limit, err := positiveHTTPQueryInteger(request, "limit", corpus.DefaultDocumentCatalogPageSize)
	if err != nil || limit > corpus.MaximumDocumentCatalogPageSize {
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{
			Code:    "invalid_input",
			Message: fmt.Sprintf("limit must be a positive integer no greater than %d", corpus.MaximumDocumentCatalogPageSize),
		})
		return
	}
	page, err := a.dependencies.browseDocuments(
		request.Context(), document.Page{Offset: offset, Limit: limit},
	)
	if err != nil {
		writeHTTPErrorResponse(writer, err, false)
		return
	}
	documents := make([]httpDocumentCatalogEntry, len(page.Documents))
	for index, entry := range page.Documents {
		value := entry.Document
		documents[index] = httpDocumentCatalogEntry{
			DocumentID: string(value.ID), Name: value.Name, MediaType: value.MediaType,
			Size: value.Size, ContentDigest: value.Digest, Selected: entry.Selected,
		}
	}
	result := httpDocumentCatalogPage{
		ZoneID: zoneID, Offset: offset, HasMore: page.Next != nil, Documents: documents,
	}
	if page.Next != nil {
		next := page.Next.Offset
		result.NextOffset = &next
	}
	writeHTTPJSON(writer, http.StatusOK, result)
}

func (a *httpApplication) submitDocument(writer http.ResponseWriter, request *http.Request) {
	zoneID, err := requestZoneID(request)
	if err != nil {
		writeHTTPErrorResponse(writer, err, false)
		return
	}
	request.Body = http.MaxBytesReader(
		writer,
		request.Body,
		document.MaximumContentBytes+documentMultipartOverhead,
	)
	if err := request.ParseMultipartForm(documentMultipartMemory); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeHTTPErrorValue(writer, http.StatusRequestEntityTooLarge, httpError{
				Code: "document_too_large", Message: "document upload exceeds the request size limit",
			})
			return
		}
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{
			Code: "invalid_input", Message: "request must be multipart/form-data with one file part",
		})
		return
	}
	defer request.MultipartForm.RemoveAll()
	files, present := request.MultipartForm.File["file"]
	if !present || len(files) != 1 || len(request.MultipartForm.File) != 1 ||
		len(request.MultipartForm.Value) != 0 {
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{
			Code: "invalid_input", Message: "request must contain exactly one file part and no form fields",
		})
		return
	}
	fileHeader := files[0]
	if fileHeader.Size > document.MaximumContentBytes {
		writeHTTPErrorValue(writer, http.StatusRequestEntityTooLarge, httpError{
			Code: "document_too_large", Message: "document exceeds the content size limit",
		})
		return
	}
	mediaType, _, err := mime.ParseMediaType(fileHeader.Header.Get("Content-Type"))
	if err != nil {
		writeHTTPErrorValue(writer, http.StatusUnsupportedMediaType, httpError{
			Code: "unsupported_document", Message: "the document media type is invalid",
		})
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		writeHTTPErrorResponse(writer, fmt.Errorf("open uploaded Document: %w", err), false)
		return
	}
	defer file.Close()

	receipt, err := a.dependencies.submitDocument(request.Context(), document.UploadCommand{
		Name: fileHeader.Filename, MediaType: mediaType, Content: file,
	})
	if err != nil {
		writeDocumentUploadError(writer, err)
		return
	}
	writeHTTPJSON(writer, http.StatusCreated, httpDocumentReceipt{
		ZoneID:        zoneID,
		DocumentID:    string(receipt.DocumentID),
		ContentDigest: receipt.ContentDigest,
		Status:        documentUploadedStatus,
	})
}

func writeDocumentUploadError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, document.ErrContentTooLarge):
		writeHTTPErrorValue(writer, http.StatusRequestEntityTooLarge, httpError{
			Code: "document_too_large", Message: "document exceeds the content size limit",
		})
	case errors.Is(err, text.ErrUnsupported), errors.Is(err, text.ErrInvalid):
		writeHTTPErrorValue(writer, http.StatusUnsupportedMediaType, httpError{
			Code: "unsupported_document", Message: "the document media type cannot be processed",
		})
	case errors.Is(err, document.ErrInvalid):
		writeHTTPErrorValue(writer, http.StatusBadRequest, httpError{
			Code: "invalid_document", Message: err.Error(),
		})
	case errors.Is(err, document.ErrContentConflict):
		writeHTTPErrorValue(writer, http.StatusConflict, httpError{
			Code: "document_conflict", Message: "the document conflicts with stored content",
		})
	default:
		writeHTTPErrorResponse(writer, err, false)
	}
}
