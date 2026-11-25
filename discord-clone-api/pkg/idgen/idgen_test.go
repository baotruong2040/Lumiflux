package idgen

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewUUIDv7(t *testing.T) {
	id1, err := NewUUIDv7()
	require.NoError(t, err)
	assert.Equal(t, uuid.Version(7), id1.Version())
	assert.Equal(t, uuid.RFC4122, id1.Variant())

	id2 := MustUUIDv7()
	assert.Equal(t, uuid.Version(7), id2.Version())
	assert.NotEqual(t, id1, id2)
}
