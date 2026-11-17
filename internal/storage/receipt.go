package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"

	"github.com/ERBM-Capstone/ERBM-backend/dto"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	pdfmodel "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

const MaxReceiptBytes int64 = 10_000_000

func init() { api.DisableConfigDir() }

type ReceiptInfo struct {
	ContentType string
	Size        int64
	Pages       int
	SHA256      string
}
type guardedPDFReader struct {
	ctx       context.Context
	r         *bytes.Reader
	remaining int64
}

func (r *guardedPDFReader) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if r.remaining <= 0 {
		return 0, dto.ErrInvalidReceipt
	}
	if int64(len(b)) > r.remaining {
		b = b[:r.remaining]
	}
	n, err := r.r.Read(b)
	r.remaining -= int64(n)
	return n, err
}
func (r *guardedPDFReader) Seek(offset int64, whence int) (int64, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Seek(offset, whence)
}

// Inspect parses real content. PDF validation uses in-memory config and a read
// budget, and rejects encrypted PDFs even when their user password is empty.
func Inspect(ctx context.Context, b []byte) (info ReceiptInfo, err error) {
	defer func() {
		if recover() != nil {
			info = ReceiptInfo{}
			err = dto.ErrInvalidReceipt
		}
	}()
	if len(b) == 0 {
		return info, dto.ErrInvalidReceipt
	}
	if int64(len(b)) > MaxReceiptBytes {
		return info, dto.ErrFileTooLarge
	}
	if err := ctx.Err(); err != nil {
		return info, err
	}
	info = ReceiptInfo{ContentType: http.DetectContentType(b), Size: int64(len(b)), Pages: 1}
	sum := sha256.Sum256(b)
	info.SHA256 = hex.EncodeToString(sum[:])
	switch info.ContentType {
	case "image/jpeg", "image/png":
		cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
		if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 40_000_000 {
			return ReceiptInfo{}, dto.ErrInvalidReceipt
		}
		if _, _, err := image.Decode(bytes.NewReader(b)); err != nil {
			return ReceiptInfo{}, dto.ErrInvalidReceipt
		}
	case "application/pdf":
		conf := pdfmodel.NewDefaultConfiguration()
		conf.ValidationMode = pdfmodel.ValidationRelaxed
		reader := &guardedPDFReader{ctx: ctx, r: bytes.NewReader(b), remaining: 100_000_000}
		pdf, err := api.ReadAndValidate(reader, conf)
		if err != nil || pdf.Encrypt != nil || pdf.PageCount < 1 || pdf.PageCount > 5 {
			return ReceiptInfo{}, dto.ErrInvalidReceipt
		}
		info.Pages = pdf.PageCount
	default:
		return ReceiptInfo{}, dto.ErrUnsupportedReceipt
	}
	return info, nil
}

func ReadBounded(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, MaxReceiptBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > MaxReceiptBytes {
		return nil, dto.ErrFileTooLarge
	}
	return b, nil
}
