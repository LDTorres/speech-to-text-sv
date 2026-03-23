package users

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	DefaultListLimit = 20
	MaxListLimit     = 100
)

type User struct {
	ID        int64
	Name      string
	Email     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type CreateUserInput struct {
	Name  string
	Email string
}

type ListUsersParams struct {
	Limit  int
	Offset int
}

type RepositoryContract interface {
	Create(ctx context.Context, user UserModel) (UserModel, error)
	GetByID(ctx context.Context, id int64) (UserModel, error)
	List(ctx context.Context, limit int, offset int) ([]UserModel, error)
}

type Service struct {
	repository RepositoryContract
}

type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

type NotFoundError struct {
	Resource string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s not found", e.Resource)
}

type UnexpectedError struct {
	Cause error
}

func (e *UnexpectedError) Error() string {
	return "unexpected error"
}

func NewService(repository RepositoryContract) *Service {
	return &Service{repository: repository}
}

func (s *Service) CreateUser(ctx context.Context, input CreateUserInput) (User, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return User{}, &ValidationError{Message: "name is required"}
	}

	email := strings.TrimSpace(input.Email)
	if email == "" {
		return User{}, &ValidationError{Message: "email is required"}
	}

	parsedAddress, err := mail.ParseAddress(email)
	if err != nil || parsedAddress.Address != email {
		return User{}, &ValidationError{Message: "email must be a valid email address"}
	}

	createdUser, err := s.repository.Create(ctx, UserModel{
		Name:  name,
		Email: email,
	})
	if err != nil {
		return User{}, &UnexpectedError{Cause: err}
	}

	return mapModelToUser(createdUser), nil
}

func (s *Service) GetUserByID(ctx context.Context, id int64) (User, error) {
	if id <= 0 {
		return User{}, &ValidationError{Message: "id must be greater than zero"}
	}

	userModel, err := s.repository.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return User{}, &NotFoundError{Resource: "user"}
		}

		return User{}, &UnexpectedError{Cause: err}
	}

	return mapModelToUser(userModel), nil
}

func (s *Service) ListUsers(ctx context.Context, params ListUsersParams) ([]User, error) {
	if params.Limit <= 0 {
		return nil, &ValidationError{Message: "limit must be greater than zero"}
	}

	if params.Limit > MaxListLimit {
		return nil, &ValidationError{Message: fmt.Sprintf("limit must be less than or equal to %d", MaxListLimit)}
	}

	if params.Offset < 0 {
		return nil, &ValidationError{Message: "offset must be greater than or equal to zero"}
	}

	userModels, err := s.repository.List(ctx, params.Limit, params.Offset)
	if err != nil {
		return nil, &UnexpectedError{Cause: err}
	}

	users := make([]User, 0, len(userModels))
	for _, userModel := range userModels {
		users = append(users, mapModelToUser(userModel))
	}

	return users, nil
}

func mapModelToUser(model UserModel) User {
	return User{
		ID:        model.ID,
		Name:      model.Name,
		Email:     model.Email,
		CreatedAt: model.CreatedAt,
		UpdatedAt: model.UpdatedAt,
	}
}
