package repository

import (
	"authservice/pkg/models"
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

func (r *AuthRepository) GetUserByIdentifier(ctx context.Context, identifier string) (*models.User, error) {
	var user models.User
	err := r.db.WithContext(ctx).Where("email_id = ? OR user_name = ?", identifier, identifier).First(&user).Error
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

func (r *AuthRepository) GetAllClients(ctx context.Context) ([]models.Client, error) {
	var clients []models.Client
	err := r.db.WithContext(ctx).Order("client_name ASC").Find(&clients).Error
	return clients, err
}

func (r *AuthRepository) ValidateClient(ctx context.Context, clientID, clientSecret string) (*models.Client, error) {
	var client models.Client
	err := r.db.WithContext(ctx).Where("client_id = ? AND client_secret = ?", clientID, clientSecret).First(&client).Error
	if err != nil {
		return nil, err
	}
	return &client, nil
}

// UpdateClientSecret updates the client's secret value
func (r *AuthRepository) UpdateClientSecret(ctx context.Context, clientID, newSecret string) error {
	return r.db.WithContext(ctx).
		Model(&models.Client{}).
		Where("client_id = ?", clientID).
		Update("client_secret", newSecret).Error
}

func (r *AuthRepository) GetClientConfig(ctx context.Context, clientID string) (*models.Client, error) {
	var client models.Client
	err := r.db.WithContext(ctx).Select("client_id", "demo_mode", "invite_only", "login_type").Where("client_id = ?", clientID).First(&client).Error
	if err != nil {
		return nil, err
	}
	return &client, nil
}

func (r *AuthRepository) UpdateClientConfig(ctx context.Context, clientID string, demoMode, inviteOnly bool, loginType string) error {
	return r.db.WithContext(ctx).
		Model(&models.Client{}).
		Where("client_id = ?", clientID).
		Updates(map[string]interface{}{
			"demo_mode":   demoMode,
			"invite_only": inviteOnly,
			"login_type":  loginType,
		}).Error
}

// Session operations
func (r *AuthRepository) CreateOrUpdateSession(ctx context.Context, session *models.Session) error {
	// This will either create or update based on the composite primary key (user_id + client_id)
	// We use OnConflict to avoid updating the created_at field with zero values
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "client_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"refresh_token", "user_agent", "expires_at", "updated_at", "deleted_at"}),
	}).Create(session).Error
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

func (r *AuthRepository) IsUsernameExists(ctx context.Context, username string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&models.User{}).Where("user_name = ?", username).Count(&count).Error
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

func (r *AuthRepository) GetSystemStats(ctx context.Context) (totalUsers, totalClients, activeSessions int64, err error) {
	err = r.db.WithContext(ctx).Model(&models.User{}).Count(&totalUsers).Error
	if err != nil {
		return
	}
	err = r.db.WithContext(ctx).Model(&models.Client{}).Count(&totalClients).Error
	if err != nil {
		return
	}
	err = r.db.WithContext(ctx).Model(&models.Session{}).Where("expires_at > ?", time.Now()).Count(&activeSessions).Error
	return
}

func (r *AuthRepository) GetUsersByClientID(ctx context.Context, clientID string) ([]models.User, error) {
	var users []models.User
	err := r.db.WithContext(ctx).Where("client_id = ?", clientID).Order("created_at DESC").Find(&users).Error
	return users, err
}

func (r *AuthRepository) GetClientStats(ctx context.Context, clientID string) (totalUsers, last24hLogins int64, err error) {
	err = r.db.WithContext(ctx).Model(&models.User{}).Where("client_id = ?", clientID).Count(&totalUsers).Error
	if err != nil {
		return
	}
	last24h := time.Now().Add(-24 * time.Hour)
	err = r.db.WithContext(ctx).Model(&models.Session{}).Where("client_id = ? AND created_at > ?", clientID, last24h).Count(&last24hLogins).Error
	return
}

// Password Reset Token operations
func (r *AuthRepository) CreatePasswordResetToken(ctx context.Context, token *models.PasswordResetToken) error {
	return r.db.WithContext(ctx).Create(token).Error
}

func (r *AuthRepository) GetPasswordResetToken(ctx context.Context, token string) (*models.PasswordResetToken, error) {
	var resetToken models.PasswordResetToken
	err := r.db.WithContext(ctx).Where("token = ? AND used = ? AND expires_at > ?", token, false, time.Now()).First(&resetToken).Error
	if err != nil {
		return nil, err
	}
	return &resetToken, nil
}

func (r *AuthRepository) MarkPasswordResetTokenUsed(ctx context.Context, token string) error {
	return r.db.WithContext(ctx).Model(&models.PasswordResetToken{}).Where("token = ?", token).Update("used", true).Error
}

func (r *AuthRepository) DeleteExpiredPasswordResetTokens(ctx context.Context) error {
	return r.db.WithContext(ctx).Delete(&models.PasswordResetToken{}, "expires_at < ? OR used = ?", time.Now(), true).Error
}

// Count active sessions for a user (excluding a specific refresh token)
func (r *AuthRepository) CountUserSessions(ctx context.Context, userID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&models.Session{}).Where("user_id = ? AND expires_at > ?", userID, time.Now()).Count(&count).Error
	return count, err
}

// Delete all sessions except the one with the given refresh token
func (r *AuthRepository) DeleteOtherUserSessions(ctx context.Context, userID string, keepRefreshToken string) (int64, error) {
	result := r.db.WithContext(ctx).Where("user_id = ? AND refresh_token != ?", userID, keepRefreshToken).Delete(&models.Session{})
	return result.RowsAffected, result.Error
}

// Invite Token operations
func (r *AuthRepository) CreateInviteToken(ctx context.Context, token *models.InviteToken) error {
	return r.db.WithContext(ctx).Create(token).Error
}

func (r *AuthRepository) GetInviteToken(ctx context.Context, token string) (*models.InviteToken, error) {
	var inviteToken models.InviteToken
	err := r.db.WithContext(ctx).Where("token = ? AND used = ? AND expires_at > ?", token, false, time.Now()).First(&inviteToken).Error
	if err != nil {
		return nil, err
	}
	return &inviteToken, nil
}

func (r *AuthRepository) MarkInviteTokenUsed(ctx context.Context, token string) error {
	return r.db.WithContext(ctx).Model(&models.InviteToken{}).Where("token = ?", token).Update("used", true).Error
}
