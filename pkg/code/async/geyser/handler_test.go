package async_geyser

import (
	"testing"

	code_data "github.com/code-payments/code-server/pkg/code/data"
)

// todo: implement me
func TestTokenProgramAccountHandler(t *testing.T) {

}

// todo: implement me
func TestTimelockV1ProgramAccountHandler(t *testing.T) {

}

type testEnv struct {
	data     code_data.Provider
	handlers map[string]ProgramAccountUpdateHandler
}

func setup(t *testing.T) *testEnv {
	data := code_data.NewTestDataProvider()
	return &testEnv{
		data:     data,
		handlers: initializeProgramAccountUpdateHandlers(&conf{}, data, nil),
	}
}
