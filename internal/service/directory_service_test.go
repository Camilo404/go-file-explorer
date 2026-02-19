package service

import (
	"testing"
	time "time"

	"github.com/stretchr/testify/require"

	"go-file-explorer/internal/model"
)

func TestSortItems(t *testing.T) {
	t.Parallel()

	t.Run("sorts by size descending and keeps stable order for ties", func(t *testing.T) {
		items := []model.FileItem{
			{Name: "alpha.txt", Size: 200},
			{Name: "beta.txt", Size: 100},
			{Name: "gamma.txt", Size: 100},
		}

		sortItems(items, "size", "desc")

		require.Equal(t, "alpha.txt", items[0].Name)
		require.Equal(t, "beta.txt", items[1].Name)
		require.Equal(t, "gamma.txt", items[2].Name)
	})

	t.Run("sorts by modified_at ascending", func(t *testing.T) {
		base := time.Now().UTC()
		items := []model.FileItem{
			{Name: "c.txt", ModifiedAt: base.Add(2 * time.Hour)},
			{Name: "a.txt", ModifiedAt: base.Add(-2 * time.Hour)},
			{Name: "b.txt", ModifiedAt: base},
		}

		sortItems(items, "modified_at", "asc")

		require.Equal(t, "a.txt", items[0].Name)
		require.Equal(t, "b.txt", items[1].Name)
		require.Equal(t, "c.txt", items[2].Name)
	})
}
