package solana

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSignatureStatus(t *testing.T) {
	zero, one := 0, 1

	testCases := []struct {
		s         SignatureStatus
		confirmed bool
		finalized bool
	}{
		{
			s: SignatureStatus{
				Slot:               10,
				ErrorResult:        nil,
				Confirmations:      &zero,
				ConfirmationStatus: "",
			},
		},
		{
			s: SignatureStatus{
				Slot:               10,
				ErrorResult:        nil,
				Confirmations:      &zero,
				ConfirmationStatus: "random",
			},
		},
		{
			s: SignatureStatus{
				Slot:               10,
				ErrorResult:        nil,
				Confirmations:      &zero,
				ConfirmationStatus: confirmationStatusProcessed,
			},
		},
		{
			s: SignatureStatus{
				Slot:               10,
				ErrorResult:        nil,
				Confirmations:      &one,
				ConfirmationStatus: "",
			},
			confirmed: true,
		},
		{
			s: SignatureStatus{
				Slot:               10,
				ErrorResult:        nil,
				Confirmations:      &zero,
				ConfirmationStatus: confirmationStatusConfirmed,
			},
			confirmed: true,
		},
		{
			s: SignatureStatus{
				Slot:               10,
				ErrorResult:        nil,
				Confirmations:      &zero,
				ConfirmationStatus: confirmationStatusFinalized,
			},
			confirmed: true,
			finalized: true,
		},
	}

	for _, tc := range testCases {
		assert.Equal(t, tc.confirmed, tc.s.Confirmed())
		assert.Equal(t, tc.finalized, tc.s.Finalized())
	}
}
