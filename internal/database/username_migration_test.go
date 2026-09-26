package database

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"authservice/pkg/models"
)

// TestMigrateUsernameUniqueness verifies that pre-existing duplicate usernames
// within a scope are reconciled (oldest keeps the name, the rest are suffixed)
// so the unique index can be added.
func TestMigrateUsernameUniqueness(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	dbCon := &DBConnection{DB: db}

	// Drop the unique index so we can insert duplicates that predate it.
	if err := dbCon.Migrator().DropIndex(&models.User{}, "idx_users_username_scope"); err != nil {
		t.Fatalf("drop index: %v", err)
	}

	base := time.Now()
	seed := func(id, name, scopeID string, order int) {
		u := models.User{
			UserID: id, UserName: name, Email: id + "@x.com",
			Password: "x", ScopeType: "app", ScopeID: scopeID, Active: true,
			CreatedAt: base.Add(time.Duration(order) * time.Second),
		}
		if err := db.Create(&u).Error; err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	// Three "dave" in app A (oldest = d1); one "dave" in app B (must be untouched).
	seed("d1", "dave", "A", 1)
	seed("d2", "dave", "A", 2)
	seed("d3", "dave", "A", 3)
	seed("d4", "dave", "B", 1)
	seed("e1", "erin", "A", 4) // distinct, untouched

	if err := dbCon.migrateUsernameUniqueness(); err != nil {
		t.Fatalf("migration: %v", err)
	}

	name := func(id string) string {
		var u models.User
		if err := db.First(&u, "user_id = ?", id).Error; err != nil {
			t.Fatalf("lookup %s: %v", id, err)
		}
		return u.UserName
	}

	if got := name("d1"); got != "dave" {
		t.Fatalf("oldest should keep name, got %q", got)
	}
	if got := name("d2"); got != "dave-2" {
		t.Fatalf("d2 want dave-2, got %q", got)
	}
	if got := name("d3"); got != "dave-3" {
		t.Fatalf("d3 want dave-3, got %q", got)
	}
	if got := name("d4"); got != "dave" {
		t.Fatalf("other scope must be untouched, got %q", got)
	}
	if got := name("e1"); got != "erin" {
		t.Fatalf("distinct username must be untouched, got %q", got)
	}

	// After reconciliation the unique index can now be created.
	if err := db.AutoMigrate(&models.User{}); err != nil {
		t.Fatalf("re-automigrate (index add) failed: %v", err)
	}
}
