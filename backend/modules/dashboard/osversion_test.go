package dashboard

import "testing"

func TestParseOSVersion(t *testing.T) {
	cases := []struct{ in, fb, want string }{
		{"NAME=\"Ubuntu\"\nVERSION=\"24.04.2 LTS (Noble Numbat)\"\nVERSION_ID=\"24.04\"\n", "24.04", "24.04.2"},
		{"VERSION=\"26.04 LTS (Resolute)\"\n", "26.04", "26.04"},
		{"VERSION=\"12 (bookworm)\"\n", "12", "12"},
		{"VERSION=\"22.04.5 LTS\"\n", "20.04", "20.04"},
		{"", "9", "9"},
	}
	for _, c := range cases {
		if got := parseOSVersion(c.in, c.fb); got != c.want {
			t.Errorf("parseOSVersion(%q,%q) = %q, want %q", c.in, c.fb, got, c.want)
		}
	}
}
