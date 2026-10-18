package auth

import (
	"encoding/base64"
	"fmt"
	"testing"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

func TestVerifyAdminPasswordAcceptsBcryptAndArgon2idHashes(t *testing.T) {
	bcryptHash, err := bcrypt.GenerateFromPassword([]byte("correct horse"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	salt := []byte("0123456789abcdef")
	digest := argon2.IDKey([]byte("correct horse"), salt, 1, 8, 1, 32)
	argon2Hash := fmt.Sprintf("$argon2id$v=19$m=8,t=1,p=1$%s$%s",
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(digest))

	for _, test := range []struct {
		name     string
		hash     string
		password string
		want     bool
	}{
		{"bcrypt correct password", string(bcryptHash), "correct horse", true},
		{"bcrypt wrong password", string(bcryptHash), "battery staple", false},
		{"bcrypt password prefix", string(bcryptHash), "correct", false},
		{"argon2id correct password", argon2Hash, "correct horse", true},
		{"argon2id wrong password", argon2Hash, "battery staple", false},
		{"argon2id empty password", argon2Hash, "", false},
		{"empty hash never matches", "", "", false},
		{"malformed hash never matches", "$argon2id$v=19$m=8,t=1,p=1$%%%$%%%", "correct horse", false},
		{"unknown scheme never matches", "$md5$whatever", "correct horse", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := VerifyAdminPassword(test.hash, test.password); got != test.want {
				t.Fatalf("VerifyAdminPassword() = %v; want %v", got, test.want)
			}
		})
	}
}
