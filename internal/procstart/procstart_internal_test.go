package procstart

import "testing"

func TestLinuxStartTime(t *testing.T) {
	for _, test := range []struct {
		name, stat, want string
	}{
		{
			name: "plain name",
			stat: "4242 (sleep) S 1 4242 4242 0 -1 4194304 97 0 0 0 0 0 0 0 20 0 1 0 8675309 2338816 128 18446744073709551615 1 1 0 0 0 0 0 0 0 0 0 0 17 3 0 0 0 0 0",
			want: "8675309",
		},
		{
			name: "name with spaces and parentheses",
			stat: "77 (a) b (c) R 1 77 77 0 -1 4194304 97 0 0 0 0 0 0 0 20 0 1 0 123 2338816 128 18446744073709551615 1 1 0 0 0 0 0 0 0 0 0 0 17 3 0 0 0 0 0\n",
			want: "123",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := linuxStartTime(test.stat)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("start time = %q, want %q", got, test.want)
			}
		})
	}
	for _, stat := range []string{"", "4242 sleep S 1", "4242 (sleep) S 1 4242"} {
		if got, err := linuxStartTime(stat); err == nil {
			t.Errorf("linuxStartTime(%q) = %q, want an error", stat, got)
		}
	}
}

func TestDarwinStartTime(t *testing.T) {
	if got := darwinStartTime(1790080486, 18109); got != "1790080486.018109" {
		t.Fatalf("start time = %q", got)
	}
}
