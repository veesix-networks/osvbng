package show

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/plugins/auth/local"
)

func init() {
	show.RegisterFactory(NewUsersHandler)
	show.RegisterFactory(NewUserHandler)
}

type UsersHandler struct {
	deps *deps.ShowDeps
}

type UserHandler struct {
	deps *deps.ShowDeps
}

type UsersResponse struct {
	Users []UserInfo `json:"users"`
}

type UserInfo struct {
	ID            int64             `json:"id"`
	Username      string            `json:"username"`
	Enabled       bool              `json:"enabled"`
	HasPassword   bool              `json:"has_password"`
	Services      []string          `json:"services,omitempty"`
	Attributes    map[string]string `json:"attributes,omitempty"`
	CreatedAt     string            `json:"created_at"`
}

func NewUsersHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &UsersHandler{deps: deps}
}

func NewUserHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &UserHandler{deps: deps}
}

func (h *UsersHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	provider := local.GetProvider()
	if provider == nil {
		return nil, fmt.Errorf("local auth provider not initialized")
	}

	db := provider.GetDB()

	users, err := local.ListUsers(db)
	if err != nil {
		return nil, err
	}

	var userInfos []UserInfo
	for _, user := range users {
		userInfos = append(userInfos, UserInfo{
			ID:          user.ID,
			Username:    user.Username,
			Enabled:     user.Enabled,
			HasPassword: user.Password != nil,
			CreatedAt:   user.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	return &UsersResponse{Users: userInfos}, nil
}

func (h *UsersHandler) PathPattern() paths.Path {
	return paths.Path(local.ShowUsersPath)
}

func (h *UsersHandler) Dependencies() []paths.Path {
	return nil
}

func (h *UserHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	provider := local.GetProvider()
	if provider == nil {
		return nil, fmt.Errorf("local auth provider not initialized")
	}

	db := provider.GetDB()

	userIDStr := extractUserIDFromPath(req.Path)
	if userIDStr == "" {
		return nil, fmt.Errorf("user ID required")
	}

	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID: %s", userIDStr)
	}

	user, err := local.GetUserByID(db, userID)
	if err != nil {
		return nil, err
	}

	userServices, err := local.GetUserServices(db, user.Username)
	if err != nil {
		return nil, err
	}

	var serviceNames []string
	for _, us := range userServices {
		service, err := local.GetServiceByID(db, us.ServiceID)
		if err != nil {
			continue
		}
		serviceNames = append(serviceNames, service.Name)
	}

	attributes, err := local.GetAttributes(db, local.EntityTypeUser, user.Username, local.AttributeTypeResponse)
	if err != nil {
		return nil, err
	}

	attrs := make(map[string]string)
	for _, attr := range attributes {
		attrs[attr.AttributeName] = attr.AttributeValue
	}

	return &UserInfo{
		ID:          user.ID,
		Username:    user.Username,
		Enabled:     user.Enabled,
		HasPassword: user.Password != nil,
		Services:    serviceNames,
		Attributes:  attrs,
		CreatedAt:   user.CreatedAt.Format("2006-01-02 15:04:05"),
	}, nil
}

func extractUserIDFromPath(path string) string {
	parts := strings.Split(path, ".")
	if len(parts) >= 5 && parts[0] == "subscriber" && parts[1] == "auth" && parts[2] == "local" && parts[3] == "users" {
		return parts[4]
	}
	return ""
}

func (h *UserHandler) PathPattern() paths.Path {
	return paths.Path(local.ShowUserPath)
}

func (h *UserHandler) Dependencies() []paths.Path {
	return nil
}
