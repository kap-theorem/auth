package service

import (
	"authservice/pkg/repository"
	"log"
	"time"

	"gorm.io/gorm"
)

type CleanupService struct {
	repo *repository.AuthRepository
}

func NewCleanupService(db *gorm.DB) *CleanupService {
	return &CleanupService{
		repo: repository.NewAuthRepository(db),
	}
}

func (c *CleanupService) StartCleanupJob() {
	ticker := time.NewTicker(1 * time.Hour) // Run every hour
	defer ticker.Stop()

	for range ticker.C {
		c.cleanupExpiredSessions()
	}
}

func (c *CleanupService) cleanupExpiredSessions() {
	log.Println("Starting cleanup of expired sessions...")

	err := c.repo.DeleteExpiredSessions()
	if err != nil {
		log.Printf("Error cleaning up expired sessions: %v", err)
		return
	}

	log.Println("Expired sessions cleanup completed")
}
