package ocr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/ERBM-Capstone/ERBM-backend/internal/pkg/money"
	"github.com/ERBM-Capstone/ERBM-backend/internal/storage"
)

const maxHTTPResponseBytes = 4_000_000

// HTTPConfig configures the OCR /ocr -> LLM /api/extract pipeline. Base URLs
// may include a path prefix. Tokens, when supplied, use Bearer authentication.
type HTTPConfig struct {
	OCRBaseURL string
	LLMBaseURL string
	OCRToken   string
	LLMToken   string
	OCRModel   string
	LLMModel   string
	// Timeout bounds both requests together; a caller's earlier deadline wins.
	// Zero defaults to 30 seconds, matching ExpenseService's default file deadline.
	Timeout time.Duration
	Client  *http.Client
}

// HTTPProvider implements Provider against the services under AI/OCR and
// AI/llm/EBRM-LLM. Extraction only supplies draft candidates; it never writes DBs.
type HTTPProvider struct {
	ocrURL, llmURL     string
	ocrToken, llmToken string
	model              string
	timeout            time.Duration
	client             *http.Client
}

var _ Provider = (*HTTPProvider)(nil)

func NewHTTPProvider(cfg HTTPConfig) (*HTTPProvider, error) {
	ocrURL, err := httpEndpoint(cfg.OCRBaseURL, "/ocr")
	if err != nil {
		return nil, fmt.Errorf("invalid OCR base URL: %w", err)
	}
	llmURL, err := httpEndpoint(cfg.LLMBaseURL, "/api/extract")
	if err != nil {
		return nil, fmt.Errorf("invalid LLM base URL: %w", err)
	}
	if cfg.Timeout < 0 || strings.ContainsAny(cfg.OCRToken+cfg.LLMToken, "\r\n") {
		return nil, errors.New("invalid AI timeout or authentication configuration")
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.OCRModel == "" {
		cfg.OCRModel = "typhoon-ocr"
	}
	if cfg.LLMModel == "" {
		cfg.LLMModel = "scb10x/llama3.1-typhoon2-8b-instruct"
	}
	model := cfg.OCRModel + " + " + cfg.LLMModel
	if utf8.RuneCountInString(model) > 100 {
		return nil, errors.New("combined AI model identifier exceeds 100 characters")
	}
	client := http.Client{}
	if cfg.Client != nil {
		client = *cfg.Client
	}
	// Do not redirect receipt bytes, OCR text or credentials to another endpoint.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &HTTPProvider{ocrURL: ocrURL, llmURL: llmURL, ocrToken: cfg.OCRToken,
		llmToken: cfg.LLMToken, model: model, timeout: cfg.Timeout, client: &client}, nil
}

func httpEndpoint(base, path string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("expected an HTTP(S) URL without credentials, query or fragment")
	}
	return strings.TrimRight(u.String(), "/") + path, nil
}

func (*HTTPProvider) Name() string    { return "typhoon-ocr-llm" }
func (p *HTTPProvider) Model() string { return p.model }

type httpOCRResponse struct {
	Pages     int      `json:"pages"`
	Text      string   `json:"text"`
	PagesText []string `json:"pages_text"`
}

type httpInvoice struct {
	InvoiceNo        *string           `json:"invoice_no"`
	Date             *string           `json:"date"`
	Vendor           *string           `json:"vendor"`
	AmountTotal      json.RawMessage   `json:"amount_total"`
	Items            []httpInvoiceItem `json:"items"`
	ExtractionMethod string            `json:"extraction_method"`
}

type httpInvoiceItem struct {
	Description string          `json:"description"`
	Quantity    json.RawMessage `json:"quantity"`
	UnitPrice   json.RawMessage `json:"unit_price"`
	Amount      json.RawMessage `json:"amount"`
}

func (p *HTTPProvider) Extract(ctx context.Context, doc Document) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p == nil || p.client == nil {
		return nil, dto.ErrOCRUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	filename := map[string]string{"image/jpeg": "receipt.jpg", "image/png": "receipt.png", "application/pdf": "receipt.pdf"}[doc.ContentType]
	if filename == "" || len(doc.Bytes) == 0 || int64(len(doc.Bytes)) > storage.MaxReceiptBytes || doc.PageCount < 1 || doc.PageCount > 5 {
		return nil, dto.ErrOCRFailed
	}

	// ExpenseService already validates and reads the bounded file from storage.
	var upload bytes.Buffer
	writer := multipart.NewWriter(&upload)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename))
	header.Set("Content-Type", doc.ContentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return nil, dto.ErrOCRFailed
	}
	if _, err := part.Write(doc.Bytes); err != nil {
		return nil, dto.ErrOCRFailed
	}
	if err := writer.Close(); err != nil {
		return nil, dto.ErrOCRFailed
	}
	ocrRaw, err := p.post(ctx, p.ocrURL, p.ocrToken, writer.FormDataContentType(), &upload)
	if err != nil {
		return nil, fmt.Errorf("OCR request: %w", err)
	}
	var text httpOCRResponse
	if json.Unmarshal(ocrRaw, &text) != nil || text.Pages != doc.PageCount || strings.TrimSpace(text.Text) == "" {
		return nil, dto.ErrOCRFailed
	}
	if (doc.ContentType == "application/pdf" || text.PagesText != nil) && len(text.PagesText) != doc.PageCount {
		return nil, dto.ErrOCRFailed
	}

	payload, err := json.Marshal(struct {
		Text string `json:"ocr_text"`
	}{text.Text})
	if err != nil {
		return nil, dto.ErrOCRFailed
	}
	llmRaw, err := p.post(ctx, p.llmURL, p.llmToken, "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("LLM request: %w", err)
	}
	result, err := normalizeHTTPInvoice(llmRaw, doc.PageCount)
	if err != nil {
		return nil, err
	}
	warnings := []string(nil)
	if doc.PageCount > 1 && len(result.LineItems) > 0 {
		// The current LLM contract has no page_no. ExpenseService treats 0 as 1,
		// so emitting these rows would falsely attribute them to the first page.
		// Keep them in raw.llm.items for review instead of guessing source pages.
		result.LineItems = nil
		warnings = append(warnings, "MULTIPAGE_ITEMS_WITHOUT_PAGE_NUMBERS: see llm.items; normalized line_items omitted")
	}
	result.Raw, err = json.Marshal(struct {
		OCR      json.RawMessage `json:"ocr"`
		LLM      json.RawMessage `json:"llm"`
		Warnings []string        `json:"adapter_warnings,omitempty"`
	}{ocrRaw, llmRaw, warnings})
	if err != nil || len(result.Raw) > 8_000_000 {
		return nil, dto.ErrOCRFailed
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (p *HTTPProvider) post(ctx context.Context, endpoint, token, contentType string, body io.Reader) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return nil, dto.ErrOCRUnavailable
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := p.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, context.DeadlineExceeded
		}
		// Transport errors contain URLs and may contain credentials; don't expose them.
		return nil, dto.ErrOCRUnavailable
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return nil, context.DeadlineExceeded
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests, http.StatusServiceUnavailable:
		return nil, dto.ErrOCRUnavailable
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, dto.ErrOCRFailed
	}
	b, err := io.ReadAll(io.LimitReader(response.Body, maxHTTPResponseBytes+1))
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return nil, context.DeadlineExceeded
	}
	if err != nil || len(b) > maxHTTPResponseBytes || !json.Valid(b) {
		return nil, dto.ErrOCRFailed
	}
	return b, nil
}

func normalizeHTTPInvoice(raw json.RawMessage, pages int) (*Result, error) {
	var fields map[string]json.RawMessage
	var invoice httpInvoice
	if json.Unmarshal(raw, &fields) != nil || fields == nil || json.Unmarshal(raw, &invoice) != nil {
		return nil, dto.ErrOCRFailed
	}
	for _, required := range []string{"invoice_no", "date", "vendor", "amount_total", "extraction_method"} {
		if _, ok := fields[required]; !ok {
			return nil, dto.ErrOCRFailed
		}
	}
	if invoice.ExtractionMethod != "llm" && invoice.ExtractionMethod != "fallback" {
		return nil, dto.ErrOCRFailed
	}
	for _, field := range []struct {
		value *string
		limit int
	}{{invoice.InvoiceNo, 100}, {invoice.Vendor, 255}} {
		if field.value != nil && (strings.TrimSpace(*field.value) == "" || utf8.RuneCountInString(*field.value) > field.limit) {
			return nil, dto.ErrOCRFailed
		}
	}
	if invoice.Date != nil {
		if _, err := time.Parse("2006-01-02", *invoice.Date); err != nil {
			return nil, dto.ErrOCRFailed
		}
	}
	amount, err := httpDecimal(invoice.AmountTotal, 13, 2, false)
	if err != nil || len(invoice.Items) > 1000 {
		return nil, dto.ErrOCRFailed
	}
	// The existing Thai Invoice Extraction schema explicitly defines amounts as
	// Thai Baht. This is contract metadata, not inferred from an arbitrary number.
	currency := "THB"
	result := &Result{PagesRead: pages, Amount: amount, Currency: &currency,
		ExpenseDate: invoice.Date, MerchantName: invoice.Vendor, InvoiceNo: invoice.InvoiceNo,
		LineItems: make([]dto.ExpenseLineItem, 0, len(invoice.Items))}
	for i, item := range invoice.Items {
		if strings.TrimSpace(item.Description) == "" || utf8.RuneCountInString(item.Description) > 10000 {
			return nil, dto.ErrOCRFailed
		}
		quantity, err := httpDecimal(item.Quantity, 12, 6, true)
		if err != nil {
			return nil, dto.ErrOCRFailed
		}
		unitPrice, err := httpDecimal(item.UnitPrice, 13, 2, false)
		if err != nil {
			return nil, dto.ErrOCRFailed
		}
		lineTotal, err := httpDecimal(item.Amount, 13, 2, false)
		if err != nil {
			return nil, dto.ErrOCRFailed
		}
		result.LineItems = append(result.LineItems, dto.ExpenseLineItem{LineNo: i + 1,
			PageNo: 1, Description: &item.Description, Quantity: quantity, UnitPrice: unitPrice, LineTotal: lineTotal})
	}
	return result, nil
}

// httpDecimal expands a JSON number exactly, including scientific notation,
// without converting through float64 or rounding away unsupported precision.
func httpDecimal(raw json.RawMessage, wholeMax, scale int, positive bool) (*string, error) {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return nil, nil
	}
	if len(s) > 64 || s[0] < '0' || s[0] > '9' || !json.Valid(raw) {
		return nil, dto.ErrOCRFailed
	}
	mantissa, exponent, _ := strings.Cut(strings.ToLower(s), "e")
	shift := 0
	if exponent != "" {
		var err error
		shift, err = strconv.Atoi(exponent)
		if err != nil || shift < -100 || shift > 100 {
			return nil, dto.ErrOCRFailed
		}
	}
	whole, fraction, _ := strings.Cut(mantissa, ".")
	digits, point := whole+fraction, len(whole)+shift
	switch {
	case point <= 0:
		whole, fraction = "0", strings.Repeat("0", -point)+digits
	case point >= len(digits):
		whole, fraction = digits+strings.Repeat("0", point-len(digits)), ""
	default:
		whole, fraction = digits[:point], digits[point:]
	}
	whole, fraction = strings.TrimLeft(whole, "0"), strings.TrimRight(fraction, "0")
	if whole == "" {
		whole = "0"
	}
	if len(whole) > wholeMax || len(fraction) > scale || (positive && whole == "0" && fraction == "") {
		return nil, dto.ErrOCRFailed
	}
	v := whole
	if fraction != "" {
		v += "." + fraction
	}
	if !positive {
		var err error
		v, err = money.Normalize(v)
		if err != nil {
			return nil, dto.ErrOCRFailed
		}
	}
	return &v, nil
}
