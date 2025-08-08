package repository

import (
	"authservice/pkg/models"
	"context"
	"time"

	"gorm.io/gorm"
)

type AuthRepository struct {
	db *gorm.DB
}

func NewAuthRepository(db *gorm.DB) *AuthRepository {
	return &AuthRepository{db: db}
}

// User operations
func (r *AuthRepository) CreateUser(ctx context.Context, user *models.User) error {
	return r.db.WithContext(ctx).Create(user).Error
}

func (r *AuthRepository) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	var user models.User
	err := r.db.WithContext(ctx).Where("email_id = ?", email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *AuthRepository) GetUserByID(ctx context.Context, userID string) (*models.User, error) {
	var user models.User
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *AuthRepository) UpdateUser(ctx context.Context, user *models.User) error {
	return r.db.WithContext(ctx).Save(user).Error
}

func (r *AuthRepository) DeleteUser(ctx context.Context, userID string) error {
	return r.db.WithContext(ctx).Delete(&models.User{}, "user_id = ?", userID).Error
}

// Client operations
func (r *AuthRepository) CreateClient(ctx context.Context, client *models.Client) error {
	return r.db.WithContext(ctx).Create(client).Error
}

func (r *AuthRepository) GetClientByID(ctx context.Context, clientID string) (*models.Client, error) {
	var client models.Client
	err := r.db.WithContext(ctx).Where("client_id = ?", clientID).First(&client).Error
	if err != nil {
		return nil, err
	}
	return &client, nil
}

func (r *AuthRepository) ValidateClient(ctx context.Context, clientID, clientSecret string) (*models.Client, error) {
	var client models.Client
	err := r.db.WithContext(ctx).Where("client_id = ? AND client_secret = ?", clientID, clientSecret).First(&client).Error
	if err != nil {
		return nil, err
	}
	return &client, nil
}

// Session operations
func (r *AuthRepository) CreateOrUpdateSession(ctx context.Context, session *models.Session) error {
	// This will either create or update based on the composite primary key (UserId + ClientId)
	return r.db.WithContext(ctx).Save(session).Error
}

func (r *AuthRepository) GetSessionByUserAndClient(ctx context.Context, userID, clientID string) (*models.Session, error) {
	var session models.Session
	err := r.db.WithContext(ctx).Where("user_id = ? AND client_id = ? AND expires_at > ?", userID, clientID, time.Now()).First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *AuthRepository) GetSessionByRefreshToken(ctx context.Context, refreshToken string) (*models.Session, error) {
	var session models.Session
	err := r.db.WithContext(ctx).Where("refresh_token = ? AND expires_at > ?", refreshToken, time.Now()).First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *AuthRepository) DeleteSessionByUserAndClient(ctx context.Context, userID, clientID string) error {
	return r.db.WithContext(ctx).Delete(&models.Session{}, "user_id = ? AND client_id = ?", userID, clientID).Error
}

func (r *AuthRepository) DeleteSessionByRefreshToken(ctx context.Context, refreshToken string) error {
	return r.db.WithContext(ctx).Delete(&models.Session{}, "refresh_token = ?", refreshToken).Error
}

func (r *AuthRepository) DeleteAllUserSessions(ctx context.Context, userID string) error {
	return r.db.WithContext(ctx).Delete(&models.Session{}, "user_id = ?", userID).Error
}

func (r *AuthRepository) DeleteExpiredSessions(ctx context.Context) error {
	return r.db.WithContext(ctx).Delete(&models.Session{}, "expires_at < ?", time.Now()).Error
}

// Utility functions
func (r *AuthRepository) IsEmailExists(ctx context.Context, email string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&models.User{}).Where("email_id = ?", email).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *AuthRepository) IsClientExists(ctx context.Context, clientID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&models.Client{}).Where("client_id = ?", clientID).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
