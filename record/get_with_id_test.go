package record_test

import (
	"context"
	"testing"

	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/record"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type gwUser struct {
	Name string `json:"name"`
}

func (gwUser) CollectionName() string { return "gwusers" }

func TestGetWithID(t *testing.T) {
	ctx := context.Background()
	db := dalgo2memory.NewDB()
	users := dal.CollectionOf[string, gwUser]()

	require.NoError(t, db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return users.SetByID(ctx, tx, "u1", gwUser{Name: "Alice"})
	}))

	// Success: returns a typed WithID carrying the id, key, and decoded record.
	// (K and T are inferred from the collection handle and id.)
	got, err := record.GetWithID(ctx, users, db, "u1") //nolint:staticcheck // covers the deprecated forwarder
	require.NoError(t, err)
	assert.Equal(t, "u1", got.ID)
	require.NotNil(t, got.Key)
	assert.Equal(t, "gwusers/u1", got.Key.String())
	require.NotNil(t, got.Record)
	assert.Equal(t, "Alice", got.Record.Data().(*gwUser).Name)

	// Not-found: returns the zero WithID and the not-found error.
	missing, err := record.GetWithID(ctx, users, db, "missing") //nolint:staticcheck // covers the deprecated forwarder
	require.Error(t, err)
	assert.True(t, dal.IsNotFound(err))
	assert.Equal(t, record.WithID[string]{}, missing)
}
