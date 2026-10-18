package auth

import (
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

// VerifyAdminPassword checks password against an administrator hash in bcrypt
// or argon2id PHC form. Malformed hashes never match.
func VerifyAdminPassword(encodedHash, password string) bool {
	if strings.HasPrefix(encodedHash, "$argon2id$") {
		return verifyArgon2id(encodedHash, password)
	}
	if strings.HasPrefix(encodedHash, "$2a$") || strings.HasPrefix(encodedHash, "$2b$") || strings.HasPrefix(encodedHash, "$2y$") {
		return bcrypt.CompareHashAndPassword([]byte(encodedHash), []byte(password)) == nil
	}
	return false
}

// verifyArgon2id parses $argon2id$v=19$m=..,t=..,p=..$salt$hash and compares the digest in constant time.
func verifyArgon2id(encodedHash, password string) bool {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[2] != "v=19" {
		return false
	}
	params := map[string]uint64{}
	for _, field := range strings.Split(parts[3], ",") {
		name, raw, ok := strings.Cut(field, "=")
		if !ok {
			return false
		}
		value, err := strconv.ParseUint(raw, 10, 32)
		if err != nil || value == 0 {
			return false
		}
		params[name] = value
	}
	memory, okM := params["m"]
	iterations, okT := params["t"]
	parallelism, okP := params["p"]
	if !okM || !okT || !okP || len(params) != 3 || parallelism > 255 {
		return false
	}
	salt, err := decodeB64(parts[4])
	if err != nil {
		return false
	}
	want, err := decodeB64(parts[5])
	if err != nil || len(want) == 0 || len(want) > 1<<16 {
		return false
	}
	digest := argon2.IDKey([]byte(password), salt, uint32(iterations), uint32(memory), uint8(parallelism), uint32(len(want)))
	return subtle.ConstantTimeCompare(digest, want) == 1
}

func decodeB64(value string) ([]byte, error) {
	decoded, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(value)
	}
	if err != nil {
		return nil, fmt.Errorf("decode base64: %w", err)
	}
	return decoded, nil
}
