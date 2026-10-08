package services

import "testing"

func TestImageExt(t *testing.T) {
	cases := map[string]string{
		"\xff\xd8\xff\xe0abc":              ".jpg",
		"\x89PNG\r\n\x1a\nxxxx":            ".png",
		"RIFF\x00\x00\x00\x00WEBPVP8 ":     ".webp",
		"\x00\x00\x00\x18ftypheic\x00\x00": ".heic",
		"<script>alert(1)</script>":        "",
		"GIF89a":                           "",
	}
	for in, want := range cases {
		if got := imageExt([]byte(in)); got != want {
			t.Errorf("imageExt(%q) = %q, want %q", in, got, want)
		}
	}
}
