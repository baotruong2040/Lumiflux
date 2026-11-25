package idgen

import "github.com/google/uuid"

// NewUUIDv7 generates a time-ordered UUIDv7 identifier.
func NewUUIDv7() (uuid.UUID, error) {
	return uuid.NewV7()
}

// MustUUIDv7 generates a UUIDv7 identifier and panics if generation fails.
func MustUUIDv7() uuid.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		panic(err)
	}
	return id
}
