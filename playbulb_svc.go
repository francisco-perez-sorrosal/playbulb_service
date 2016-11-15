package main

import (
	"log"
	"net/http"

	"github.com/paypal/gatt"
	"strings"
	"flag"
	"os"
	"encoding/json"
	"encoding/hex"
	"github.com/gorilla/mux"
)

const (
	PLAYBULB_SERVICE_ID = "ff07"
	PLAYBULB_COLOR_CHARACTERISTIC = "fffc"
)

type Playbulb struct {
	Action string `json:"action"`
	R      string `json:"r"`
	G      string `json:"g"`
	B      string `json:"b"`
}

var COLOR_DEFAULT = []byte{0x10, 0x00, 0x00, 0xFF}
var COLOR_OFF = []byte{0x00, 0x00, 0x00, 0x00}
var COLOR_WHITE = []byte{0xFF, 0xFF, 0xFF, 0xFF}

var color = COLOR_OFF

//var DefaultClientOptions = []gatt.Option{
//	gatt.MacDeviceRole(gatt.CentralManager),
//}

var DefaultClientOptions = []gatt.Option{
	gatt.LnxMaxConnections(1),
	gatt.LnxDeviceID(-1, true),
}

var peripheralId string
var peripheral gatt.Peripheral

func handleDeviceRequest(w http.ResponseWriter, r *http.Request) {

	if r.Body == nil {
		http.Error(w, "Please send a request body", http.StatusBadRequest)
		return
	}
	var playbulb Playbulb
	err := json.NewDecoder(r.Body).Decode(&playbulb)
	if err != nil {
		color = COLOR_OFF
		http.Error(w, "Cannot decode string. ", http.StatusBadRequest)
	} else {
		switch playbulb.Action {
		case "off":
			color = COLOR_OFF
		case "on":
			color = COLOR_WHITE
		case "default":
			color = COLOR_DEFAULT
		case "custom":
			hexR, cerr := hex.DecodeString(playbulb.R)
			if cerr != nil {
				http.Error(w, "Cannot decode R(ed) param. ", http.StatusBadRequest)
			}
			hexG, cerr := hex.DecodeString(playbulb.G)
			if cerr != nil {
				http.Error(w, "Cannot decode G(een) param. ", http.StatusBadRequest)
			}
			hexB, cerr := hex.DecodeString(playbulb.B)
			if cerr != nil {
				http.Error(w, "Cannot decode B(lue) param. ", http.StatusBadRequest)
			}
			color = []byte{0x00, hexR[0], hexG[0], hexB[0]}
		default:
			color = COLOR_OFF
		}
	}

	peripheral.Device().CancelConnection(peripheral)
	peripheral.Device().Connect(peripheral)

}

func onPeriphDiscovered(p gatt.Peripheral, advert *gatt.Advertisement, rssi int) {

	if strings.ToUpper(p.ID()) == peripheralId {
		log.Printf("Target device found. ID %s, %s", p.ID(), p.Name())
		p.Device().StopScanning()
		p.Device().Connect(p)
		peripheral = p
	}

}

func onPeriphConnected(p gatt.Peripheral, err error) {

	log.Printf("Connected to %s", p.Name())

	if err := p.SetMTU(500); err != nil {
		log.Printf("Failed to set MTU, err: %s\n", err)
	}

	ss, err := p.DiscoverServices(nil)
	if err != nil {
		log.Printf("Failed to discover services, err: %s\n", err)
		return
	}
	for _, s := range ss {
		if s.UUID().String() == PLAYBULB_SERVICE_ID {
			cs, err := p.DiscoverCharacteristics(nil, s)
			if err != nil {
				log.Printf("Failed to discover characteristics, err: %s\n", err)
				break
			}
			for _, c := range cs {
				if c.UUID().String() == PLAYBULB_COLOR_CHARACTERISTIC {
					p.WriteCharacteristic(c, color, true)
				}
			}
		}
	}
}

func onPeriphDisconnected(p gatt.Peripheral, err error) {
	log.Printf("Disconnected")
}

func onStateChanged(d gatt.Device, s gatt.State) {
	log.Printf("State: %s", s)
	switch s {
	case gatt.StatePoweredOn:
		log.Printf("Scanning for PLAYBULB peripherals...")
		d.Scan([]gatt.UUID{}, true)
		return
	default:
		d.StopScanning()
	}
}

func main() {

	flag.Parse()
	if len(flag.Args()) != 1 {
		log.Fatalf("usage: %s [options] peripheral-id\n", os.Args[0])
	}

	// Gatt configuration
	peripheralId = strings.ToUpper(flag.Args()[0])

	device, err := gatt.NewDevice(DefaultClientOptions...)
	if err != nil {
		log.Fatalf("Failed to open device, err: %s\n", err)
		return
	}

	device.Handle(
		gatt.PeripheralDiscovered(onPeriphDiscovered),
		gatt.PeripheralConnected(onPeriphConnected),
		gatt.PeripheralDisconnected(onPeriphDisconnected),
	)

	device.Init(onStateChanged)

	// Routes for web service
	router := mux.NewRouter().StrictSlash(true)
	router.HandleFunc("/living/stripe", handleDeviceRequest)

	log.Fatal(http.ListenAndServe(":56666", router))

}