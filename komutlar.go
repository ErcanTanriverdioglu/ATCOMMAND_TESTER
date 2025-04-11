package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"go.bug.st/serial"
)

var cid = 1
var pdpType = "IP"
var apn = "internet"

// ATCommand yapısı ve komut listesi
type ATCommand struct {
	Command         string
	SuccessResponse string
	Timeout         time.Duration
	Flag            uint64
}

var ATCommands = []ATCommand{
	// Hata durumları
	{"", "ERROR", 2 * time.Second, gsmERROR},
	{"", "+CME ERROR", 2 * time.Second, gsmERROR},
	{"", "+CMS ERROR", 2 * time.Second, gsmERROR},

	// Temel komutlar
	{"AT+IPR=115200", "OK", 2 * time.Second, gsmOK},
	{"ATE0", "OK", 2 * time.Second, gsmOK},
	{"AT+CPIN?", "+CPIN: READY", 5 * time.Second, gsmCPIN},
	{"AT+CREG?", "+CREG:", 2 * time.Second, gsmCREG},
	{"AT+QGSN", "+QGSN:", 2 * time.Second, gsmIMEI},
	{"AT+CTZU=3", "OK", 2 * time.Second, gsmOK},
	{"AT+CCLK?", "+CCLK:", 2 * time.Second, gsmTIME},
	{"AT+CGATT?", "+CGATT: 1", 2 * time.Second, gsmOK},
	{"AT+CGDCONT", "OK", time.Second, gsmOK},
	{"AT+CGACT=1,1", "OK", 150 * time.Second, gsmOK},
	{"AT+QIOPEN", "CONNECT OK", 20 * time.Second, gsmCONNECT_OK},
	{"AT+QIDEACT", "DEACT OK", 2 * time.Second, gsmDEACT_OK},
}

// Flag tanımları
const (
	gsmERROR = 1 << iota
	gsmCPIN
	gsmIMEI
	gsmOK
	gsmCREG
	gsmTIME
	gsmCONNECT_OK
	gsmDEACT_OK
	// Diğer flag'ler...
)

func main() {
	fmt.Println("SCANNING COM PORT...")
	ports, err := serial.GetPortsList()
	if err != nil {
		log.Fatal("COM LIST GET ERROR:", err)
	}
	if len(ports) == 0 {
		log.Fatal("COM LIST NOT FIND!")
	}
	for _, port := range ports {
		fmt.Println("USABLE COM:")
		fmt.Println("-", port)
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("\nCOM PORT SELECT: ")
	comPort, _ := reader.ReadString('\n')
	comPort = strings.TrimSpace(comPort)

	fmt.Print("\nBAUD RATE: ")
	baudRateStr, _ := reader.ReadString('\n')
	baudRateStr = strings.TrimSpace(baudRateStr)

	var baudRate int
	if _, err := fmt.Sscanf(baudRateStr, "%d", &baudRate); err != nil {
		log.Fatal("INVALID BAUD RATE:", err)
	}

	mode := &serial.Mode{
		BaudRate: baudRate,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}

	port, err := serial.Open(comPort, mode)
	if err != nil {
		log.Fatal("PORT OPEN ERROR:", err)
	}
	defer port.Close()

	fmt.Print("\nCHOOSE MODE (TEST / MANUAL): ")
	modeInput, _ := reader.ReadString('\n')
	modeInput = strings.TrimSpace(strings.ToUpper(modeInput))

	switch modeInput {
	case "TEST":
		fmt.Println("\nTEST MODE: Sending AT Command...")
		runTestMode(port)
	case "MANUAL":
		fmt.Println("\nMANUAL MODE: Enter Command. \"STOP\" for exit.")
		runManualMode(port, reader)
	default:
		fmt.Println("Error. Program terminated.")
	}
}

func runTestMode(port serial.Port) {
	sendATCommand(port, ATCommands[3], "")
	sendATCommand(port, ATCommands[4], "")
	sendATCommand(port, ATCommands[5], "")
	sendATCommand(port, ATCommands[6], "")
	sendATCommand(port, ATCommands[7], "")
	sendATCommand(port, ATCommands[8], "")
	sendATCommand(port, ATCommands[9], "")
	sendATCommand(port, ATCommands[10], "")
	sendATCommand(port, ATCommands[11], "%d,\"%s\",\"%s\"", cid, pdpType, apn)
	sendATCommand(port, ATCommands[12], "")
	sendATCommand(port, ATCommands[13], "")
	sendATCommand(port, ATCommands[14], "")
	fmt.Println("\n✅ TEST FINISHED.")
}

func runManualMode(port serial.Port, reader *bufio.Reader) {
	for {
		fmt.Print("MANUAL >> ")
		input, err := reader.ReadString('\n')
		if err != nil {
			log.Printf("INPUT READ ERROR: %v", err)
			continue
		}

		input = strings.TrimSpace(input)
		if strings.ToUpper(input) == "STOP" {
			fmt.Println("MANUAL MOD STOPPED.")
			break
		}

		if input == "" {
			continue
		}

		_, err = port.Write([]byte(input + "\r\n"))
		if err != nil {
			log.Printf("SEND ERROR: %v", err)
			continue
		}

		// 5 saniye boyunca gelen cevapları oku
		readAllResponses(port, 5*time.Second)
	}
}

func readAllResponses(port serial.Port, idleTimeout time.Duration) {
	buf := make([]byte, 256)
	response := strings.Builder{}
	lastReceived := time.Now()

	for {
		// Eğer idle süresi geçtiyse döngüyü kır
		if time.Since(lastReceived) > idleTimeout {
			break
		}

		port.SetReadTimeout(100 * time.Millisecond) // Her 100 ms'de bir veri kontrol et
		n, err := port.Read(buf)
		if err != nil {
			continue
		}

		if n > 0 {
			chunk := string(buf[:n])
			response.WriteString(chunk)
			fmt.Print(chunk)

			// Veri geldiyse zamanlayıcıyı sıfırla
			lastReceived = time.Now()
		}
	}
}

func sendATCommand(port serial.Port, cmd ATCommand, format string, a ...interface{}) {
	if cmd.Command == "" {
		return
	}

	fullCommand := cmd.Command
	if format != "" {
		fullCommand = fmt.Sprintf("%s=%s", fullCommand, fmt.Sprintf(format, a...))
	}

	const maxRetries = 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		fmt.Printf("\n%d. TRIAL >>> SEND: [%s]\n", attempt, fullCommand)

		_, err := port.Write([]byte(fullCommand + "\r\n"))
		if err != nil {
			log.Printf("SEND ERROR: %v", err)
			continue
		}

		response, matched := readResponseWithTimeout(port, cmd.Timeout, cmd.SuccessResponse)
		if matched {
			fmt.Printf("SUCCESS RETURN VALUE : %s\n", cmd.SuccessResponse)
			time.Sleep(1 * time.Second)
			return
		} else {
			log.Printf("WARNING: Expected response not received. Command: %s\nExpected: %s\nReceived: %s",
				fullCommand, cmd.SuccessResponse, response)
		}
	}

	log.Fatalf("\n❌ The command was tried 3 times but did not succeed. The process is being stopped. Command: %s\n", fullCommand)
}

func readResponseWithTimeout(port serial.Port, timeout time.Duration, expected string) (string, bool) {
	buf := make([]byte, 256)
	var response strings.Builder
	timeoutChan := time.After(timeout)
	matched := false

	for {
		select {
		case <-timeoutChan:
			return response.String(), matched
		default:
			n, err := port.Read(buf)
			if err != nil {
				continue
			}
			if n > 0 {
				response.Write(buf[:n])
				fmt.Printf("%s", buf[:n]) // Gerçek zamanlı görüntüleme

				// Beklenen yanıtı kontrol et
				if strings.Contains(response.String(), expected) {
					matched = true
					return response.String(), true
				}

				// Hata durumlarını kontrol et
				if strings.Contains(response.String(), "ERROR") ||
					strings.Contains(response.String(), "+CME ERROR") ||
					strings.Contains(response.String(), "+CMS ERROR") {
					return response.String(), false
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}
