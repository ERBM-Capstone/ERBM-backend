package dto

// Meta contains pagination information for list responses, including a zero total.
type Meta struct {
	Page    int `json:"page,omitempty"`
	PerPage int `json:"per_page,omitempty"`
	Total   int `json:"total"`
}
type ListQuery struct {
	Page     int `form:"page" binding:"omitempty,min=1"`
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=100"`
}
type URIParam struct {
	ID int64 `uri:"id" binding:"required,min=1"`
}
