package testutil

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
)

func NewRandomAccount(t *testing.T) *common.Account {
	account, err := common.NewRandomAccount()
	require.NoError(t, err)

	return account
}

func SetupRandomSubsidizer(t *testing.T, data code_data.Provider) *common.Account {
	account := NewRandomAccount(t)
	require.NoError(t, common.InjectTestSubsidizer(context.Background(), data, account))
	return account
}
