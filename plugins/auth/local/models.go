package local

type CreateUserRequest struct {
	Username string  `json:"username"`
	Password *string `json:"password,omitempty"`
	Enabled  bool    `json:"enabled"`
}

type CreateUserResponse struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Message  string `json:"message"`
}

type SetUserPasswordRequest struct {
	Password string `json:"password"`
}

type SetUserEnabledRequest struct {
	Enabled bool `json:"enabled"`
}

type SetUserServiceRequest struct {
	Priority int `json:"priority"`
}

type SetUserAttributeRequest struct {
	Attribute string `json:"attribute"`
	Value     string `json:"value"`
	Op        string `json:"op"`
}

type SetServiceAttributeRequest struct {
	Attribute string `json:"attribute"`
	Value     string `json:"value"`
	Op        string `json:"op"`
}

type CreateServiceRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type CreateServiceResponse struct {
	ServiceID   int64  `json:"service_id"`
	Name        string `json:"name"`
	Message     string `json:"message"`
}

type OperResponse struct {
	Message string `json:"message"`
}
