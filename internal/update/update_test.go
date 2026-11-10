package update

import "testing"

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"v0.2.0", "v0.1.9", true},
		{"v1.0.0", "v0.99.99", true},
		{"v0.1.0", "v0.1.0", false},
		{"v0.1.0", "v0.2.0", false},
		{"v0.1.1", "dev", true},
		{"dev", "v0.1.0", false},
		{"v0.2.0-rc1", "v0.1.0", true},
		{"garbage", "v0.1.0", false},
	}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q,%q)=%v want %v", c.a, c.b, got, c.want)
		}
	}
}
