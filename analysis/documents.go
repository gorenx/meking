package analysis

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// MaxDocumentContentBytes is the transport hard limit for one local rich
	// document. Project policy may select a smaller value before calling the
	// client, but no caller can expand the Sidecar protocol beyond this bound.
	MaxDocumentContentBytes int64 = 64 << 20
	maxDocumentWarnings           = 64
	maxDocumentWarningRunes       = 1024
)

var sha256HexPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// DocumentConversionRequest carries one already validated local file as a
// stream. It deliberately contains no path that the Sidecar could open.
type DocumentConversionRequest struct {
	Filename      string
	Extension     string
	MediaType     string
	ContentSHA256 string
	ContentLength int64
	Content       io.Reader
}

// DocumentConversionResponse is transport output only. The corpus consumer
// decides title fallback, source metadata, identity, and Document ownership.
type DocumentConversionResponse struct {
	ContractVersion int      `json:"contract_version"`
	Markdown        string   `json:"markdown"`
	Title           *string  `json:"title"`
	Warnings        []string `json:"warnings"`
}

// ConvertDocument streams one bounded file through the authenticated local
// conversion endpoint and validates the complete response before returning.
func (c *Client) ConvertDocument(
	ctx context.Context,
	request DocumentConversionRequest,
) (DocumentConversionResponse, error) {
	if err := validateDocumentConversionRequest(request); err != nil {
		return DocumentConversionResponse{}, failed("document_conversion_request", false, err)
	}
	if c == nil || c.baseURL == nil || c.httpClient == nil {
		return DocumentConversionResponse{}, unavailable(
			"document_conversion", false, errors.New("analysis client is not configured"),
		)
	}
	configurationHash, err := cacheHash(struct {
		Extension string `json:"extension"`
		MediaType string `json:"media_type"`
	}{Extension: request.Extension, MediaType: request.MediaType})
	if err != nil {
		return DocumentConversionResponse{}, unavailable("document_conversion_cache", false, err)
	}
	cacheKey, cacheEnabled, err := c.cacheKey(
		CapabilityMarkItDown, configurationHash, request.ContentSHA256,
	)
	if err != nil {
		return DocumentConversionResponse{}, unavailable("document_conversion_cache", false, err)
	}
	if cacheEnabled {
		var cached DocumentConversionResponse
		found, err := c.readCache(ctx, cacheKey, CapabilityMarkItDown, &cached)
		if err != nil {
			return DocumentConversionResponse{}, unavailable("document_conversion_cache", false, err)
		}
		if found {
			if err := cached.Validate(); err != nil {
				return DocumentConversionResponse{}, unavailable("document_conversion_cache", false, err)
			}
			return cached, nil
		}
	}

	reader, contentType, contentLength, err := documentMultipart(request)
	if err != nil {
		return DocumentConversionResponse{}, failed("document_conversion_request", false, err)
	}
	payload, err := c.do(
		ctx, http.MethodPost, "/v1/documents/convert", reader, contentType, contentLength,
	)
	if err != nil {
		return DocumentConversionResponse{}, err
	}
	var response DocumentConversionResponse
	if err := decodeSingleJSON(payload, &response); err != nil {
		return DocumentConversionResponse{}, failed("document_conversion_response", false, err)
	}
	if err := response.Validate(); err != nil {
		return DocumentConversionResponse{}, failed("document_conversion_response", false, err)
	}
	if cacheEnabled {
		if err := c.writeCache(ctx, cacheKey, CapabilityMarkItDown, response); err != nil {
			return DocumentConversionResponse{}, unavailable("document_conversion_cache", false, err)
		}
	}
	return response, nil
}

// Validate proves version, UTF-8, title, and bounded warning semantics.
func (r DocumentConversionResponse) Validate() error {
	if r.ContractVersion != ContractVersion {
		return fmt.Errorf("document conversion contract version %d is incompatible", r.ContractVersion)
	}
	if !utf8.ValidString(r.Markdown) {
		return errors.New("document conversion markdown is not valid UTF-8")
	}
	if r.Title != nil {
		if !utf8.ValidString(*r.Title) || strings.TrimSpace(*r.Title) != *r.Title {
			return errors.New("document conversion title is invalid")
		}
		if *r.Title == "" {
			return errors.New("document conversion title must be null instead of empty")
		}
	}
	if len(r.Warnings) > maxDocumentWarnings {
		return errors.New("document conversion returned too many warnings")
	}
	for _, warning := range r.Warnings {
		if !validDocumentWarning(warning) {
			return errors.New("document conversion returned an invalid warning")
		}
	}
	return nil
}

func validateDocumentConversionRequest(request DocumentConversionRequest) error {
	if request.Content == nil {
		return errors.New("document content stream is required")
	}
	if request.ContentLength <= 0 || request.ContentLength > MaxDocumentContentBytes {
		return fmt.Errorf("document content length must be between 1 and %d bytes", MaxDocumentContentBytes)
	}
	if !sha256HexPattern.MatchString(request.ContentSHA256) {
		return errors.New("document content SHA-256 is invalid")
	}
	filename := strings.TrimSpace(request.Filename)
	if filename == "" || filename != request.Filename || filepath.Base(filename) != filename ||
		strings.ContainsAny(filename, `/\\`) || !utf8.ValidString(filename) {
		return errors.New("document filename is invalid")
	}
	wantedMediaType, ok := supportedDocumentMediaType(request.Extension)
	if !ok {
		return fmt.Errorf("document extension %q is unsupported", request.Extension)
	}
	if request.MediaType != wantedMediaType {
		return fmt.Errorf("document media type %q does not match extension %q", request.MediaType, request.Extension)
	}
	return nil
}

// SupportsDocumentConversion reports whether a filename and canonical media
// type identify one of the bounded rich-document conversions exposed by the
// current protocol.
func SupportsDocumentConversion(filename, mediaType string) bool {
	if filename == "" || strings.TrimSpace(filename) != filename ||
		filepath.Base(filename) != filename ||
		strings.ContainsAny(filename, `/\\`) || !utf8.ValidString(filename) {
		return false
	}
	wantedMediaType, supported := supportedDocumentMediaType(
		strings.ToLower(filepath.Ext(filename)),
	)
	return supported && mediaType == wantedMediaType
}

// IsDocumentConversionCandidate reports whether either side of the supplied
// format identifies a rich-document conversion. Candidates that do not pass
// SupportsDocumentConversion contain mismatched source metadata.
func IsDocumentConversionCandidate(filename, mediaType string) bool {
	if _, supported := supportedDocumentMediaType(
		strings.ToLower(filepath.Ext(strings.TrimSpace(filename))),
	); supported {
		return true
	}
	switch mediaType {
	case "application/pdf",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.openxmlformats-officedocument.presentationml.presentation",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"text/html":
		return true
	default:
		return false
	}
}

func supportedDocumentMediaType(extension string) (string, bool) {
	switch extension {
	case ".pdf":
		return "application/pdf", true
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document", true
	case ".pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation", true
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", true
	case ".html", ".htm":
		return "text/html", true
	default:
		return "", false
	}
}

func documentMultipart(request DocumentConversionRequest) (io.Reader, string, int64, error) {
	var framing bytes.Buffer
	multipartWriter := multipart.NewWriter(&framing)
	contentType := multipartWriter.FormDataContentType()
	fields := []struct{ name, value string }{
		{name: "contract_version", value: strconv.Itoa(ContractVersion)},
		{name: "extension", value: request.Extension},
		{name: "media_type", value: request.MediaType},
		{name: "content_sha256", value: request.ContentSHA256},
	}
	for _, field := range fields {
		if err := multipartWriter.WriteField(field.name, field.value); err != nil {
			return nil, "", 0, err
		}
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(
		`form-data; name="file"; filename="%s"`, escapeMultipartFilename(request.Filename),
	))
	header.Set("Content-Type", request.MediaType)
	if _, err := multipartWriter.CreatePart(header); err != nil {
		return nil, "", 0, err
	}
	prefixLength := framing.Len()
	if err := multipartWriter.Close(); err != nil {
		return nil, "", 0, err
	}
	framingBytes := framing.Bytes()
	prefix := append([]byte(nil), framingBytes[:prefixLength]...)
	suffix := append([]byte(nil), framingBytes[prefixLength:]...)
	body := io.MultiReader(
		bytes.NewReader(prefix),
		&exactDocumentReader{source: request.Content, remaining: request.ContentLength},
		bytes.NewReader(suffix),
	)
	return body, contentType, int64(len(prefix)+len(suffix)) + request.ContentLength, nil
}

type exactDocumentReader struct {
	source    io.Reader
	remaining int64
	checked   bool
}

func (r *exactDocumentReader) Read(buffer []byte) (int, error) {
	if r.remaining > 0 {
		if int64(len(buffer)) > r.remaining {
			buffer = buffer[:r.remaining]
		}
		read, err := r.source.Read(buffer)
		r.remaining -= int64(read)
		if errors.Is(err, io.EOF) && r.remaining > 0 {
			return read, io.ErrUnexpectedEOF
		}
		return read, err
	}
	if r.checked {
		return 0, io.EOF
	}
	r.checked = true
	var extra [1]byte
	read, err := r.source.Read(extra[:])
	if read > 0 || err == nil {
		return 0, errors.New("document content length changed while streaming")
	}
	if !errors.Is(err, io.EOF) {
		return 0, err
	}
	return 0, io.EOF
}

func escapeMultipartFilename(filename string) string {
	return strings.NewReplacer("\\", "_", `"`, "_").Replace(filename)
}

func validDocumentWarning(warning string) bool {
	if warning == "" || strings.TrimSpace(warning) != warning || !utf8.ValidString(warning) ||
		utf8.RuneCountInString(warning) > maxDocumentWarningRunes {
		return false
	}
	for _, value := range warning {
		if unicode.IsControl(value) {
			return false
		}
	}
	return true
}
