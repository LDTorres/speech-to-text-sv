package users

import "time"

type CreateUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type UserResponse struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UserListResponse struct {
	Users  []UserResponse `json:"users"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

type responseError struct {
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

type envelope struct {
	Data  any            `json:"data"`
	Error *responseError `json:"error"`
}

func newUserResponse(user User) UserResponse {
	return UserResponse{
		ID:        user.ID,
		Name:      user.Name,
		Email:     user.Email,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}
}

func newUserListResponse(users []User, limit int, offset int) UserListResponse {
	items := make([]UserResponse, 0, len(users))

	for _, user := range users {
		items = append(items, newUserResponse(user))
	}

	return UserListResponse{
		Users:  items,
		Limit:  limit,
		Offset: offset,
	}
}

func newSuccessEnvelope(data any) envelope {
	return envelope{
		Data:  data,
		Error: nil,
	}
}

func newErrorEnvelope(message string, code string) envelope {
	return envelope{
		Data: nil,
		Error: &responseError{
			Message: message,
			Code:    code,
		},
	}
}
