package dto

type SetInitialPasswordRequest struct {
	Token    string `json:"token" binding:"required,min=1"`
	Password string `json:"password" binding:"required,min=8,max=72"`
}

type ResendActivationRequest struct {
	Email string `json:"email" binding:"required,email,max=255"`
}
