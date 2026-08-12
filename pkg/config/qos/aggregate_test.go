package qos

import "testing"

func TestParseSVLANs(t *testing.T) {
	tests := []struct {
		name    string
		svlans  []string
		want    []SVLANRange
		wantErr bool
	}{
		{name: "single tag", svlans: []string{"100"}, want: []SVLANRange{{100, 100}}},
		{name: "range", svlans: []string{"200-299"}, want: []SVLANRange{{200, 299}}},
		{
			name:   "disjoint list",
			svlans: []string{"100", "200-299"},
			want:   []SVLANRange{{100, 100}, {200, 299}},
		},
		// The list is how disjoint sets are expressed. Accepting commas too
		// would give one config two spellings that behave differently
		// elsewhere in the stack.
		{name: "commas rejected", svlans: []string{"100,200"}, wantErr: true},
		{name: "descending rejected", svlans: []string{"300-200"}, wantErr: true},
		{name: "above 4095 rejected", svlans: []string{"4096"}, wantErr: true},
		{name: "non-numeric rejected", svlans: []string{"abc"}, wantErr: true},
		{name: "empty rejected", svlans: []string{""}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := (&Aggregate{SVLANs: tt.svlans}).ParseSVLANs()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("range %d: got %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestAggregateLevel(t *testing.T) {
	if lvl := (&Aggregate{}).Level(); lvl != AggregateLevelPort {
		t.Fatalf("no svlans should be a port aggregate, got %d", lvl)
	}
	if lvl := (&Aggregate{SVLANs: []string{"100"}}).Level(); lvl != AggregateLevelSVLAN {
		t.Fatalf("svlans should be an S-VLAN aggregate, got %d", lvl)
	}
}

func TestValidateBounds(t *testing.T) {
	base := func() *Aggregate {
		return &Aggregate{Name: "a", Interface: "Te0/0/0", Rate: 1000}
	}

	if err := base().Validate(); err != nil {
		t.Fatalf("minimal config should validate: %v", err)
	}

	a := base()
	a.Interface = ""
	if err := a.Validate(); err == nil {
		t.Fatal("missing interface should not validate")
	}

	a = base()
	a.Rate = 0
	if err := a.Validate(); err == nil {
		t.Fatal("missing rate should not validate")
	}

	for _, w := range []uint32{WeightMax + 1, 9999} {
		a = base()
		a.Weight = w
		if err := a.Validate(); err == nil {
			t.Fatalf("weight %d should not validate", w)
		}
	}

	// Zero means "default", which is distinct from "out of range".
	a = base()
	a.Weight = 0
	if err := a.Validate(); err != nil {
		t.Fatalf("weight 0 means default and should validate: %v", err)
	}

	for _, b := range []uint32{BurstMsMin - 1, BurstMsMax + 1} {
		a = base()
		a.BurstMs = b
		if err := a.Validate(); err == nil {
			t.Fatalf("burst-ms %d should not validate", b)
		}
	}
}

func TestValidateAggregatesSet(t *testing.T) {
	t.Run("svlan needs a port aggregate", func(t *testing.T) {
		err := ValidateAggregates(map[string]*Aggregate{
			"cust": {Interface: "Te0/0/0", Rate: 500, SVLANs: []string{"100"}},
		})
		if err == nil {
			t.Fatal("an S-VLAN with no port aggregate should not validate")
		}
	})

	t.Run("svlan may not exceed its port", func(t *testing.T) {
		err := ValidateAggregates(map[string]*Aggregate{
			"port": {Interface: "Te0/0/0", Rate: 1000},
			"cust": {Interface: "Te0/0/0", Rate: 2000, SVLANs: []string{"100"}},
		})
		if err == nil {
			t.Fatal("an S-VLAN shaped above its port should not validate")
		}
	})

	t.Run("oversubscription across svlans is allowed", func(t *testing.T) {
		// The sum exceeding the port is the normal case and the reason the
		// tier exists; only a single child above the port is nonsense.
		err := ValidateAggregates(map[string]*Aggregate{
			"port": {Interface: "Te0/0/0", Rate: 1000},
			"a":    {Interface: "Te0/0/0", Rate: 800, SVLANs: []string{"100-199"}},
			"b":    {Interface: "Te0/0/0", Rate: 800, SVLANs: []string{"200-299"}},
		})
		if err != nil {
			t.Fatalf("oversubscription should validate: %v", err)
		}
	})

	t.Run("overlapping tags rejected", func(t *testing.T) {
		err := ValidateAggregates(map[string]*Aggregate{
			"port": {Interface: "Te0/0/0", Rate: 1000},
			"a":    {Interface: "Te0/0/0", Rate: 500, SVLANs: []string{"100-200"}},
			"b":    {Interface: "Te0/0/0", Rate: 500, SVLANs: []string{"200-300"}},
		})
		if err == nil {
			t.Fatal("overlapping S-VLAN sets on one port should not validate")
		}
	})

	t.Run("same tags on different ports are fine", func(t *testing.T) {
		err := ValidateAggregates(map[string]*Aggregate{
			"p1": {Interface: "Te0/0/0", Rate: 1000},
			"p2": {Interface: "Te0/0/1", Rate: 1000},
			"a":  {Interface: "Te0/0/0", Rate: 500, SVLANs: []string{"100"}},
			"b":  {Interface: "Te0/0/1", Rate: 500, SVLANs: []string{"100"}},
		})
		if err != nil {
			t.Fatalf("the map is per port, so this should validate: %v", err)
		}
	})

	t.Run("two port aggregates on one interface rejected", func(t *testing.T) {
		err := ValidateAggregates(map[string]*Aggregate{
			"p1": {Interface: "Te0/0/0", Rate: 1000},
			"p2": {Interface: "Te0/0/0", Rate: 2000},
		})
		if err == nil {
			t.Fatal("one interface cannot have two port aggregates")
		}
	})
}
