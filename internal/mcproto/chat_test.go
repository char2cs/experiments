package mcproto

import (
	"encoding/json"
	"testing"
)

func TestChat_Unmarshal(t *testing.T) {
	testCases := []struct {
		name string
		in   string
		want string
	}{
		{"plain string", `"Hello"`, "Hello"},
		{"object", `{"text":"Hello"}`, "Hello"},
		{"object with extra", `{"text":"A ","extra":[{"text":"B"},"C"]}`, "A BC"},
		{"empty string", `""`, ""},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var c Chat
			if err := json.Unmarshal([]byte(tc.in), &c); err != nil {
				t.Fatal(err)
			}
			if c.Text != tc.want {
				t.Fatalf("got %q want %q", c.Text, tc.want)
			}
		})
	}
}

func TestChat_Unmarshal_Invalid(t *testing.T) {
	var c Chat
	if err := json.Unmarshal([]byte(`[1,2]`), &c); err == nil {
		t.Fatal("array is not a component")
	}
}

func TestStatus_DecodesARealServerResponse(t *testing.T) {
	raw := `{"description":"Hello","players":{"max":20,"online":0},"version":{"name":"1.21.11","protocol":774}}`
	var st Status
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		t.Fatal(err)
	}
	if st.Description.Text != "Hello" || st.Players.Max != 20 || st.Version.Name != "1.21.11" || st.Version.Protocol != 774 {
		t.Fatalf("%+v", st)
	}
}
