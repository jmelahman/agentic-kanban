package jsonc

import (
	"encoding/json"
	"testing"
)

func TestStripComments(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"no comments", `{"a":1}`, `{"a":1}`},
		{"line comment", "{\n// comment\n\"a\":1\n}", "{\n\n\"a\":1\n}"},
		{"trailing line comment", `{"a":1} // tail`, `{"a":1} `},
		{"block comment", `{"a":1,/* b */"c":2}`, `{"a":1,"c":2}`},
		{"multiline block", "{/*\nx\ny\n*/\"a\":1}", `{"a":1}`},
		// Comment markers inside string values must survive untouched.
		{"slashes in string", `{"url":"http://x/y"}`, `{"url":"http://x/y"}`},
		{"block marker in string", `{"s":"a/* not a comment */b"}`, `{"s":"a/* not a comment */b"}`},
		{"escaped quote in string", `{"s":"he said \"// hi\""}`, `{"s":"he said \"// hi\""}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := string(StripComments([]byte(c.in))); got != c.want {
				t.Fatalf("StripComments(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestStripCommentsDecodes confirms the stripped output is valid JSON the
// stdlib decoder accepts, which is the whole point of the helper.
func TestStripCommentsDecodes(t *testing.T) {
	in := []byte("{\n  // name of the thing\n  \"name\": \"x\", /* inline */ \"n\": 2\n}")
	var out struct {
		Name string `json:"name"`
		N    int    `json:"n"`
	}
	if err := json.Unmarshal(StripComments(in), &out); err != nil {
		t.Fatalf("Unmarshal after strip: %v", err)
	}
	if out.Name != "x" || out.N != 2 {
		t.Fatalf("decoded %+v, want {Name:x N:2}", out)
	}
}
