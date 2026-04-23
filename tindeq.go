package gotindeq

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"tinygo.org/x/bluetooth"
)

const TARGET_DEVICE = "Progressor"

var adapter = bluetooth.DefaultAdapter

type WeightMeasurement struct {
	Weight    float64
	Timestamp time.Duration
}

type responseCodes struct {
	cmdResponse   byte
	weightMeasure byte
	lowPower      byte
}

func newResponseCodes() responseCodes {
	return responseCodes{
		cmdResponse:   0x00,
		weightMeasure: 0x01,
		lowPower:      0x04,
	}
}

type serviceUUIDS struct {
	serviceUUID bluetooth.UUID
	writeUUID   bluetooth.UUID
	notifyUUID  bluetooth.UUID
}

func setServiceUUIDS() serviceUUIDS {
	return serviceUUIDS{
		serviceUUID: parseServiceUUID("7e4e1701-1ea6-40c9-9dcc-13d34ffead57"),
		writeUUID:   parseServiceUUID("7e4e1703-1ea6-40c9-9dcc-13d34ffead57"),
		notifyUUID:  parseServiceUUID("7e4e1702-1ea6-40c9-9dcc-13d34ffead57"),
	}
}

type commands struct {
	TARE_SCALE                 byte
	START_WEIGHT_MEAS          byte
	STOP_WEIGHT_MEAS           byte
	START_PEAK_RFD_MEAS        byte
	START_PEAK_RFD_MEAS_SERIES byte
	ADD_CALIB_POINT            byte
	SAVE_CALIB                 byte
	GET_APP_VERSION            byte
	GET_ERR_INFO               byte
	CLR_ERR_INFO               byte
	SLEEP                      byte
	GET_BATT_VLTG              byte
}

func newCommands() commands {
	return commands{
		TARE_SCALE:                 0x64,
		START_WEIGHT_MEAS:          0x65,
		STOP_WEIGHT_MEAS:           0x66,
		START_PEAK_RFD_MEAS:        0x67,
		START_PEAK_RFD_MEAS_SERIES: 0x68,
		ADD_CALIB_POINT:            0x69,
		SAVE_CALIB:                 0x6A,
		GET_APP_VERSION:            0x6B,
		GET_ERR_INFO:               0x6C,
		CLR_ERR_INFO:               0x6D,
		SLEEP:                      0x6E,
		GET_BATT_VLTG:              0x6F,
	}
}

type TindeqClient struct {
	ConnectedDevice      bluetooth.Device
	responseCodes        responseCodes
	Commands             commands
	serviceUUIDS         serviceUUIDS
	NotifyCharacteristic bluetooth.DeviceCharacteristic
	WriteCharacteristic  bluetooth.DeviceCharacteristic
}

func NewTindeqClient() (*TindeqClient, error) {
	tq := &TindeqClient{
		responseCodes: newResponseCodes(),
		Commands:      newCommands(),
		serviceUUIDS:  setServiceUUIDS(),
	}

	if err := tq.connect(); err != nil {
		return nil, err
	}

	if err := tq.discoverServices(); err != nil {
		return nil, err
	}

	return tq, nil

}

func (tq *TindeqClient) connect() error {
	fmt.Println("Searching for progressor...")

	if err := adapter.Enable(); err != nil {
		return err
	}

	var scanResult bluetooth.ScanResult

	fmt.Println("Scanning for devices...")
	if err := adapter.Scan(func(adapter *bluetooth.Adapter, device bluetooth.ScanResult) {
		if strings.Contains(device.LocalName(), TARGET_DEVICE) {
			scanResult = device
			adapter.StopScan()
			fmt.Println("found Progressor:", device.Address.String(), device.RSSI, device.LocalName())
		}
	}); err != nil {
		return err
	}

	fmt.Println("Now connecting...")
	dev, err := adapter.Connect(scanResult.Address, bluetooth.ConnectionParams{
		ConnectionTimeout: bluetooth.NewDuration(time.Second * 5),
		MinInterval:       bluetooth.NewDuration(time.Millisecond * 500),
		MaxInterval:       bluetooth.NewDuration(time.Second * 1),
		Timeout:           bluetooth.NewDuration(time.Minute * 10),
	})
	if err != nil {
		return err
	}
	tq.ConnectedDevice = dev
	fmt.Println("Succesfully connected to Progressor, now discovering services...")
	return nil
}

func (tq *TindeqClient) discoverServices() error {
	deviceServices, err := tq.ConnectedDevice.DiscoverServices([]bluetooth.UUID{tq.serviceUUIDS.serviceUUID})
	if err != nil {
		return err
	}
	fmt.Println("Found service: ", deviceServices[0].UUID())
	deviceCharacteristics, err := deviceServices[0].DiscoverCharacteristics([]bluetooth.UUID{
		tq.serviceUUIDS.notifyUUID,
		tq.serviceUUIDS.writeUUID,
	})
	if err != nil {
		return err
	}

	tq.NotifyCharacteristic = deviceCharacteristics[0]
	tq.WriteCharacteristic = deviceCharacteristics[1]

	return nil
}

func (tq *TindeqClient) SendCommand(cmd byte) error {
	//TODO Change the command type, maybe use consts?
	_, err := tq.WriteCharacteristic.WriteWithoutResponse([]byte{cmd})
	if err != nil {
		return err
	}

	return nil
}

func (tq *TindeqClient) EnableNotifcations(ch chan<- WeightMeasurement) error {
	if err := tq.NotifyCharacteristic.EnableNotifications(func(buf []byte) {
		parseTLV(buf, ch)
	}); err != nil {
		return err
	}

	return nil
}

func (tq *TindeqClient) Close() {
	tq.NotifyCharacteristic.EnableNotifications(nil)
	tq.ConnectedDevice.Disconnect()
}

func parseServiceUUID(uuid string) bluetooth.UUID {
	parsed_uuid, err := bluetooth.ParseUUID(uuid)
	if err != nil {
		log.Fatalln("Failed to parse service uuid")
	}
	return parsed_uuid
}

func parseTLV(buf []byte, ch chan<- WeightMeasurement) error {
	fmt.Println("Total buf length: ", len(buf))
	if len(buf) < 2 {
		return fmt.Errorf("Malformed TLV, not enough bytes for tag & length")
	}

	tag := buf[0]
	length := int(buf[1])

	if 2+length > len(buf) {
		return fmt.Errorf("  Warning: malformed TLV - tag=0x%02X, declared length=%d but insufficient data\n", tag, length)
	}

	value := buf[2 : 2+length]

	// fmt.Printf("  TLV → Tag: 0x%02X, Length: %d, Value: % x\n", tag, length, value)

	i := 0
	step := 4
	//The weight measurement (float32) and timestamp (uint_t32) to go along with it are each 4 bytes or rather 32 bits.
	iter := 1

	switch tag {
	case 0x01:

		for i < length {

			weightBits := binary.LittleEndian.Uint32(value[i : i+step])
			weightMeasurement := math.Round(float64(math.Float32frombits(weightBits))*10) / 10
			i += step
			fmt.Printf("Weight measurement: %v kg\n", weightMeasurement)
			sec := time.Duration(binary.LittleEndian.Uint32(value[i:i+step])) * time.Microsecond

			fmt.Printf("Time: %v\n", sec.Seconds())

			ch <- WeightMeasurement{
				Weight:    weightMeasurement,
				Timestamp: sec,
			}

			i += step
			iter += 1

		}

	}
	return nil
}
