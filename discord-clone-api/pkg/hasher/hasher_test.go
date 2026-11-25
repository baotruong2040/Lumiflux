package hasher

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestBcryptHasher(t *testing.T) {
	h := NewBcryptHasher(bcrypt.MinCost)

	hashed, err := h.Hash("SuperSecret123!")
	require.NoError(t, err)
	assert.NotEmpty(t, hashed)
	assert.NotEqual(t, "SuperSecret123!", hashed)

	assert.NoError(t, h.Compare(hashed, "SuperSecret123!"))
	assert.Error(t, h.Compare(hashed, "WrongPassword!"))
}
