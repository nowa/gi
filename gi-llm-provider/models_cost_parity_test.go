package gillmprovider

import (
	"reflect"
	"testing"
)

func TestCalculateCostMatchesPiIEEE754Evaluation(t *testing.T) {
	tests := []struct {
		name  string
		model Model
		usage Usage
		want  UsageCost
	}{
		{
			name: "tiered total preserves Pi operation order",
			model: Model{Cost: ModelCost{
				Input:      0.5,
				Output:     0.075,
				CacheRead:  18,
				CacheWrite: 3,
				Tiers: []ModelCostTier{
					{InputTokensAbove: 0, Input: 0.1875, Output: 4, CacheRead: 0, CacheWrite: 2.5},
					{InputTokensAbove: 888533, Input: 2, Output: 0.2, CacheRead: 1, CacheWrite: 12},
					{InputTokensAbove: 888532, Input: 0.075, Output: 8, CacheRead: 25, CacheWrite: 5},
				},
			}},
			usage: Usage{
				Input:        128710,
				Output:       51600,
				CacheRead:    383608,
				CacheWrite:   376215,
				CacheWrite1h: 237396,
			},
			want: UsageCost{
				Input:      0.00965325,
				Output:     0.4128,
				CacheRead:  9.590200000000001,
				CacheWrite: 0.7297044,
				Total:      10.74235765,
			},
		},
		{
			name: "one hour cache write preserves Pi operation order",
			model: Model{Cost: ModelCost{
				Input:      0.01,
				Output:     4,
				CacheRead:  8,
				CacheWrite: 7.5,
				Tiers: []ModelCostTier{{
					InputTokensAbove: 781887,
					Input:            0.075,
					Output:           0.6,
					CacheRead:        0.01,
					CacheWrite:       0.075,
				}},
			}},
			usage: Usage{
				Input:        385009,
				Output:       89989,
				CacheRead:    225600,
				CacheWrite:   171279,
				CacheWrite1h: 57637,
			},
			want: UsageCost{
				Input:      0.028875675,
				Output:     0.0539934,
				CacheRead:  0.002256,
				CacheWrite: 0.0171687,
				Total:      0.10229377499999999,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := CalculateCost(test.model, test.usage)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("cost = %#v, want %#v", got, test.want)
			}
		})
	}
}
