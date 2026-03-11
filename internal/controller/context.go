package controller

import "github.com/ignaciotcrespo/gitshelf/internal/types"

// IsChangelistContext returns true if the current pivot is changelists.
func IsChangelistContext(s State) bool {
	return s.Pivot == types.PanelChangelists
}
