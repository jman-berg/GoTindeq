package tindeq

import (
	"fmt"
	"strings"
	"time"

	"tinygo.org/x/bluetooth"
)

const TARGET_DEVICE = "Progressor"

var adapter = bluetooth.DefaultAdapter

type response_codes struct {
	cmd_resp       int
	weight_measure int
	low_pwr        int
}

func newResponseCodes() response_codes {
	return response_codes{
		cmd_resp:       0,
		weight_measure: 1,
		low_pwr:        4,
	}
}

type Cmd byte

type commands struct {
	TARE_SCALE                 Cmd
	START_WEIGHT_MEAS          Cmd
	STOP_WEIGHT_MEAS           Cmd
	START_PEAK_RFD_MEAS        Cmd
	START_PEAK_RFD_MEAS_SERIES Cmd
	ADD_CALIB_POINT            Cmd
	SAVE_CALIB                 Cmd
	GET_APP_VERSION            Cmd
	GET_ERR_INFO               Cmd
	CLR_ERR_INFO               Cmd
	SLEEP                      Cmd
	GET_BATT_VLTG              Cmd
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

type TindeqProgressor struct {
	connected_device bluetooth.Device
	response_codes   response_codes
	cmds             commands
	service_uuid     string
	write_uuid       string
	notify_uuid      string
}

func NewTindeqClient() TindeqProgressor {
	return TindeqProgressor{
		response_codes: newResponseCodes(),
		cmds:           newCommands(),
		service_uuid:   "7e4e1701-1ea6-40c9-9dcc-13d34ffead57",
		write_uuid:     "7e4e1703-1ea6-40c9-9dcc-13d34ffead57",
		notify_uuid:    "7e4e1702-1ea6-40c9-9dcc-13d34ffead57",
	}

}

func (tq *TindeqProgressor) Connect() error {
	fmt.Println("Searching for progressor...")

	if err := adapter.Enable(); err != nil {
		return err
	}

	var scan_result bluetooth.ScanResult

	fmt.Println("Scanning for devices...")
	if err := adapter.Scan(func(adapter *bluetooth.Adapter, device bluetooth.ScanResult) {
		if strings.Contains(device.LocalName(), TARGET_DEVICE) {
			scan_result = device
			adapter.StopScan()
			fmt.Println("found Progressor:", device.Address.String(), device.RSSI, device.LocalName())
		}
	}); err != nil {
		return err
	}

	fmt.Println("Now connecting...")
	dev, err := adapter.Connect(scan_result.Address, bluetooth.ConnectionParams{
		ConnectionTimeout: bluetooth.NewDuration(time.Second * 5),
		MinInterval:       bluetooth.NewDuration(time.Millisecond * 500),
		MaxInterval:       bluetooth.NewDuration(time.Second * 1),
		Timeout:           bluetooth.NewDuration(time.Minute * 10),
	})
	if err != nil {
		return err
	}
	tq.connected_device = dev
	fmt.Println("Succesfully connected to Progressor")
	return nil
}
