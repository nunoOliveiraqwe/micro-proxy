package api

// --- FTS DTOs ---

type FtsStatusResponse struct {
	IsFtsCompleted bool `json:"isFtsCompleted"`
}

type CompleteFtsRequest struct {
	Password string `json:"password"`
}

// --- Auth DTOs ---

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type ChangePasswordRequest struct {
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
}

type UserIdentityResponse struct {
	Username string `json:"username"`
}
