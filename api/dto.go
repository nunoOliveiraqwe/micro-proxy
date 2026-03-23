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

type LoginResponse struct {
	Token string `json:"token"`
}
