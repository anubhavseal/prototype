package parkinglot

import "time"

type ParkingLot struct {
	levels []Level
}

func NewParkingLot() {

}
func (p *ParkingLot) GetStatus()  map[SpotType] int {}
func (p *ParkingLot) ReserveSpot() Ticket {}
func (p *ParkingLot) Exit()  {}

type Level struct {
	spots []Spot
}

func (l *Level) ReserveSpot() {}

type Spot struct {
	id        int
	available bool
	spotType  VehicleType
}

func (l *Spot) ReserveSpot() {}

type Ticket struct {
	id string
	vehiclenumber string
	entryTime time.Time
	exitTime time.Time
	spotId int
}

type VehicleType int

const (
	Random VehicleType = iota
	Car
	Bike
	Truck
)

type Vehicle struct {
	number string
	vehicleType VehicleType
}

type SpotType int

const (
	Random SpotType = iota
	Small
	Medium
	Large
)

type Vehicle struct {
	number string
	vehicleType VehicleType
}

var stringToVehicleType = map[string]VehicleType {
	"Car": Car,
	"Bike": Bike,
	"Truck": Truck,
}

var vehicleTypeToString = map[VehicleType]string  {
	Car:   "Car",
	Bike:  "Bike",
	Truck: "Truck",
}


func (v VehicleType) conversionFactor () {
	var a map[VehicleType]map[VehicleType]int 
	a = map[VehicleType]map[VehicleType]int{
		Bike: map[VehicleType]int{
			Car: 4
		}
	}
}