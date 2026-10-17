// Package pagination contains the shared paging defaults and bounds for list APIs.
package pagination

const (
	DefaultPage     = 1
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// WithDefaults replaces zero values only. Negative and oversized inputs remain
// unchanged so validation can reject them rather than silently correcting them.
func WithDefaults(page, pageSize int) (int, int) {
	if page == 0 {
		page = DefaultPage
	}
	if pageSize == 0 {
		pageSize = DefaultPageSize
	}
	return page, pageSize
}

// IsValid checks positive paging values and that (page-1)*pageSize fits in int.
// Callers choose whether to apply defaults before validation.
func IsValid(page, pageSize int) bool {
	if page < 1 || pageSize < 1 || pageSize > MaxPageSize {
		return false
	}
	maxInt := int(^uint(0) >> 1)
	return page-1 <= maxInt/pageSize
}
