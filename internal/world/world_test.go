package world

import (
	"strings"
	"testing"
)

func TestParseValidation(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr string
	}{
		{"ok", `{"start":"a","regions":[{"id":"a","foes":["rat"],"exits":["b"]},{"id":"b","foes":["rat"],"exits":["a"]}]}`, ""},
		{"bad json", `{`, "decode world"},
		{"no regions", `{"start":"a","regions":[]}`, "no regions"},
		{"bad start", `{"start":"x","regions":[{"id":"a","foes":["rat"]}]}`, "start region"},
		{"dangling exit", `{"start":"a","regions":[{"id":"a","foes":["rat"],"exits":["nowhere"]}]}`, "unknown region"},
		{"duplicate", `{"start":"a","regions":[{"id":"a","foes":["rat"]},{"id":"a","foes":["rat"]}]}`, "duplicate"},
		{"no foes", `{"start":"a","regions":[{"id":"a"}]}`, "no foes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, err := Parse([]byte(tt.json))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				a, _ := w.Region("a")
				if !a.HasExit("b") || a.HasExit("c") {
					t.Fatal("HasExit is wrong")
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestShippedWorldIsValid(t *testing.T) {
	if _, err := Load("../../world.json"); err != nil {
		t.Fatal(err)
	}
}
