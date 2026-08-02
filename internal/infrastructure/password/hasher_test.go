package password

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestHasher(t *testing.T) {
	hasher := Hasher{}
	hash, err := hasher.Hash("correct-password")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") || hash == "correct-password" || !hasher.Verify(hash, "correct-password") || hasher.Verify(hash, "wrong-password") {
		t.Fatal("Argon2id hash verification failed")
	}
	longPassword := strings.Repeat("🙂", 250)
	longHash, err := hasher.Hash(longPassword)
	if err != nil || !hasher.Verify(longHash, longPassword) {
		t.Fatalf("long password verification failed: %v", err)
	}
	legacy, err := bcrypt.GenerateFromPassword([]byte("legacy-password"), bcrypt.DefaultCost)
	if err != nil || !hasher.Verify(string(legacy), "legacy-password") {
		t.Fatalf("legacy bcrypt verification failed: %v", err)
	}
}
