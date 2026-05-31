package models

import (
	"fmt"
	"sort"
	"time"

	"github.com/dal-go/dalgo/dal"
)

const CitiesCollection = "DalgoTest_Cities"

type City struct {
	Name          string
	State         string
	Country       string
	Population    int // in people
	AreaSqKm      int // in square kilometers
	IsCapital     bool
	HasAirport    bool
	Founded       time.Time
	LastUpdatedAt time.Time
}

func Year(year int) time.Time {
	return time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
}

var Cities = []City{
	{
		Name:          "Tokyo",
		State:         "Tokyo",
		Country:       "JP",
		Population:    37400068,
		AreaSqKm:      2187,
		IsCapital:     true,
		HasAirport:    true,
		Founded:       Year(1457),
		LastUpdatedAt: time.Now(),
	},
	{
		Name:          "Delhi",
		State:         "Delhi",
		Country:       "IN",
		Population:    30290936,
		AreaSqKm:      1484,
		IsCapital:     true,
		HasAirport:    true,
		Founded:       Year(1911),
		LastUpdatedAt: time.Now(),
	},
	{
		Name:          "Shanghai",
		State:         "Shanghai",
		Country:       "CN",
		Population:    27058480,
		AreaSqKm:      6340,
		IsCapital:     false,
		HasAirport:    true,
		Founded:       Year(1291),
		LastUpdatedAt: time.Now(),
	},
	{
		Name:          "São Paulo",
		State:         "São Paulo",
		Country:       "BR",
		Population:    22046000,
		AreaSqKm:      1521,
		IsCapital:     false,
		HasAirport:    true,
		Founded:       Year(1554),
		LastUpdatedAt: time.Now(),
	},
	{
		Name:          "Mumbai",
		State:         "Maharashtra",
		Country:       "IN",
		Population:    21344117,
		AreaSqKm:      603,
		IsCapital:     false,
		HasAirport:    true,
		Founded:       Year(1661),
		LastUpdatedAt: time.Now(),
	},
	{
		Name:          "Beijing",
		State:         "Beijing",
		Country:       "CN",
		Population:    21051600,
		AreaSqKm:      16410,
		IsCapital:     true,
		HasAirport:    true,
		Founded:       Year(1045),
		LastUpdatedAt: time.Now(),
	},
	{
		Name:          "Cairo",
		State:         "Cairo",
		Country:       "EG",
		Population:    20474100,
		AreaSqKm:      214,
		IsCapital:     true,
		HasAirport:    true,
		Founded:       Year(969),
		LastUpdatedAt: time.Now(),
	},
	{
		Name:          "Dhaka",
		State:         "Dhaka",
		Country:       "BD",
		Population:    20183500,
		AreaSqKm:      306,
		IsCapital:     true,
		HasAirport:    true,
		Founded:       Year(1608),
		LastUpdatedAt: time.Now(),
	},
	{
		Name:          "Karachi",
		State:         "Sindh",
		Country:       "PK",
		Population:    15741000,
		AreaSqKm:      3780,
		IsCapital:     false,
		HasAirport:    true,
		Founded:       Year(1729),
		LastUpdatedAt: time.Now(),
	},
	{
		Name:          "Istanbul",
		State:         "Istanbul",
		Country:       "TR",
		Population:    15029231,
		AreaSqKm:      5343,
		IsCapital:     false,
		HasAirport:    true,
		Founded:       Year(660),
		LastUpdatedAt: time.Now(),
	},
}

var SortedCityIDs []string

var CityIDsSortedByPopulation []string

func CityID(city City) string {
	return fmt.Sprintf("%s_%s", city.State, city.Name)
}

func init() {
	SortedCityIDs = make([]string, len(Cities))
	for i, city := range Cities {
		SortedCityIDs[i] = CityID(city)
		SortedCityIDs[i] = dal.EscapeID(CityID(city))
	}
	sort.Strings(SortedCityIDs)

	citiesSortedByPopulation := make([]City, len(Cities))
	copy(citiesSortedByPopulation, Cities)
	sort.Slice(citiesSortedByPopulation, func(i, j int) bool {
		return citiesSortedByPopulation[i].Population < citiesSortedByPopulation[j].Population
	})
	CityIDsSortedByPopulation = make([]string, len(citiesSortedByPopulation))
	for i, city := range citiesSortedByPopulation {
		CityIDsSortedByPopulation[i] = CityID(city)
	}
}
