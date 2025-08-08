package database

import (
	"log"
	"os"

	"github.com/joho/godotenv"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	models "authservice/pkg/models"
)

type DBConnection struct {
	*gorm.DB
}

func init() {
	if err := godotenv.Load(); err != nil {
		log.Println()
	}
}

func GetDBConnection() *DBConnection {

	connectionString := os.Getenv("DB_CONNECTION_STRING")

	db, err := gorm.Open(mysql.Open(connectionString), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	return &DBConnection{
		db,
	}
}

func (dbCon *DBConnection) CreateTables() {
	// Use safe migration that only creates tables if they don't exist
	err := dbCon.createTablesIfNotExist()
	if err != nil {
		log.Fatalf("Failed to create tables in database: %v", err)
	}

	log.Println("Database migration completed successfully")
}

func (dbCon *DBConnection) createTablesIfNotExist() error {
	// Use GORM AutoMigrate which only creates/updates schema if needed
	// This is safe and won't drop existing data

	log.Println("Running database migrations...")

	// Create Client table first (no dependencies)
	if err := dbCon.AutoMigrate(&models.Client{}); err != nil {
		log.Printf("Error migrating Client table: %v", err)
		return err
	}
	log.Println("Clients table migration completed")

	// Create User table second (depends on Client)
	if err := dbCon.AutoMigrate(&models.User{}); err != nil {
		log.Printf("Error migrating User table: %v", err)
		return err
	}
	log.Println("Users table migration completed")

	// Create Session table last (depends on User and Client)
	if err := dbCon.AutoMigrate(&models.Session{}); err != nil {
		log.Printf("Error migrating Session table: %v", err)
		return err
	}
	log.Println("Sessions table migration completed")

	// Add foreign key constraints if they don't exist
	dbCon.addForeignKeyConstraintsIfNotExist()

	return nil
}

func (dbCon *DBConnection) addForeignKeyConstraintsIfNotExist() {
	log.Println("Checking and adding foreign key constraints if needed...")

	// Check and add foreign key constraint for User -> Client
	if !dbCon.constraintExists("users", "fk_users_client_id") {
		result := dbCon.Exec(`
			ALTER TABLE users 
			ADD CONSTRAINT fk_users_client_id 
			FOREIGN KEY (client_id) REFERENCES clients(client_id) 
			ON UPDATE CASCADE ON DELETE CASCADE
		`)
		if result.Error != nil {
			log.Printf("Warning: Could not add user->client constraint: %v", result.Error)
		} else {
			log.Println("Added foreign key constraint: fk_users_client_id")
		}
	}

	// Check and add foreign key constraint for Session -> User
	if !dbCon.constraintExists("sessions", "fk_sessions_user_id") {
		result := dbCon.Exec(`
			ALTER TABLE sessions 
			ADD CONSTRAINT fk_sessions_user_id 
			FOREIGN KEY (user_id) REFERENCES users(user_id) 
			ON UPDATE CASCADE ON DELETE CASCADE
		`)
		if result.Error != nil {
			log.Printf("Warning: Could not add session->user constraint: %v", result.Error)
		} else {
			log.Println("Added foreign key constraint: fk_sessions_user_id")
		}
	}

	// Check and add foreign key constraint for Session -> Client
	if !dbCon.constraintExists("sessions", "fk_sessions_client_id") {
		result := dbCon.Exec(`
			ALTER TABLE sessions 
			ADD CONSTRAINT fk_sessions_client_id 
			FOREIGN KEY (client_id) REFERENCES clients(client_id) 
			ON UPDATE CASCADE ON DELETE CASCADE
		`)
		if result.Error != nil {
			log.Printf("Warning: Could not add session->client constraint: %v", result.Error)
		} else {
			log.Println("Added foreign key constraint: fk_sessions_client_id")
		}
	}

	log.Println("Foreign key constraints check completed")
}

func (dbCon *DBConnection) constraintExists(tableName, constraintName string) bool {
	var count int64
	dbCon.Raw(`
		SELECT COUNT(*) 
		FROM information_schema.table_constraints 
		WHERE table_schema = DATABASE() 
		AND table_name = ? 
		AND constraint_name = ?
	`, tableName, constraintName).Scan(&count)

	return count > 0
}

// CleanupExpiredSessions removes expired sessions from the database
func (dbCon *DBConnection) CleanupExpiredSessions() {
	result := dbCon.Where("expires_at < NOW()").Delete(&models.Session{})
	if result.Error != nil {
		log.Printf("Error cleaning up expired sessions: %v", result.Error)
	} else if result.RowsAffected > 0 {
		log.Printf("Cleaned up %d expired sessions", result.RowsAffected)
	}
}
