package diff

import "strings"

// ExtractFileDiff extracts the diff section for a single file from a unified patch.
func ExtractFileDiff(patch, file string) string {
	lines := strings.Split(patch, "\n")
	var result []string
	capturing := false
	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git ") {
			if capturing {
				break
			}
			if strings.Contains(line, "b/"+file) {
				capturing = true
			}
		}
		if capturing {
			result = append(result, line)
		}
	}
	return strings.Join(result, "\n")
}
