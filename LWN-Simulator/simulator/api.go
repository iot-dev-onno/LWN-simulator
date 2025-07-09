package simulator

import (
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/arslab/lwnsimulator/shared"
	"log"
	"strings"
	"os"
	

	"github.com/brocaar/lorawan"

	"github.com/arslab/lwnsimulator/codes"
	"github.com/arslab/lwnsimulator/models"

	dev "github.com/arslab/lwnsimulator/simulator/components/device"
	f "github.com/arslab/lwnsimulator/simulator/components/forwarder"
	mfw "github.com/arslab/lwnsimulator/simulator/components/forwarder/models"
	gw "github.com/arslab/lwnsimulator/simulator/components/gateway"
	c "github.com/arslab/lwnsimulator/simulator/console"
	"github.com/arslab/lwnsimulator/simulator/util"
	"github.com/arslab/lwnsimulator/socket"
	socketio "github.com/googollee/go-socket.io"
)

// Variables globales para el manejo de payloads CSV
type CSVPayload struct {
	Payload []byte
	FPort   uint8
}

var csvPayloads []CSVPayload
var csvIndex int


// Función para cargar los payloads desde un archivo CSV
func loadPayloadsFromCSV(path string) ([]CSVPayload, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	var payloads []CSVPayload

	// Si hay encabezado, descartarlo:
	if _, err := reader.Read(); err != nil {
		// puede no tener encabezado, pero seguimos
	}

	for {
		record, err := reader.Read()
		if err != nil {
			break
		}
		if len(record) < 2 {
			log.Println("Fila incompleta en CSV, se omite:", record)
			continue
		}

		hexStr := strings.TrimSpace(record[0])
		portStr := strings.TrimSpace(record[1])
		if hexStr == "" || portStr == "" {
			continue
		}

		// Decodificar payload hex
		b, err := hex.DecodeString(hexStr)
		if err != nil {
			log.Printf("Payload inválido en CSV: %s", hexStr)
			continue
		}

		// Parsear FPort
		var fport uint8
		if _, err := fmt.Sscanf(portStr, "%d", &fport); err != nil {
			log.Printf("FPort inválido en CSV: %s", portStr)
			continue
		}

		payloads = append(payloads, CSVPayload{
			Payload: b,
			FPort:   fport,
		})
	}
	return payloads, nil
}

// Funcion Modificada para hacer Dinamico el Payload y port 
func GetInstance() *Simulator {
	var s Simulator
	shared.DebugPrint("Init new Simulator instance")

	// Estado inicial
	s.State = util.Stopped
	// Carga de JSON
	s.loadData()

	// Inicializacion de mapas
	s.ActiveDevices = make(map[int]int)
	s.ActiveGateways = make(map[int]int)

	// --- Inyeccion de PayloadProvider ---
	const targetEUI = "4c565b2684f0319e"
	//staticPart := []byte{0xFF, 0xAA}

	// Cargar payload y fport de CSV
	var err error
	csvPayloads, err = loadPayloadsFromCSV("resultado.csv")
	if err != nil || len(csvPayloads) == 0 {
		log.Printf("Error loading CSV payloads: %v", err)
		// Fallback: un solo payload vacío y puerto 0
		csvPayloads = []CSVPayload{{Payload: []byte{0x00}, FPort: 0}}
	}
	csvIndex = 0

	for _, d := range s.Devices {
		// Guardamos el payload original por defecto
		var orig []byte
		if pl, ok := d.Info.Status.Payload.(*lorawan.DataPayload); ok {
			orig = append([]byte(nil), pl.Bytes...)
		}
		d.PayloadProvider = func() []byte {
			buf := make([]byte, len(orig))
			copy(buf, orig)
			return buf
		}

		// Si coincide el DevEUI, asignamos proveedor CSV
		if hex.EncodeToString(d.Info.DevEUI[:]) == targetEUI {
			d.PayloadProvider = func() []byte {
				p := csvPayloads[csvIndex].Payload
				// incrementamos índice (cíclico)
				csvIndex = (csvIndex + 1) % len(csvPayloads)
				out := make([]byte, len(p))
				copy(out, p)
				return out
			}
			d.FPortProvider = func() uint8 {
				// el puerto corresponde al payload anterior
				idx := (csvIndex - 1 + len(csvPayloads)) % len(csvPayloads)
				return csvPayloads[idx].FPort
			}
		}
	}

	//s.Forwarder = *forwarder.Setup()
	//s.Console = console.Console{}
	s.Forwarder = *f.Setup()
	s.Console = c.Console{}
	return &s
}





func (s *Simulator) AddWebSocket(WebSocket *socketio.Conn) {
	s.Console.SetupWebSocket(WebSocket)
	s.Resources.AddWebSocket(WebSocket)
	s.SetupConsole()
}

// Run starts the simulation environment
func (s *Simulator) Run() {
	shared.DebugPrint("Executing Run")
	s.State = util.Running
	s.setup()
	s.Print("START", nil, util.PrintBoth)
	shared.DebugPrint("Turning ON active components")
	for _, id := range s.ActiveGateways {
		s.turnONGateway(id)
	}
	for _, id := range s.ActiveDevices {
		s.turnONDevice(id)
	}
}

// Stop terminates the simulation environment
func (s *Simulator) Stop() {
	shared.DebugPrint("Executing Stop")
	s.State = util.Stopped
	s.Resources.ExitGroup.Add(len(s.ActiveGateways) + len(s.ActiveDevices) - s.ComponentsInactiveTmp)
	shared.DebugPrint("Turning OFF active components")
	for _, id := range s.ActiveGateways {
		s.Gateways[id].TurnOFF()
	}
	for _, id := range s.ActiveDevices {
		s.Devices[id].TurnOFF()
	}
	s.Resources.ExitGroup.Wait()
	s.saveStatus()
	s.Forwarder.Reset()
	s.Print("STOPPED", nil, util.PrintBoth)
	s.reset()
}

// SaveBridgeAddress stores the bridge address in the simulator struct and saves it to the simulator.json file
func (s *Simulator) SaveBridgeAddress(remoteAddr models.AddressIP) error {
	// Store the bridge address in the simulator struct
	s.BridgeAddress = fmt.Sprintf("%v:%v", remoteAddr.Address, remoteAddr.Port)
	pathDir, err := util.GetPath()
	if err != nil {
		log.Fatal(err)
	}
	path := pathDir + "/simulator.json"
	s.saveComponent(path, &s)
	s.Print("Gateway Bridge Address saved", nil, util.PrintOnlyConsole)
	return nil
}

// GetBridgeAddress returns the bridge address stored in the simulator struct
func (s *Simulator) GetBridgeAddress() models.AddressIP {
	// Create an empty AddressIP struct with default values
	var rServer models.AddressIP
	if s.BridgeAddress == "" {
		return rServer
	}
	// Split the bridge address into address and port
	parts := strings.Split(s.BridgeAddress, ":")
	rServer.Address = parts[0]
	rServer.Port = parts[1]
	return rServer
}

// GetGateways returns an array of all gateways in the simulator
func (s *Simulator) GetGateways() []gw.Gateway {
	var gateways []gw.Gateway
	for _, g := range s.Gateways {
		gateways = append(gateways, *g)
	}
	return gateways
}

// GetDevices returns an array of all devices in the simulator
func (s *Simulator) GetDevices() []dev.Device {
	var devices []dev.Device
	for _, d := range s.Devices {
		devices = append(devices, *d)
	}
	return devices
}

// SetGateway adds or updates a gateway
func (s *Simulator) SetGateway(gateway *gw.Gateway, update bool) (int, int, error) {
	shared.DebugPrint(fmt.Sprintf("Adding/Updating Gateway [%s]", gateway.Info.MACAddress.String()))
	emptyAddr := lorawan.EUI64{0, 0, 0, 0, 0, 0, 0, 0}
	// Check if the MAC address is valid
	if gateway.Info.MACAddress == emptyAddr {
		s.Print("Error: MAC Address invalid", nil, util.PrintOnlyConsole)
		return codes.CodeErrorAddress, -1, errors.New("Error: MAC Address invalid")
	}
	// If the gateway is new, assign a new ID
	if !update {
		gateway.Id = s.NextIDGw
	} else { // If the gateway is being updated, it must be turned off
		if s.Gateways[gateway.Id].IsOn() {
			return codes.CodeErrorDeviceActive, -1, errors.New("Gateway is running, unable update")
		}
	}
	// Check if the name is already used
	code, err := s.searchName(gateway.Info.Name, gateway.Id, true)
	if err != nil {
		s.Print("Name already used", nil, util.PrintOnlyConsole)
		return code, -1, err
	}
	// Check if the name is already used
	code, err = s.searchAddress(gateway.Info.MACAddress, gateway.Id, true)
	if err != nil {
		s.Print("DevEUI already used", nil, util.PrintOnlyConsole)
		return code, -1, err
	}
	if !gateway.Info.TypeGateway {

		if s.BridgeAddress == "" {
			return codes.CodeNoBridge, -1, errors.New("No gateway bridge configured")
		}

	}

	s.Gateways[gateway.Id] = gateway

	pathDir, err := util.GetPath()
	if err != nil {
		log.Fatal(err)
	}

	path := pathDir + "/gateways.json"
	s.saveComponent(path, &s.Gateways)
	path = pathDir + "/simulator.json"
	s.saveComponent(path, &s)

	s.Print("Gateway Saved", nil, util.PrintOnlyConsole)

	if gateway.Info.Active {

		s.ActiveGateways[gateway.Id] = gateway.Id

		if s.State == util.Running {
			s.Gateways[gateway.Id].Setup(&s.BridgeAddress, &s.Resources, &s.Forwarder)
			s.turnONGateway(gateway.Id)
		}

	} else {
		_, ok := s.ActiveGateways[gateway.Id]
		if ok {
			delete(s.ActiveGateways, gateway.Id)
		}
	}
	s.NextIDGw++
	return codes.CodeOK, gateway.Id, nil
}

func (s *Simulator) DeleteGateway(Id int) bool {

	if s.Gateways[Id].IsOn() {
		return false
	}

	delete(s.Gateways, Id)
	delete(s.ActiveGateways, Id)

	pathDir, err := util.GetPath()
	if err != nil {
		log.Fatal(err)
	}

	path := pathDir + "/gateways.json"
	s.saveComponent(path, &s.Gateways)

	s.Print("Gateway Deleted", nil, util.PrintOnlyConsole)

	return true
}

func (s *Simulator) SetDevice(device *dev.Device, update bool) (int, int, error) {

	emptyAddr := lorawan.EUI64{0, 0, 0, 0, 0, 0, 0, 0}

	if device.Info.DevEUI == emptyAddr {

		s.Print("DevEUI invalid", nil, util.PrintOnlyConsole)
		return codes.CodeErrorAddress, -1, errors.New("Error: DevEUI invalid")

	}

	if !update { //new

		device.Id = s.NextIDDev
		s.NextIDDev++

	} else {

		if s.Devices[device.Id].IsOn() {
			return codes.CodeErrorDeviceActive, -1, errors.New("Device is running, unable update")
		}

	}

	code, err := s.searchName(device.Info.Name, device.Id, false)
	if err != nil {

		s.Print("Name already used", nil, util.PrintOnlyConsole)
		return code, -1, err

	}

	code, err = s.searchAddress(device.Info.DevEUI, device.Id, false)
	if err != nil {

		s.Print("DevEUI already used", nil, util.PrintOnlyConsole)
		return code, -1, err

	}

	s.Devices[device.Id] = device

	pathDir, err := util.GetPath()
	if err != nil {
		log.Fatal(err)
	}

	path := pathDir + "/devices.json"
	s.saveComponent(path, &s.Devices)
	path = pathDir + "/simulator.json"
	s.saveComponent(path, &s)

	s.Print("Device Saved", nil, util.PrintOnlyConsole)

	if device.Info.Status.Active {

		s.ActiveDevices[device.Id] = device.Id

		if s.State == util.Running {
			s.turnONDevice(device.Id)
		}

	} else {
		_, ok := s.ActiveDevices[device.Id]
		if ok {
			delete(s.ActiveDevices, device.Id)
		}
	}

	return codes.CodeOK, device.Id, nil
}

func (s *Simulator) DeleteDevice(Id int) bool {

	if s.Devices[Id].IsOn() {
		return false
	}

	delete(s.Devices, Id)
	delete(s.ActiveDevices, Id)

	pathDir, err := util.GetPath()
	if err != nil {
		log.Fatal(err)
	}

	path := pathDir + "/devices.json"
	s.saveComponent(path, &s.Devices)

	s.Print("Device Deleted", nil, util.PrintOnlyConsole)

	return true
}

func (s *Simulator) ToggleStateDevice(Id int) {

	if s.Devices[Id].State == util.Stopped {
		s.turnONDevice(Id)
	} else if s.Devices[Id].State == util.Running {
		s.turnOFFDevice(Id)
	}

}

func (s *Simulator) SendMACCommand(cid lorawan.CID, data socket.MacCommand) {

	if !s.Devices[data.Id].IsOn() {
		s.Console.PrintSocket(socket.EventResponseCommand, s.Devices[data.Id].Info.Name+" is turned off")
		return
	}

	err := s.Devices[data.Id].SendMACCommand(cid, data.Periodicity)
	if err != nil {
		s.Console.PrintSocket(socket.EventResponseCommand, "Unable to send command: "+err.Error())
	} else {
		s.Console.PrintSocket(socket.EventResponseCommand, "MACCommand will be sent to the next uplink")
	}

}

func (s *Simulator) ChangePayload(pl socket.NewPayload) (string, bool) {

	devEUIstring := hex.EncodeToString(s.Devices[pl.Id].Info.DevEUI[:])

	if !s.Devices[pl.Id].IsOn() {
		s.Console.PrintSocket(socket.EventResponseCommand, s.Devices[pl.Id].Info.Name+" is turned off")
		return devEUIstring, false
	}

	MType := lorawan.UnconfirmedDataUp
	if pl.MType == "ConfirmedDataUp" {
		MType = lorawan.ConfirmedDataUp
	}

	Payload := &lorawan.DataPayload{
		Bytes: []byte(pl.Payload),
	}

	s.Devices[pl.Id].ChangePayload(MType, Payload)

	s.Console.PrintSocket(socket.EventResponseCommand, s.Devices[pl.Id].Info.Name+": Payload changed")

	return devEUIstring, true
}

func (s *Simulator) SendUplink(pl socket.NewPayload) {

	if !s.Devices[pl.Id].IsOn() {
		s.Console.PrintSocket(socket.EventResponseCommand, s.Devices[pl.Id].Info.Name+" is turned off")
		return
	}

	MType := lorawan.UnconfirmedDataUp
	if pl.MType == "ConfirmedDataUp" {
		MType = lorawan.ConfirmedDataUp
	}

	s.Devices[pl.Id].NewUplink(MType, pl.Payload)

	s.Console.PrintSocket(socket.EventResponseCommand, "Uplink queued")
}

func (s *Simulator) ChangeLocation(l socket.NewLocation) bool {

	if !s.Devices[l.Id].IsOn() {
		return false
	}

	s.Devices[l.Id].ChangeLocation(l.Latitude, l.Longitude, l.Altitude)

	info := mfw.InfoDevice{
		DevEUI:   s.Devices[l.Id].Info.DevEUI,
		Location: s.Devices[l.Id].Info.Location,
		Range:    s.Devices[l.Id].Info.Configuration.Range,
	}

	s.Forwarder.UpdateDevice(info)

	return true
}

func (s *Simulator) ToggleStateGateway(Id int) {

	if s.Gateways[Id].State == util.Stopped {
		s.turnONGateway(Id)
	} else {
		s.turnOFFGateway(Id)
	}

}