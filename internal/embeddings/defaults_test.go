package embeddings

import "testing"

func TestDefaultThreshold(t *testing.T) {
	cases := []struct {
		provider string
		want     float32
	}{
		{ProviderOpenAI, 1.5},
		{ProviderLocal, 0.6},
		{"", 0.6},          // unrecognized → local default
		{"something", 0.6}, // unrecognized → local default
	}
	for _, c := range cases {
		if got := DefaultThreshold(c.provider); got != c.want {
			t.Errorf("DefaultThreshold(%q) = %v, want %v", c.provider, got, c.want)
		}
	}
}
