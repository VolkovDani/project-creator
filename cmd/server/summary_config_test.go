package main

import "testing"

func TestSummaryRequiresExplicitLocalPinnedConfiguration(t *testing.T) {
	for _, cfg := range []map[string]string{{"PROJECT_CREATOR_SUMMARY_MODEL": "qwen2.5:7b"}, {"PROJECT_CREATOR_SUMMARY_MODEL": "qwen2.5:7b", "PROJECT_CREATOR_SUMMARY_DIGEST": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "PROJECT_CREATOR_SUMMARY_URL": "http://example.com"}} {
		if _, e := configuredSummarizer(func(k string) string { return cfg[k] }); e == nil {
			t.Fatal("accepted partial or remote summary config")
		}
	}
	g, e := configuredSummarizer(func(string) string { return "" })
	if e != nil || g != nil {
		t.Fatal("unconfigured summaries must not call models")
	}
}
