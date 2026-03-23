package users

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type repositoryStub struct {
	createCalls int
}

func (r *repositoryStub) Create(_ context.Context, _ UserModel) (UserModel, error) {
	r.createCalls++
	return UserModel{}, nil
}

func (r *repositoryStub) GetByID(_ context.Context, _ int64) (UserModel, error) {
	return UserModel{}, nil
}

func (r *repositoryStub) List(_ context.Context, _ int, _ int) ([]UserModel, error) {
	return nil, nil
}

func TestCreateUser_InvalidEmail(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		email string
	}{
		{
			name:  "missing domain",
			email: "user@",
		},
		{
			name:  "display name format",
			email: "User <user@example.com>",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			repository := &repositoryStub{}
			service := NewService(repository)

			_, err := service.CreateUser(context.Background(), CreateUserInput{
				Name:  "Test User",
				Email: testCase.email,
			})

			require.Error(t, err)
			require.IsType(t, &ValidationError{}, err)
			require.Equal(t, 0, repository.createCalls)
		})
	}
}
