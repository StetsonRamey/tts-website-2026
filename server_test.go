package main

import "testing"

func TestAppendAttribution(t *testing.T) {
	got := appendAttribution("Needs roofline lighting", `{"landing_page":"/free-estimate/","utm_source":"google","utm_campaign":"holiday_lighting_2026","gclid":"abc123","ignored":"nope"}`)
	want := "Needs roofline lighting\n\nLead attribution:\nLanding page: /free-estimate/\nUTM source: google\nUTM campaign: holiday_lighting_2026\nGoogle click ID: abc123"
	if got != want {
		t.Fatalf("appendAttribution() = %q, want %q", got, want)
	}
}

func TestAppendAttributionIgnoresInvalidData(t *testing.T) {
	if got := appendAttribution("Customer note", "not json"); got != "Customer note" {
		t.Fatalf("appendAttribution() = %q, want original message", got)
	}
}
