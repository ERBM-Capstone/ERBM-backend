# HTTP OCR and LLM provider

`HTTPProvider` implements `Provider` using the existing services in `AI/`:

1. POST the document to `<OCRBaseURL>/ocr` as multipart field `file`.
2. POST `{"ocr_text":"..."}` to `<LLMBaseURL>/api/extract`.
3. Return normalized draft candidates in `Result`.

Construct the provider and pass it through the existing service options:

```go
provider, err := ocr.NewHTTPProvider(ocr.HTTPConfig{
    OCRBaseURL: "http://localhost:8000",
    LLMBaseURL: "http://localhost:8081",
    Timeout:    30 * time.Second,
})
if err != nil {
    // Handle invalid deployment configuration at startup.
    return err
}
options := service.ExpenseOptions{Storage: stores, OCR: provider}
```

These URLs assume the Go server runs on the host and the LLM host port is 8081.
For containers, supply URLs reachable from the backend's own Docker network.
The existing LLM Compose files publish 8080 by default, also the Go server's
default port. Configure distinct host ports when running both on one host.

`OCRToken` and `LLMToken` optionally set Bearer authorization independently.
`OCRModel` and `LLMModel` describe deployed models for extraction metadata;
the Python services select their models using their own configuration.
Never put real credentials in source code. Redirects are rejected.

The timeout bounds the entire pipeline, with no automatic retries. The caller's
earlier deadline still wins. Server startup passes `AI_REQUEST_TIMEOUT` to both
the provider and `ExpenseOptions.OCRTimeout`, so ExpenseService's deadline and
PROCESSING lock expiry use the same duration. The default is 30 seconds; omitted,
invalid or non-positive environment values fall back to that default. Direct
callers must configure both options consistently.

JSON numbers are converted to decimal strings exactly, including exponent
notation. Null totals and absent/null optional item values remain nil, zero
remains `"0.00"`, and excessive precision, overflow and negative amounts are
rejected rather than rounded.
The LLM schema defines monetary values in THB; the provider records that currency.

`Result.Raw` contains `ocr` and `llm` objects with the original response fields,
including `needs_review`, warnings and taxes. Each response is limited to 4 MB
and combined evidence to 8 MB. Upstream error bodies and transport details are
not included in returned errors. Context cancellation interrupts HTTP I/O; it
does not guarantee that a remote Python service stops inference already running.

PDF page counts and `pages_text` must match the input. The current LLM schema
does not identify which page each item came from. For multipage documents,
normalized `LineItems` are therefore omitted; original items remain in
`Raw.llm.items` with an `adapter_warnings` entry. Totals and document fields are
still returned for review. Single-page documents receive normal numbered items.

Run local HTTP and ExpenseService integration tests without invoking real AI:

```sh
go test ./tests -run '^TestHTTPProvider' -count=1
```

The repository currently ignores `/tests/`; explicitly include
`tests/ocr_http_provider_test.go` when preparing a Git commit.
