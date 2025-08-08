package repository

import (
	"authservice/pkg/models"
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
func (r *AuthRepository) CreateUser(user *models.User) error {
	return r.db.Create(user).Error
}

func (r *AuthRepository) GetUserByEmail(email string) (*models.User, error) {
	var user models.User
	err := r.db.Where("email_id = ?", email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *AuthRepository) GetUserByID(userID string) (*models.User, error) {
	var user models.User
	err := r.db.Where("user_id = ?", userID).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *AuthRepository) UpdateUser(user *models.User) error {
	return r.db.Save(user).Error
}

func (r *AuthRepository) DeleteUser(userID string) error {
	return r.db.Delete(&models.User{}, "user_id = ?", userID).Error
}

// Client operations
func (r *AuthRepository) CreateClient(client *models.Client) error {
	return r.db.Create(client).Error
}

func (r *AuthRepository) GetClientByID(clientID string) (*models.Client, error) {
	var client models.Client
	err := r.db.Where("client_id = ?", clientID).First(&client).Error
	if err != nil {
		return nil, err
	}
	return &client, nil
}

func (r *AuthRepository) ValidateClient(clientID, clientSecret string) (*models.Client, error) {
	var client models.Client
	err := r.db.Where("client_id = ? AND client_secret = ?", clientID, clientSecret).First(&client).Error
	if err != nil {
		return nil, err
	}
	return &client, nil
}

// Session operations
func (r *AuthRepository) CreateOrUpdateSession(session *models.Session) error {
	// This will either create or update based on the composite primary key (UserId + ClientId)
	return r.db.Save(session).Error
}

func (r *AuthRepository) GetSessionByUserAndClient(userID, clientID string) (*models.Session, error) {
	var session models.Session
	err := r.db.Where("user_id = ? AND client_id = ? AND expires_at > ?", userID, clientID, time.Now()).First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *AuthRepository) GetSessionByRefreshToken(refreshToken string) (*models.Session, error) {
	var session models.Session
	err := r.db.Where("refresh_token = ? AND expires_at > ?", refreshToken, time.Now()).First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *AuthRepository) DeleteSessionByUserAndClient(userID, clientID string) error {
	return r.db.Delete(&models.Session{}, "user_id = ? AND client_id = ?", userID, clientID).Error
}

func (r *AuthRepository) DeleteSessionByRefreshToken(refreshToken string) error {
	return r.db.Delete(&models.Session{}, "refresh_token = ?", refreshToken).Error
}

func (r *AuthRepository) DeleteAllUserSessions(userID string) error {
	return r.db.Delete(&models.Session{}, "user_id = ?", userID).Error
}

func (r *AuthRepository) DeleteExpiredSessions() error {
	return r.db.Delete(&models.Session{}, "expires_at < ?", time.Now()).Error
}

// Utility functions
func (r *AuthRepository) IsEmailExists(email string) (bool, error) {
	var count int64
	err := r.db.Model(&models.User{}).Where("email_id = ?", email).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *AuthRepository) IsClientExists(clientID string) (bool, error) {
	var count int64
	err := r.db.Model(&models.Client{}).Where("client_id = ?", clientID).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
