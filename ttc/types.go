// Package ttc is a Go port of the Tbilisi Transport Company (TTC) API wrapper.
// It is a complete copy of the TypeScript ttc-api library, providing real-time
// access to public transport data in Tbilisi, Georgia.
package ttc

// LatLng represents a [latitude, longitude] coordinate pair.
type LatLng = []float64

// Locale is the response language: "ka" (Georgian) or "en" (English).
type Locale = string

// BusMode is the transport mode for a bus route.
type BusMode = string

// VehicleMode describes the kind of vehicle serving a stop.
type VehicleMode = string

// BusColor is one of the known route colors.
type BusColor = string

const (
	LocaleKa Locale = "ka"
	LocaleEn Locale = "en"

	VehicleModeBus     VehicleMode = "BUS"
	VehicleModeGondola VehicleMode = "GONDOLA"
	VehicleModeSubway  VehicleMode = "SUBWAY"
)

// BusStop is a single stop in the transport network.
type BusStop struct {
	ID          string      `json:"id"`
	Code        *string     `json:"code"`
	Name        string      `json:"name"`
	Lat         float64     `json:"lat"`
	Lon         float64     `json:"lon"`
	VehicleMode VehicleMode `json:"vehicleMode"`
}

// BusArrival is an arrival prediction for a route at a stop.
type BusArrival struct {
	ShortName               string `json:"shortName"`
	Color                   string `json:"color"`
	Headsign                string `json:"headsign"`
	Realtime                bool   `json:"realtime"`
	RealtimeArrivalMinutes  int    `json:"realtimeArrivalMinutes"`
	ScheduledArrivalMinutes int    `json:"scheduledArrivalMinutes"`
}

// Bus is a transport route.
type Bus struct {
	ID        string   `json:"id"`
	ShortName string   `json:"shortName"`
	LongName  string   `json:"longName"`
	Color     BusColor `json:"color"`
	Mode      BusMode  `json:"mode"`
}

// BusRouteFull is a fully detailed route description.
type BusRouteFull struct {
	ID        string `json:"id"`
	ShortName string `json:"shortName"`
	LongName  string `json:"longName"`
	Color     string `json:"color"`
	Mode      string `json:"mode"`
	Circular  bool   `json:"circular"`
	LongNames struct {
		ForwardLongName  string `json:"forwardLongName"`
		BackwardLongName string `json:"backwardLongName"`
	} `json:"longNames"`
}

// BusLocation is the real-time position of a bus on a route.
type BusLocation struct {
	Lat        float64 `json:"lat"`
	Lon        float64 `json:"lon"`
	Heading    float64 `json:"heading"`
	NextStopID *string `json:"nextStopId,omitempty"`
}

// BusPlan is the result of a journey planning request.
type BusPlan struct {
	From        From        `json:"from"`
	To          From        `json:"to"`
	Itineraries []Itinerary `json:"itineraries"`
}

// From is a named coordinate (origin/destination of a leg or plan).
type From struct {
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
	Name string  `json:"name"`
}

// Itinerary is a single suggested journey.
type Itinerary struct {
	StartTime    int64   `json:"startTime"`
	EndTime      int64   `json:"endTime"`
	Duration     float64 `json:"duration"`
	WalkTime     float64 `json:"walkTime"`
	WalkDistance float64 `json:"walkDistance"`
	Legs         []Leg   `json:"legs"`
}

// Leg is one segment of an itinerary.
type Leg struct {
	From              From               `json:"from"`
	To                From               `json:"to"`
	StartTime         int64              `json:"startTime"`
	EndTime           int64              `json:"endTime"`
	Duration          float64            `json:"duration"`
	LegPolyline       LegPolyline        `json:"legPolyline"`
	Mode              Mode               `json:"mode"`
	Steps             []Step             `json:"steps"`
	IntermediateStops []IntermediateStop `json:"intermediateStops"`
	Route             *Bus               `json:"route"`
	RealTime          bool               `json:"realTime"`
	ArrivalDelay      float64            `json:"arrivalDelay"`
	Distance          float64            `json:"distance"`
}

// IntermediateStop is a stop passed through during a leg.
type IntermediateStop struct {
	ID          string  `json:"id"`
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	VehicleMode Mode    `json:"vehicleMode"`
}

// Mode is the travel mode of a leg or stop.
type Mode = string

const (
	ModeBus  Mode = "BUS"
	ModeWalk Mode = "WALK"
)

// LegPolyline is the encoded geometry of a leg.
type LegPolyline struct {
	EncodedValue string  `json:"encodedValue"`
	Color        *string `json:"color"`
}

// Step is a turn-by-turn walking instruction.
type Step struct {
	RelativeDirection string  `json:"relativeDirection"`
	Distance          float64 `json:"distance"`
	StreetName        string  `json:"streetName"`
	Lat               float64 `json:"lat"`
	Lon               float64 `json:"lon"`
}

// Polyline is the encoded geometry returned by the polyline endpoint.
type Polyline struct {
	EncodedValue string `json:"encodedValue"`
}
