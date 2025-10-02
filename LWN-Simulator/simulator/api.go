package simulator

import (
	"encoding/binary"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/arslab/lwnsimulator/shared"

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

// Índice por DevEUI para secuencial / último índice usado (también lo pisan los aleatorios)
var csvIndice map[string]int

// RNG por dispositivo para modo aleatorio
var csvRnd map[string]*rand.Rand

// Mutex para proteger maps
var csvMu sync.RWMutex

// Qué grupos (ruta CSV) usan payload aleatorio
var randomGroups = map[string]bool{
	//"SensorNPS/nps.csv": true, // <-- este CSV se envía en orden aleatorio
}

// Función para cargar los payloads desde un archivo CSV
func loadPayloadsFromCSV(path string) ([]CSVPayload, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	var payloads []CSVPayload

	// Intentar leer la primera fila y decidir si es header o dato
	first, err := reader.Read()
	if err == nil && len(first) >= 2 {
		hexStr := strings.TrimSpace(first[0])
		portStr := strings.TrimSpace(first[1])

		// Intentar parsear como dato; si falla, asumimos encabezado y no agregamos
		if hexStr != "" && portStr != "" {
			if b, errHex := hex.DecodeString(hexStr); errHex == nil {
				var fport uint8
				if _, errPort := fmt.Sscanf(portStr, "%d", &fport); errPort == nil {
					payloads = append(payloads, CSVPayload{Payload: b, FPort: fport})
				}
			}
		}
	} else if err != nil {
		// archivo vacío o error de lectura inicial
		return nil, err
	}

	// Leer el resto de filas normalmente
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

	// Cargar payload y fport de CSV
	//--------------------------------------------------------------------------------------------------------------------------------------
	// EN ESTA PARTE SE INGRESA EL SENSOR QUE SE QUIERE SIMULAR Y LOS DISPOSITIVOS
	groupConfigs := map[string][]string{
		"SensorA/contador.csv": {
			"84e58aa1be8c69e8",
		},
		//-----------------------------------SENSOR DE APERTURA------------------------------------------------------------------------------------
		"SensorApertura/apertura1.csv": {
			"1aca540a14c43e2b",
		},
		"SensorApertura/apertura2.csv": {
			"8579dd1b567b809b",
		},
		"SensorApertura/apertura3.csv": {
			"3a611a1546007ea6",
		},
		"SensorApertura/apertura4.csv": {
			"eb6b49ea494cba92",
		},
		"SensorApertura/apertura5.csv": {
			"6644b52c9cc294eb",
		},
		//-----------------------------------SENSOR DE GPS------------------------------------------------------------------------------------
		"SensorGPS/SENSOR_RPM.csv": {
			"6746e85e6f91dd63",
		},
		//-----------------------------------------SENSORES NPS----------------------------------------------------
		"SensorNPS/s1.csv": {
			"cad6d3096056b748",
		},
		"SensorNPS/s2.csv": {
			"ef2512b553568286",
		},
		"SensorNPS/s3.csv": {
			"aea3589943cc9ee6",
		},
		"SensorNPS/s4.csv": {
			"4bb04a7ca707a7ed",
		},
		"SensorNPS/s5.csv": {
			"29837198855b0855",
		},
		//------------------------------------------------------SENSOR DE HUELLA---------------------------------------------------------------------
		"HUELLA/id1.csv": {
			"71b2dcc1b34135ae",
		},
		"HUELLA/id2.csv": {
			"e9c1f31f99366222",
		},
		"HUELLA/id3.csv": {
			"262e89356d320035",
		},
		"HUELLA/id4.csv": {
			"1e0d568b9544d928",
		},
		"HUELLA/id5.csv": {
			"c57949d36e38792a",
		},
		"HUELLA/id6.csv": {
			"a63113c90e4deb87",
		},
		"HUELLA/id7.csv": {
			"7653162876071e7b",
		},
		"HUELLA/id8.csv": {
			"0d2bb51e68339d18",
		},
		"HUELLA/id9.csv": {
			"de6b82c111978638",
		},
		"HUELLA/id10.csv": {
			"69e5c4b295323442",
		},
		"HUELLA/id11.csv": {
			"6e289d7a3685157a",
		},
		"HUELLA/id12.csv": {
			"b2a4f816bc29de7a",
		},
		"HUELLA/id13.csv": {
			"424c113e674ee54c",
		},
		"HUELLA/id14.csv": {
			"7de581d16a94b117",
		},
		"HUELLA/id15.csv": {
			"51e036ca22c9716a",
		},
		"HUELLA/id16.csv": {
			"4cbdfd83bc390f81",
		},
		"HUELLA/id17.csv": {
			"23a8b57ca1744b6a",
		},
		"HUELLA/id18.csv": {
			"86aa9b60cf7c99f2",
		},
		"HUELLA/id19.csv": {
			"8aedba156bd2f32f",
		},
		"HUELLA/id20.csv": {
			"c7f27c980dd3dfe2",
		},
		"HUELLA/id21.csv": {
			"2f388648c7df09dd",
		},
		"HUELLA/id22.csv": {
			"00659621923c246e",
		},
		"HUELLA/id23.csv": {
			"fbbac082dbea913f",
		},
		"HUELLA/id24.csv": {
			"770825c4f81b22dc",
		},
		"HUELLA/id25.csv": {
			"d942ffe79af90954",
		},
		"HUELLA/id26.csv": {
			"8e7e2d701d11d704",
		},
		"HUELLA/id27.csv": {
			"0359a2a9c43298dd",
		},
		"HUELLA/id28.csv": {
			"605716f929b775f8",
		},
		"HUELLA/id29.csv": {
			"1052099b498d6e07",
		},
		"HUELLA/id30.csv": {
			"2efbf9fd7c251461",
		},
		"HUELLA/id31.csv": {
			"824c9885f021dcaa",
		},
		"HUELLA/id32.csv": {
			"9373523a72f7e971",
		},
		"HUELLA/id33.csv": {
			"17d33959c1c29aaa",
		},
		"HUELLA/id34.csv": {
			"bcec7b4dee1e3edc",
		},
		"HUELLA/id35.csv": {
			"43e928db02b4b72a",
		},
		"HUELLA/id36.csv": {
			"89f095e12a1a2a74",
		},
		"HUELLA/id37.csv": {
			"0bcfee32372b385a",
		},
		"HUELLA/id38.csv": {
			"54a32679a312b36d",
		},
		"HUELLA/id39.csv": {
			"512b1ce4149338da",
		},
		"HUELLA/id40.csv": {
			"e15392c602def648",
		},
		"HUELLA/id41.csv": {
			"6fb3bac34b55dd88",
		},
		"HUELLA/id42.csv": {
			"9e433ba8ccfcd8c5",
		},
		"HUELLA/id43.csv": {
			"a2371d3fa358ccf1",
		},
		"HUELLA/id44.csv": {
			"0e236cd5f037cedd",
		},
		"HUELLA/id45.csv": {
			"c751bfecae0bfa00",
		},
		"HUELLA/id46.csv": {
			"6f1887ecd9124ffa",
		},
		"HUELLA/id47.csv": {
			"145f71f35a702ac3",
		},
		"HUELLA/id48.csv": {
			"70dde8be250233bd",
		},
		"HUELLA/id49.csv": {
			"69cd3752697ed38a",
		},
		"HUELLA/id50.csv": {
			"ac8e9f26c7ba9166",
		},
	}
	//---------------------------------------------------------------------------------------------------------------------------------------

	groupPayloads := make(map[string][]CSVPayload, len(groupConfigs))
	for csvPath := range groupConfigs {
		payloads, err := loadPayloadsFromCSV(csvPath)
		if err != nil || len(payloads) == 0 {
			payloads = []CSVPayload{{Payload: []byte{0x00}, FPort: 0}}
		}
		groupPayloads[csvPath] = payloads
	}

	// Inicializar estructuras globales
	csvIndice = make(map[string]int)
	csvRnd = make(map[string]*rand.Rand)

	// Configurar providers por dispositivo
	for _, d := range s.Devices {
		euiStr := hex.EncodeToString(d.Info.DevEUI[:])

		// Guardar el payload original por defecto
		var orig []byte
		if pl, ok := d.Info.Status.Payload.(*lorawan.DataPayload); ok {
			orig = append([]byte(nil), pl.Bytes...)
		}
		d.PayloadProvider = func() []byte {
			buf := make([]byte, len(orig))
			copy(buf, orig)
			return buf
		}

		for csvPath, euis := range groupConfigs {
			for _, g := range euis {
				if g == euiStr {
					// Inicializar índice si no existe
					csvMu.Lock()
					if _, seen := csvIndice[euiStr]; !seen {
						csvIndice[euiStr] = 0
					}
					csvMu.Unlock()

					key, payloads := euiStr, groupPayloads[csvPath]
					if len(payloads) == 0 {
						// Seguridad por si no cargó nada
						payloads = []CSVPayload{{Payload: []byte{0x00}, FPort: 0}}
					}

					// ¿Este grupo usa aleatorio?
					if randomGroups[csvPath] {
						// Generador por-dev (semilla estable por tiempo + EUI)
						csvMu.Lock()
						if _, ok := csvRnd[key]; !ok {
							seed := time.Now().UnixNano() ^
								int64(binary.LittleEndian.Uint32(d.Info.DevEUI[0:4]))
							csvRnd[key] = rand.New(rand.NewSource(seed))
						}
						r := csvRnd[key]
						csvMu.Unlock()

						// ALEATORIO: cada uplink escoge un índice random y lo guarda en csvIndice[key]
						d.PayloadProvider = func() []byte {
							csvMu.Lock()
							idx := r.Intn(len(payloads))
							csvIndice[key] = idx // guardar "último usado" para FPort
							p := payloads[idx].Payload
							csvMu.Unlock()

							out := make([]byte, len(p))
							copy(out, p)
							return out
						}
						d.FPortProvider = func() uint8 {
							csvMu.RLock()
							last := csvIndice[key]
							fp := payloads[last].FPort
							csvMu.RUnlock()
							return fp
						}
					} else {
						// SECUENCIAL (como ya lo tenías)
						d.PayloadProvider = func() []byte {
							csvMu.Lock()
							idx := csvIndice[key]
							p := payloads[idx].Payload
							// avanzar circularmente
							csvIndice[key] = (idx + 1) % len(payloads)
							csvMu.Unlock()

							out := make([]byte, len(p))
							copy(out, p)
							return out
						}
						d.FPortProvider = func() uint8 {
							// Para el FPort, usar el índice previamente enviado
							csvMu.RLock()
							prev := (csvIndice[key] - 1 + len(payloads)) % len(payloads)
							fp := payloads[prev].FPort
							csvMu.RUnlock()
							return fp
						}
					}
					break
				}
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
