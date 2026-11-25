package hasher

import "golang.org/x/crypto/bcrypt"

// BcryptHasher hashes and verifies passwords using bcrypt.
type BcryptHasher struct {
	cost int
}

// NewBcryptHasher creates a BcryptHasher with the given cost (clamped to DefaultCost if invalid).
func NewBcryptHasher(cost int) *BcryptHasher {
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		cost = bcrypt.DefaultCost
	}
	return &BcryptHasher{cost: cost}
}

// Hash computes a bcrypt hash of the plaintext password.
func (h *BcryptHasher) Hash(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), h.cost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// Compare verifies that the plaintext password matches the bcrypt hash.
func (h *BcryptHasher) Compare(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
