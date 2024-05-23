package memory

import (
	"testing"

	"github.com/code-payments/code-server/pkg/cluster"
	clustertests "github.com/code-payments/code-server/pkg/cluster/tests"
)

func TestMemory(t *testing.T) {
	clustertests.RunClusterTests(t, func() (cluster.Cluster, error) {
		return NewCluster(), nil
	})
}
