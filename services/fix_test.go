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

func TestPickFixCustomer(t *testing.T) {
	smith1 := fixCandidate{ID: "rec1", First: "Jo", Last: "Smith", Active: true}
	smith2 := fixCandidate{ID: "rec2", First: "Jo", Last: "Smith", Active: false}
	jones := fixCandidate{ID: "rec3", First: "Al", Last: "Jones", Active: true}
	cases := []struct {
		name  string
		cands []fixCandidate
		who   string
		want  string
	}{
		{"single", []fixCandidate{smith1}, "Whoever", "rec1"},
		{"none", nil, "Jo Smith", ""},
		{"by last name", []fixCandidate{smith1, jones}, "Al Jones", "rec3"},
		{"same name, one active", []fixCandidate{smith1, smith2}, "Jo Smith", "rec1"},
		{"ambiguous", []fixCandidate{smith1, jones}, "Pat Lee", "rec1x"[:0]},
		{"both active same name", []fixCandidate{smith1, {ID: "rec4", Last: "Smith", Active: true}}, "Jo Smith", ""},
	}
	for _, c := range cases {
		if got := pickFixCustomer(c.cands, c.who); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestAttachmentsCopied(t *testing.T) {
	mine := []interface{}{map[string]interface{}{"url": photoBaseURL + "/fix-a.jpg"}}
	theirs := []interface{}{map[string]interface{}{"url": "https://v5.airtableusercontent.com/x"}}
	if attachmentsCopied(mine) || attachmentsCopied(nil) || !attachmentsCopied(theirs) {
		t.Error("attachmentsCopied wrong")
	}
}
