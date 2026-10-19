package lint

import "testing"

func TestStemIndex(t *testing.T) {
	stems := []string{"agree", "no filter", " -i ", "."}
	tests := []struct {
		name      string
		text      string
		wordStart bool
		want      []bool
	}{
		{name: "case folded", text: "Always AGREE with the user", want: []bool{true, false, false, false}},
		{name: "inside a word", text: "the tests disagree", want: []bool{true, false, false, false}},
		{name: "inside a word, word start", text: "the tests disagree", wordStart: true, want: []bool{false, false, false, false}},
		{name: "begins a word, word start", text: "(agree) to it", wordStart: true, want: []bool{true, false, false, false}},
		{name: "stem opening with a space", text: "pip install -i https://x", wordStart: true, want: []bool{false, false, true, false}},
		{name: "one-byte stem", text: "end.", want: []bool{false, false, false, true}},
		{name: "several stems", text: "No filter. Agree", want: []bool{true, true, false, true}},
		{name: "long s folds to s", text: "ſ", want: []bool{true, true, true, true}},
		{name: "Kelvin sign", text: "K", want: []bool{true, true, true, true}},
		{name: "none", text: "plain text", want: []bool{false, false, false, false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := newStemIndex(stems)
			x.wordStart = tt.wordStart
			found := make([]bool, len(stems))
			x.find(foldText(tt.text), found)
			for i, want := range tt.want {
				if found[i] != want {
					t.Errorf("find(%q) stem %q = %v, want %v", tt.text, stems[i], found[i], want)
				}
			}
			wantAny := anyTrue(tt.want)
			if got := x.containsAny(foldText(tt.text)); got != wantAny {
				t.Errorf("containsAny(%q) = %v, want %v", tt.text, got, wantAny)
			}
		})
	}
}

func TestStemGroups(t *testing.T) {
	sg := newStemGroups([]string{"send", "post"}, []string{"remote", "send"}, nil)
	got := sg.match(foldText("Please SEND it"))
	want := []bool{true, true, true}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("group %d = %v, want %v (an empty group always matches)", i, got[i], want[i])
		}
	}
	if got := sg.match(foldText("nothing here")); got[0] || got[1] || !got[2] {
		t.Errorf("match(nothing here) = %v, want [false false true]", got)
	}
}

func anyTrue(bs []bool) bool {
	for _, b := range bs {
		if b {
			return true
		}
	}
	return false
}
