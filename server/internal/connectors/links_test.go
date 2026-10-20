package connectors

import (
	"reflect"
	"testing"
)

func TestExtractReferences(t *testing.T) {
	cases := []struct {
		input string
		want  []Reference
	}{
		{
			input: "Hey team, check out ENG-123 regarding the auth timeout.",
			want: []Reference{
				{Kind: "ticket", Key: "eng-123"},
			},
		},
		{
			input: "Fixed in #42 and merged into main.",
			want: []Reference{
				{Kind: "github", Key: "#42"},
			},
		},
		{
			input: "Discussion on acme/backend#312 links to https://linear.app/acme/issue/CORE-99 and Sentry #9412.",
			want: []Reference{
				{Kind: "ticket", Key: "core-99"},
				{Kind: "sentry", Key: "9412"},
				{Kind: "github", Key: "acme/backend#312"},
			},
		},
		{
			input: "See https://github.com/memrohq/memro/pull/6 for details on issue ENG-45.",
			want: []Reference{
				{Kind: "ticket", Key: "eng-45"},
				{Kind: "github", Key: "memrohq/memro#6"},
			},
		},
		{
			input: "No tickets here, just general discussion about lunch.",
			want:  nil,
		},
	}

	for _, c := range cases {
		got := ExtractReferences(c.input)
		if len(got) == 0 && len(c.want) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("ExtractReferences(%q) = %+v, want %+v", c.input, got, c.want)
		}
	}
}

func TestReferenceTags(t *testing.T) {
	text := "Resolves ENG-123 and #42"
	tags := ReferenceTags(text)
	expected := []string{"ticket:eng-123", "github:#42"}
	if !reflect.DeepEqual(tags, expected) {
		t.Fatalf("ReferenceTags(%q) = %v, want %v", text, tags, expected)
	}
}
