package main

import (
"context"
"fmt"

"authservice/internal/database"
"authservice/pkg/repository"
)

func main() {
db := database.GetDBConnection()
repo := repository.NewAuthRepository(db.DB)

// Test email lookup
u1, err := repo.GetUserByIdentifier(context.Background(), "admin@example.com")
if err != nil {
fmt.Printf("Email lookup failed: %v\n", err)
} else {
fmt.Printf("Email lookup success: %s (Client: %s)\n", u1.Email, u1.ClientID)
}

// Test username lookup
u2, err := repo.GetUserByIdentifier(context.Background(), "admin")
if err != nil {
fmt.Printf("Username lookup failed: %v\n", err)
} else {
fmt.Printf("Username lookup success: %s (Client: %s)\n", u2.UserName, u2.ClientID)
}
}
