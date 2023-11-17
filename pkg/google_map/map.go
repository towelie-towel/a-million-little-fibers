package google_map

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"googlemaps.github.io/maps"
)

type Location struct {
	Lat  float64
	Lon  float64
	Head int16
}

func (loc Location) String() string {
	return fmt.Sprintf("%f,%f,%d", loc.Lat, loc.Lon, loc.Head)
}

func ParseLocation(location string) (Location, error) {
	parts := strings.Split(location, ",")
	if len(parts) < 2 {
		return Location{}, fmt.Errorf("invalid location format")
	}
	latitude, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return Location{}, fmt.Errorf("invalid latitude: %v", err)
	}
	longitude, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return Location{}, fmt.Errorf("invalid longitude: %v", err)
	}
	if len(parts) == 2 {
		return Location{Lat: latitude, Lon: longitude, Head: 0}, nil
	}
	heading, err := strconv.ParseInt(strings.TrimSpace(parts[2]), 10, 16)
	if err != nil {
		return Location{}, fmt.Errorf("invalid heading: %v", err)
	}
	if len(parts) == 3 {
		return Location{Lat: latitude, Lon: longitude, Head: int16(heading)}, nil
	}
	return Location{}, fmt.Errorf("invalid location format")
}

func NewMapClient() (*maps.Client, error) {
	return maps.NewClient(maps.WithAPIKey(os.Getenv("GOOGLE_MAPS_API_KEY")))
}

// traditional function
func GetRoute(mapClient *maps.Client, from, to string) []maps.Route {
	request := &maps.DirectionsRequest{
		Origin:      from,
		Destination: to,
	}
	route, _, err := mapClient.Directions(context.Background(), request)
	if err != nil {
		log.Fatalf("fatal direction error: %s", err)
	}
	return route
}
