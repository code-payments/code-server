package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/code/data/event"
)

func RunTests(t *testing.T, s event.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s event.Store){
		testHappyPath,
	} {
		tf(t, s)
		teardown()
	}
}

func testHappyPath(t *testing.T, s event.Store) {
	t.Run("testHappyPath", func(t *testing.T) {
		ctx := context.Background()

		start := time.Now()

		expected := &event.Record{
			EventId:   "event_id",
			EventType: event.MicroPayment,

			SourceCodeAccount:      "source_code_account",
			DestinationCodeAccount: pointer.String("destination_code_account"),
			ExternalTokenAccount:   pointer.String("external_token_account"),

			SourceIdentity:      "source_identity",
			DestinationIdentity: pointer.String("destination_identity"),

			SourceClientIp:           pointer.String("source_client_ip"),
			SourceClientCity:         pointer.String("source_client_city"),
			SourceClientCountry:      pointer.String("source_client_country"),
			DestinationClientIp:      pointer.String("destination_client_ip"),
			DestinationClientCity:    pointer.String("destination_client_city"),
			DestinationClientCountry: pointer.String("destination_client_country"),

			UsdValue: pointer.Float64(123.456),

			SpamConfidence: 0.75,
		}

		_, err := s.Get(ctx, expected.EventId)
		assert.Equal(t, event.ErrEventNotFound, err)

		cloned := expected.Clone()
		require.NoError(t, s.Save(ctx, expected))

		assert.EqualValues(t, 1, expected.Id)
		assert.True(t, expected.CreatedAt.After(start))
		assert.True(t, expected.CreatedAt.Before(start.Add(500*time.Millisecond)))

		actual, err := s.Get(ctx, cloned.EventId)
		require.NoError(t, err)
		assert.EqualValues(t, 1, actual.Id)
		assert.EqualValues(t, expected.CreatedAt.Unix(), actual.CreatedAt.Unix())
		assertEquivalentRecords(t, &cloned, actual)

		expected.DestinationCodeAccount = pointer.String("destination_code_account_updated")
		expected.DestinationIdentity = pointer.String("destination_identity_updated")
		expected.DestinationClientIp = pointer.String("destination_client_ip_updated")
		expected.DestinationClientCity = pointer.String("destination_client_city_updated")
		expected.DestinationClientCountry = pointer.String("destination_client_country_updated")
		expected.SpamConfidence = 0.5

		cloned = expected.Clone()
		require.NoError(t, s.Save(ctx, expected))
		assert.EqualValues(t, 1, expected.Id)

		actual, err = s.Get(ctx, cloned.EventId)
		require.NoError(t, err)
		assert.EqualValues(t, 1, actual.Id)
		assert.EqualValues(t, expected.CreatedAt.Unix(), actual.CreatedAt.Unix())
		assertEquivalentRecords(t, &cloned, actual)
	})
}

func assertEquivalentRecords(t *testing.T, obj1, obj2 *event.Record) {
	assert.Equal(t, obj1.EventId, obj2.EventId)
	assert.Equal(t, obj1.EventType, obj2.EventType)

	assert.Equal(t, obj1.SourceCodeAccount, obj2.SourceCodeAccount)
	assert.EqualValues(t, obj1.DestinationCodeAccount, obj2.DestinationCodeAccount)
	assert.EqualValues(t, obj1.ExternalTokenAccount, obj2.ExternalTokenAccount)

	assert.Equal(t, obj1.SourceIdentity, obj2.SourceIdentity)
	assert.EqualValues(t, obj1.DestinationIdentity, obj2.DestinationIdentity)

	assert.EqualValues(t, obj1.SourceClientIp, obj2.SourceClientIp)
	assert.EqualValues(t, obj1.SourceClientCity, obj2.SourceClientCity)
	assert.EqualValues(t, obj1.SourceClientCountry, obj2.SourceClientCountry)
	assert.EqualValues(t, obj1.DestinationClientIp, obj2.DestinationClientIp)
	assert.EqualValues(t, obj1.DestinationClientCity, obj2.DestinationClientCity)
	assert.EqualValues(t, obj1.DestinationClientCountry, obj2.DestinationClientCountry)

	assert.EqualValues(t, obj1.UsdValue, obj2.UsdValue)

	assert.Equal(t, obj1.SpamConfidence, obj2.SpamConfidence)
}
