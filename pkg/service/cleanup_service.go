package service

import (
	"authservice/pkg/repository"
	"context"
	"log"
	"sync"
	"time"

	"gorm.io/gorm"
)

type CleanupService struct {
	repo *repository.AuthRepository
	stop chan struct{}
	wg   sync.WaitGroup
}

func NewCleanupService(db *gorm.DB) *CleanupService {
	return &CleanupService{
		repo: repository.NewAuthRepository(db),
		stop: make(chan struct{}),
	}
}

func (c *CleanupService) Start() {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		ticker := time.NewTicker(1 * time.Hour) // Run every hour
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// derive a bounded context for the cleanup run
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				c.cleanupExpiredSessions(ctx)
				cancel()
			case <-c.stop:
				return
			}
		}
	}()
}

func (c *CleanupService) cleanupExpiredSessions(ctx context.Context) {
	log.Println("Starting cleanup of expired sessions...")

	err := c.repo.DeleteExpiredSessions(ctx)
	if err != nil {
		log.Printf("Error cleaning up expired sessions: %v", err)
		return
	}

	log.Println("Expired sessions cleanup completed")
}

func (c *CleanupService) Stop() {
	close(c.stop)
	c.wg.Wait()
}
