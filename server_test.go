package main

import "testing"

func TestWhichForm(t *testing.T) {
	if got := whichForm("/free-estimate/ ads lander"); got != "/free-estimate/ ads lander" {
		t.Fatalf("whichForm() = %q, want paid landing-page value", got)
	}
	if got := whichForm("untrusted value"); got != "Main Contact" {
		t.Fatalf("whichForm() = %q, want default form value", got)
	}
}

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
