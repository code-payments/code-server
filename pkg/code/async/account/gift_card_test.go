package async_account

/*

func TestGiftCardAutoReturn_ExpiryWindow(t *testing.T) {
	for _, tc := range []struct {
		creationTs     time.Time
		isAutoReturned bool
	}{
		{
			creationTs:     time.Now(),
			isAutoReturned: false,
		},
		{
			creationTs:     time.Now().Add(-giftCardExpiry + time.Minute),
			isAutoReturned: false,
		},
		{
			creationTs:     time.Now().Add(-giftCardExpiry),
			isAutoReturned: true,
		},
		{
			creationTs:     time.Now().Add(-giftCardExpiry - time.Minute),
			isAutoReturned: true,
		},
	} {
		env := setup(t)

		giftCard := env.generateRandomGiftCard(t, tc.creationTs)

		require.NoError(t, env.service.maybeInitiateGiftCardAutoReturn(env.ctx, giftCard.accountInfoRecord))

		if tc.isAutoReturned {
			env.assertGiftCardAutoReturned(t, giftCard)
		} else {
			env.assertGiftCardNotAutoReturned(t, giftCard, tc.isAutoReturned)
		}
	}
}

func TestGiftCardAutoReturn_AlreadyClaimed(t *testing.T) {
	env := setup(t)

	giftCard1 := env.generateRandomGiftCard(t, time.Now())
	env.simulateGiftCardBeingClaimed(t, giftCard1)

	giftCard2 := env.generateRandomGiftCard(t, time.Now().Add(-giftCardExpiry-24*time.Hour))
	env.simulateGiftCardBeingClaimed(t, giftCard2)

	require.NoError(t, env.service.maybeInitiateGiftCardAutoReturn(env.ctx, giftCard1.accountInfoRecord))
	env.assertGiftCardNotAutoReturned(t, giftCard1, true)

	require.NoError(t, env.service.maybeInitiateGiftCardAutoReturn(env.ctx, giftCard2.accountInfoRecord))
	env.assertGiftCardNotAutoReturned(t, giftCard2, true)
}

func TestGiftCardAutoReturn_IntentId(t *testing.T) {
	intentId1 := testutil.NewRandomAccount(t).PublicKey().ToBase58()
	intentId2 := testutil.NewRandomAccount(t).PublicKey().ToBase58()

	generated1 := getAutoReturnIntentId(intentId1)
	generated2 := getAutoReturnIntentId(intentId2)

	assert.NotEqual(t, intentId1, generated1)
	assert.NotEqual(t, intentId2, generated2)
	assert.NotEqual(t, generated1, generated2)

	for _, generated := range []string{generated1, generated2} {
		decoded, err := base58.Decode(generated)
		require.NoError(t, err)
		assert.Len(t, decoded, 32)
	}

	for i := 0; i < 100; i++ {
		assert.Equal(t, generated1, getAutoReturnIntentId(intentId1))
		assert.Equal(t, generated2, getAutoReturnIntentId(intentId2))
	}
}

*/
