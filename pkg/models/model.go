package models

import (
	"time"

	"gorm.io/gorm"
)

type Client struct {
	ClientID     string         `gorm:"column:client_id;primaryKey;size:36" json:"client_id"`
	ClientName   string         `gorm:"size:100;not null" json:"client_name"`
	ClientSecret string         `gorm:"size:255;not null" json:"-"`
	CreatedAt    time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

type User struct {
	UserID    string         `gorm:"column:user_id;primaryKey;size:36" json:"user_id"`
	UserName  string         `gorm:"column:user_name;size:100;not null" json:"username"`
	Email     string         `gorm:"column:email_id;size:255;uniqueIndex;not null" json:"email"`
	Password  string         `gorm:"size:255;not null" json:"-"`
	ClientID  string         `gorm:"column:client_id;size:36;not null;index" json:"client_id"`
	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

type Session struct {
	UserID       string         `gorm:"column:user_id;primaryKey;size:36" json:"user_id"`
	ClientID     string         `gorm:"column:client_id;primaryKey;size:36" json:"client_id"`
	RefreshToken string         `gorm:"size:255;uniqueIndex;not null" json:"-"`
	UserAgent    string         `gorm:"size:500" json:"user_agent"`
	ExpiresAt    time.Time      `gorm:"not null" json:"expires_at"`
	CreatedAt    time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

func GetAllModels() []any {
	return []any{
		&Client{},  // Create clients table first (parent)
		&User{},    // Then users table (references clients)
		&Session{}, // Finally sessions table (references both users and clients)
	}
}
