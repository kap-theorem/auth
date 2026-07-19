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

// GetUserByEmail resolves a user by email within a client scope
// (email uniqueness is per client).
func (r *AuthRepository) GetUserByEmail(ctx context.Context, email, clientID string) (*models.User, error) {
	var user models.User
	err := r.db.WithContext(ctx).Where("email_id = ? AND client_id = ?", email, clientID).First(&user).Error
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

// UpdateClientSecretHash updates the client's stored secret hash.
func (r *AuthRepository) UpdateClientSecretHash(ctx context.Context, clientID, newSecretHash string) error {
	return r.db.WithContext(ctx).
		Model(&models.Client{}).
		Where("client_id = ?", clientID).
		Update("client_secret", newSecretHash).Error
}

// Session operations
func (r *AuthRepository) CreateSession(ctx context.Context, session *models.Session) error {
	return r.db.WithContext(ctx).Create(session).Error
}

func (r *AuthRepository) UpdateSession(ctx context.Context, session *models.Session) error {
	return r.db.WithContext(ctx).Save(session).Error
}

func (r *AuthRepository) GetSessionByID(ctx context.Context, sessionID string) (*models.Session, error) {
	var session models.Session
	err := r.db.WithContext(ctx).Where("session_id = ? AND expires_at > ?", sessionID, time.Now()).First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// GetSessionsByUserAndClient lists all active sessions for a user under a client.
func (r *AuthRepository) GetSessionsByUserAndClient(ctx context.Context, userID, clientID string) ([]models.Session, error) {
	var sessions []models.Session
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND client_id = ? AND expires_at > ?", userID, clientID, time.Now()).
		Order("created_at ASC").
		Find(&sessions).Error
	if err != nil {
		return nil, err
	}
	return sessions, nil
}

func (r *AuthRepository) DeleteSessionByID(ctx context.Context, sessionID string) error {
	return r.db.WithContext(ctx).Delete(&models.Session{}, "session_id = ?", sessionID).Error
}

// DeleteUserClientSessions removes every session a user holds under a client.
func (r *AuthRepository) DeleteUserClientSessions(ctx context.Context, userID, clientID string) (int64, error) {
	result := r.db.WithContext(ctx).Delete(&models.Session{}, "user_id = ? AND client_id = ?", userID, clientID)
	return result.RowsAffected, result.Error
}

// DeleteOtherUserSessions removes all of a user's sessions except the one to keep.
func (r *AuthRepository) DeleteOtherUserSessions(ctx context.Context, userID, keepSessionID string) error {
	return r.db.WithContext(ctx).Delete(&models.Session{}, "user_id = ? AND session_id <> ?", userID, keepSessionID).Error
}

func (r *AuthRepository) DeleteAllUserSessions(ctx context.Context, userID string) error {
	return r.db.WithContext(ctx).Delete(&models.Session{}, "user_id = ?", userID).Error
}

func (r *AuthRepository) DeleteExpiredSessions(ctx context.Context) error {
	return r.db.WithContext(ctx).Delete(&models.Session{}, "expires_at < ?", time.Now()).Error
}

// Utility functions
func (r *AuthRepository) IsEmailExists(ctx context.Context, email, clientID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&models.User{}).Where("email_id = ? AND client_id = ?", email, clientID).Count(&count).Error
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
