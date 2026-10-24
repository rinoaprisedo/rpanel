package seeders

import (
	"log"

	"rpanel/backend/modules/users"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// SetAdmin rewrites the seeded superadmin's credentials. Credentials come
// from env (not argv) so the password never shows up in `ps`.
func SetAdmin(db *gorm.DB, email, password string) {
	if email == "" || password == "" {
		log.Fatal("RPANEL_ADMIN_EMAIL and RPANEL_ADMIN_PASSWORD are required")
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		log.Fatalf("failed to hash password: %v", err)
	}
	var user users.User
	if err := db.Joins("Role").Where("\"Role\".is_superadmin = ?", true).Order("users.id").First(&user).Error; err != nil {
		log.Fatalf("superadmin user not found (run seed first): %v", err)
	}
	if err := db.Model(&user).Updates(map[string]any{"email": email, "password": string(hashed)}).Error; err != nil {
		log.Fatalf("failed to update admin: %v", err)
	}
	log.Printf("admin credentials updated for %s", email)
}
