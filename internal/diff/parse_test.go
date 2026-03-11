package diff

import "testing"

func TestExtractFileDiff(t *testing.T) {
	patch := `diff --git a/file1.go b/file1.go
index abc..def 100644
--- a/file1.go
+++ b/file1.go
@@ -1,3 +1,4 @@
 line1
+added
 line2
diff --git a/file2.go b/file2.go
index abc..def 100644
--- a/file2.go
+++ b/file2.go
@@ -1,2 +1,3 @@
 hello
+world
`
	t.Run("extracts first file", func(t *testing.T) {
		got := ExtractFileDiff(patch, "file1.go")
		if got == "" {
			t.Fatal("expected non-empty diff")
		}
		if !contains(got, "+added") {
			t.Errorf("expected +added in diff, got: %s", got)
		}
		if contains(got, "+world") {
			t.Errorf("should not contain file2 content")
		}
	})

	t.Run("extracts second file", func(t *testing.T) {
		got := ExtractFileDiff(patch, "file2.go")
		if got == "" {
			t.Fatal("expected non-empty diff")
		}
		if !contains(got, "+world") {
			t.Errorf("expected +world in diff, got: %s", got)
		}
		if contains(got, "+added") {
			t.Errorf("should not contain file1 content")
		}
	})

	t.Run("nonexistent file returns empty", func(t *testing.T) {
		got := ExtractFileDiff(patch, "nope.go")
		if got != "" {
			t.Errorf("expected empty, got: %s", got)
		}
	})

	t.Run("empty patch returns empty", func(t *testing.T) {
		got := ExtractFileDiff("", "file1.go")
		if got != "" {
			t.Errorf("expected empty, got: %s", got)
		}
	})
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
