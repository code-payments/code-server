package memory

import (
	"net/http"
	"testing"

	_ "net/http/pprof"

	"github.com/code-payments/code-server/pkg/cluster"
	clustertests "github.com/code-payments/code-server/pkg/cluster/tests"
)

func TestMemory(t *testing.T) {
	go func() {
		http.ListenAndServe(":6060", nil)
	}()

	clustertests.RunClusterTests(t, func() (cluster.Cluster, error) {
		return NewCluster(), nil
	})
}
