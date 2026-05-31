package models

import (
	"sort"
	"testing"
	"time"

	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/require"
)

func TestYear(t *testing.T) {
	tests := []struct {
		year int
	}{
		{0},
		{1},
		{660},
		{969},
		{1457},
		{2000},
		{2024},
	}
	for _, tt := range tests {
		t.Run("year_"+strconvI(tt.year), func(t *testing.T) {
			got := Year(tt.year)
			require.Equal(t, time.Date(tt.year, 1, 1, 0, 0, 0, 0, time.UTC), got)
		})
	}
}

func TestCityID(t *testing.T) {
	c := City{State: "São Paulo", Name: "São Paulo"}
	require.Equal(t, "São Paulo_São Paulo", CityID(c))

	c2 := City{State: "New-York", Name: "Albany"}
	require.Equal(t, "New-York_Albany", CityID(c2))
}

func TestSortedCityIDs(t *testing.T) {
	// Build expected list
	expected := make([]string, len(Cities))
	for i, city := range Cities {
		expected[i] = dal.EscapeID(CityID(city))
	}
	sort.Strings(expected)

	require.Equal(t, len(expected), len(SortedCityIDs))
	require.Equal(t, expected, SortedCityIDs)

	// Check that all are unique
	seen := make(map[string]struct{}, len(SortedCityIDs))
	for _, id := range SortedCityIDs {
		if _, ok := seen[id]; ok {
			t.Fatalf("duplicate id in SortedCityIDs: %s", id)
		}
		seen[id] = struct{}{}
	}
}

func TestCityIDsSortedByPopulation(t *testing.T) {
	// Sort a copy of Cities by population ascending to build expected list
	byPop := make([]City, len(Cities))
	copy(byPop, Cities)
	sort.Slice(byPop, func(i, j int) bool { return byPop[i].Population < byPop[j].Population })

	expected := make([]string, len(byPop))
	for i, c := range byPop {
		expected[i] = CityID(c)
	}

	require.Equal(t, len(expected), len(CityIDsSortedByPopulation))
	require.Equal(t, expected, CityIDsSortedByPopulation)
}

// strconvI is a tiny helper to avoid importing strconv for one use.
func strconvI(i int) string { // covered by TestYear
	// minimal, deterministic conversion
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	var digits [20]byte
	pos := len(digits)
	for i > 0 {
		pos--
		digits[pos] = byte('0' + (i % 10))
		i /= 10
	}
	if neg {
		pos--
		digits[pos] = '-'
	}
	return string(digits[pos:])
}
