package tui

// VisibleRange returns the start and end indices of items to display,
// ensuring the selected item is always visible within maxLines.
func VisibleRange(selected, count, maxLines, linesPerItem int) (int, int) {
	if linesPerItem < 1 {
		linesPerItem = 1
	}
	visible := maxLines / linesPerItem
	if visible <= 0 {
		visible = 1
	}
	if visible >= count {
		return 0, count
	}
	start := selected - visible/2
	if start < 0 {
		start = 0
	}
	end := start + visible
	if end > count {
		end = count
		start = end - visible
	}
	return start, end
}

// ClampCursor ensures cursor is within [0, count-1]. Returns 0 if count is 0.
func ClampCursor(cursor, count int) int {
	if count == 0 {
		return 0
	}
	if cursor < 0 {
		return 0
	}
	if cursor >= count {
		return count - 1
	}
	return cursor
}

// TabPanel cycles focus through the given flow of panels.
// dir should be 1 (forward) or -1 (backward).
func TabPanel(focus PanelID, flow []PanelID, dir int) PanelID {
	current := 0
	for i, p := range flow {
		if p == focus {
			current = i
			break
		}
	}
	next := (current + dir + len(flow)) % len(flow)
	return flow[next]
}
