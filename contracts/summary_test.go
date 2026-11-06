package contracts

import (
	"encoding/json"
	"os"
	"testing"
)

func TestSummarySharedFixtures(t *testing.T) {
	raw, err := os.ReadFile("summary.fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name, Text string
		Valid      bool
	}
	if err = json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			if got := ValidSummary(c.Text); got != c.Valid {
				t.Fatalf("valid=%v want=%v", got, c.Valid)
			}
		})
	}
	if ValidSummary(string([]byte{0xff})) {
		t.Fatal("invalid UTF8 accepted")
	}
}
