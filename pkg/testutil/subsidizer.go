package testutil

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
)

func SetupRandomSubsidizer(t *testing.T, data code_data.Provider) *common.Account {
	account := NewRandomAccount(t)
	require.NoError(t, common.InjectTestSubsidizer(context.Background(), data, account))
	return account
}
