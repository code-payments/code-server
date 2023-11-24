package treasury

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRecordRootUtilities(t *testing.T) {
	record := &Record{
		HistoryListSize: 5,
		HistoryList: []string{
			"root1",
			"root2",
			"root3",
			"root4",
			"root5",
		},
		CurrentIndex: 0,
	}

	assert.Equal(t, "root1", record.GetMostRecentRoot())
	assert.Equal(t, "root5", record.GetPreviousMostRecentRoot())

	record.CurrentIndex = 2
	assert.Equal(t, "root3", record.GetMostRecentRoot())
	assert.Equal(t, "root2", record.GetPreviousMostRecentRoot())

	record.CurrentIndex = 1

	reorderedFromMostRecentRoot := []string{
		"root2",
		"root1",
		"root5",
		"root4",
		"root3",
	}
	for i, recentRoot := range reorderedFromMostRecentRoot {
		containsRecentRoot, deltaFromMostRecent := record.ContainsRecentRoot(recentRoot)
		assert.True(t, containsRecentRoot)
		assert.Equal(t, i, deltaFromMostRecent)
	}

	containsRecentRoot, _ := record.ContainsRecentRoot("root6")
	assert.False(t, containsRecentRoot)
}
