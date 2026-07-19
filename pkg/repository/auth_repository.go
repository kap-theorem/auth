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

// GetUserByEmail resolves a user by email within an identity scope
// (email uniqueness is per scope: app or org).
func (r *AuthRepository) GetUserByEmail(ctx context.Context, email, scopeType, scopeID string) (*models.User, error) {
	var user models.User
	err := r.db.WithContext(ctx).Where("email_id = ? AND scope_type = ? AND scope_id = ?", email, scopeType, scopeID).First(&user).Error
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
func (r *AuthRepository) IsEmailExists(ctx context.Context, email, scopeType, scopeID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&models.User{}).Where("email_id = ? AND scope_type = ? AND scope_id = ?", email, scopeType, scopeID).Count(&count).Error
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

// Organization operations
func (r *AuthRepository) CreateOrganization(ctx context.Context, org *models.Organization) error {
	return r.db.WithContext(ctx).Create(org).Error
}

func (r *AuthRepository) GetOrganizationByID(ctx context.Context, orgID string) (*models.Organization, error) {
	var org models.Organization
	err := r.db.WithContext(ctx).Where("org_id = ?", orgID).First(&org).Error
	if err != nil {
		return nil, err
	}
	return &org, nil
}

func (r *AuthRepository) ListOrganizations(ctx context.Context) ([]models.Organization, error) {
	var orgs []models.Organization
	err := r.db.WithContext(ctx).Order("created_at ASC").Find(&orgs).Error
	if err != nil {
		return nil, err
	}
	return orgs, nil
}

// Developer operations
func (r *AuthRepository) CreateDeveloper(ctx context.Context, dev *models.Developer) error {
	return r.db.WithContext(ctx).Create(dev).Error
}

func (r *AuthRepository) GetDeveloperByID(ctx context.Context, developerID string) (*models.Developer, error) {
	var dev models.Developer
	err := r.db.WithContext(ctx).Where("developer_id = ?", developerID).First(&dev).Error
	if err != nil {
		return nil, err
	}
	return &dev, nil
}

func (r *AuthRepository) GetDeveloperByEmail(ctx context.Context, email string) (*models.Developer, error) {
	var dev models.Developer
	err := r.db.WithContext(ctx).Where("email = ?", email).First(&dev).Error
	if err != nil {
		return nil, err
	}
	return &dev, nil
}

func (r *AuthRepository) ListDevelopers(ctx context.Context) ([]models.Developer, error) {
	var devs []models.Developer
	err := r.db.WithContext(ctx).Order("created_at ASC").Find(&devs).Error
	if err != nil {
		return nil, err
	}
	return devs, nil
}

// Client (app) management operations
func (r *AuthRepository) ListClientsByOrg(ctx context.Context, orgID string) ([]models.Client, error) {
	var clients []models.Client
	err := r.db.WithContext(ctx).Where("org_id = ?", orgID).Order("created_at ASC").Find(&clients).Error
	if err != nil {
		return nil, err
	}
	return clients, nil
}

func (r *AuthRepository) ListAllClients(ctx context.Context) ([]models.Client, error) {
	var clients []models.Client
	err := r.db.WithContext(ctx).Order("created_at ASC").Find(&clients).Error
	if err != nil {
		return nil, err
	}
	return clients, nil
}

func (r *AuthRepository) UpdateClient(ctx context.Context, client *models.Client) error {
	return r.db.WithContext(ctx).Save(client).Error
}

// DeleteClient soft-deletes a client (app).
func (r *AuthRepository) DeleteClient(ctx context.Context, clientID string) error {
	return r.db.WithContext(ctx).Delete(&models.Client{}, "client_id = ?", clientID).Error
}

// SetClientSuspended flips a client's suspended flag. Suspended clients fail
// all RPC authentication.
func (r *AuthRepository) SetClientSuspended(ctx context.Context, clientID string, suspended bool) error {
	return r.db.WithContext(ctx).
		Model(&models.Client{}).
		Where("client_id = ?", clientID).
		Update("suspended", suspended).Error
}

// Relation tuple operations
// UpsertTuples writes tuples, overwriting the condition of an existing tuple
// with the same primary key.
func (r *AuthRepository) UpsertTuples(ctx context.Context, tuples []models.RelationTuple) error {
	if len(tuples) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{UpdateAll: true}).
		Create(&tuples).Error
}

// DeleteTuple removes one tuple by its full primary key (condition_expr is
// not part of the key).
func (r *AuthRepository) DeleteTuple(ctx context.Context, t *models.RelationTuple) error {
	return r.db.WithContext(ctx).Delete(&models.RelationTuple{},
		"client_id = ? AND object_type = ? AND object_id = ? AND relation = ? AND subject_type = ? AND subject_id = ? AND effect = ?",
		t.ClientID, t.ObjectType, t.ObjectID, t.Relation, t.SubjectType, t.SubjectID, t.Effect).Error
}

// ListTuples returns a client's tuples, optionally filtered by object and/or
// subject (empty filter fields match anything).
func (r *AuthRepository) ListTuples(ctx context.Context, clientID, objectType, objectID, subjectType, subjectID string) ([]models.RelationTuple, error) {
	q := r.db.WithContext(ctx).Where("client_id = ?", clientID)
	if objectType != "" {
		q = q.Where("object_type = ?", objectType)
	}
	if objectID != "" {
		q = q.Where("object_id = ?", objectID)
	}
	if subjectType != "" {
		q = q.Where("subject_type = ?", subjectType)
	}
	if subjectID != "" {
		q = q.Where("subject_id = ?", subjectID)
	}
	var tuples []models.RelationTuple
	if err := q.Order("created_at ASC").Find(&tuples).Error; err != nil {
		return nil, err
	}
	return tuples, nil
}

// FindMatchingTuples returns tuples exactly matching an (object, relation,
// subject) triple with the given effect — the Phase 2 minimal Check lookup.
func (r *AuthRepository) FindMatchingTuples(ctx context.Context, clientID, objectType, objectID, relation, subjectType, subjectID, effect string) ([]models.RelationTuple, error) {
	var tuples []models.RelationTuple
	err := r.db.WithContext(ctx).
		Where("client_id = ? AND object_type = ? AND object_id = ? AND relation = ? AND subject_type = ? AND subject_id = ? AND effect = ?",
			clientID, objectType, objectID, relation, subjectType, subjectID, effect).
		Find(&tuples).Error
	if err != nil {
		return nil, err
	}
	return tuples, nil
}

// Authz model operations
// CreateAuthzModelVersion stores model JSON as the next version for the
// client and returns the new version number.
func (r *AuthRepository) CreateAuthzModelVersion(ctx context.Context, clientID, modelJSON string) (int32, error) {
	var version int32
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var latest int32
		if err := tx.Model(&models.AuthzModel{}).
			Where("client_id = ?", clientID).
			Select("COALESCE(MAX(version), 0)").
			Scan(&latest).Error; err != nil {
			return err
		}
		version = latest + 1
		return tx.Create(&models.AuthzModel{
			ClientID:  clientID,
			Version:   version,
			ModelJSON: modelJSON,
		}).Error
	})
	if err != nil {
		return 0, err
	}
	return version, nil
}

// GetLatestAuthzModel returns the newest model version for a client.
func (r *AuthRepository) GetLatestAuthzModel(ctx context.Context, clientID string) (*models.AuthzModel, error) {
	var model models.AuthzModel
	err := r.db.WithContext(ctx).
		Where("client_id = ?", clientID).
		Order("version DESC").
		First(&model).Error
	if err != nil {
		return nil, err
	}
	return &model, nil
}

// Platform metrics
func (r *AuthRepository) CountDevelopers(ctx context.Context) (int64, error) {
	return r.count(ctx, &models.Developer{})
}

func (r *AuthRepository) CountOrganizations(ctx context.Context) (int64, error) {
	return r.count(ctx, &models.Organization{})
}

func (r *AuthRepository) CountClients(ctx context.Context) (int64, error) {
	return r.count(ctx, &models.Client{})
}

func (r *AuthRepository) CountUsers(ctx context.Context) (int64, error) {
	return r.count(ctx, &models.User{})
}

func (r *AuthRepository) CountActiveSessions(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&models.Session{}).Where("expires_at > ?", time.Now()).Count(&count).Error
	return count, err
}

func (r *AuthRepository) CountTuples(ctx context.Context) (int64, error) {
	return r.count(ctx, &models.RelationTuple{})
}

func (r *AuthRepository) count(ctx context.Context, model any) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(model).Count(&count).Error
	return count, err
}
