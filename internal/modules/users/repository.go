package users

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, user UserModel) (UserModel, error) {
	if err := r.db.WithContext(ctx).Create(&user).Error; err != nil {
		return UserModel{}, fmt.Errorf("create user: %w", err)
	}

	return user, nil
}

func (r *Repository) GetByID(ctx context.Context, id int64) (UserModel, error) {
	var user UserModel

	if err := r.db.WithContext(ctx).First(&user, id).Error; err != nil {
		return UserModel{}, fmt.Errorf("get user by id: %w", err)
	}

	return user, nil
}

func (r *Repository) List(ctx context.Context, limit int, offset int) ([]UserModel, error) {
	var users []UserModel

	if err := r.db.WithContext(ctx).
		Order("id ASC").
		Limit(limit).
		Offset(offset).
		Find(&users).Error; err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}

	return users, nil
}
