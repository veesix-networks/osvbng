package conf

import "testing"

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{
			pattern: "subscriber.auth.local.user.<*>.enabled",
			path:    "subscriber.auth.local.user.alice.enabled",
			want:    true,
		},
		{
			pattern: "subscriber.auth.local.user.<*>.password",
			path:    "subscriber.auth.local.user.bob.password",
			want:    true,
		},
		{
			pattern: "interfaces.<*>.enabled",
			path:    "interfaces.eth0.enabled",
			want:    true,
		},
		{
			pattern: "interfaces.<*>.enabled",
			path:    "interfaces.eth0.description",
			want:    false,
		},
		{
			pattern: "subscriber.<*>.<*>.user",
			path:    "subscriber.auth.local.user",
			want:    true,
		},
		{
			pattern: "protocols.ospf.areas.<*>.interfaces.<*>",
			path:    "protocols.ospf.areas.0.interfaces.eth0",
			want:    true,
		},
		{
			pattern: "vrfs.<*>",
			path:    "vrfs.default",
			want:    true,
		},
		{
			pattern: "vrfs.<*>",
			path:    "vrfs.default.extra",
			want:    false,
		},
	}

	for _, tt := range tests {
		got := matchPattern(tt.pattern, tt.path)
		if got != tt.want {
			t.Errorf("matchPattern(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
		}
	}
}
