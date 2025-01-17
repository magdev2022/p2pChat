package main

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

type User struct {
	Addr string
	Name string
}

var (
	users       = make(map[string]User)
	mutex       = &sync.Mutex{}
	chatLogView *widget.List
	input       *widget.Entry
	usersList   *widget.List
	username    = "User-" + generateRandomID()
	broadcast   = "192.168.10.255:9999"
	localPort   = ":9999"
	tcpPort     = ":8080"
	selectedIP  string
	myApp       fyne.App
	myWindow    fyne.Window

	chatList binding.StringList
)

func main() {
	myApp = app.NewWithID("com.wimbot.app")
	myWindow = myApp.NewWindow("P2PChat")

	//init chat history
	chatList = binding.NewStringList()

	input = widget.NewEntry()
	input.SetPlaceHolder("Type your message here...")

	chatLogView = widget.NewListWithData(chatList,
		func() fyne.CanvasObject {
			return widget.NewLabel("template")
		},
		func(i binding.DataItem, o fyne.CanvasObject) {
			o.(*widget.Label).Bind(i.(binding.String))
		})

	chatLogContainer := container.NewGridWrap(fyne.NewSize(300, 300), chatLogView)

	usersList = widget.NewList(
		func() int {
			return len(users)
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("User")
		},
		func(i int, item fyne.CanvasObject) {
			mutex.Lock()
			defer mutex.Unlock()
			keys := make([]string, 0, len(users))
			for k := range users {
				keys = append(keys, k)
			}
			user := users[keys[i]]
			item.(*widget.Label).SetText(user.Name + " (" + user.Addr + ")")
		})

	usersList.OnSelected = func(id widget.ListItemID) {
		mutex.Lock()
		defer mutex.Unlock()
		keys := make([]string, 0, len(users))
		for k := range users {
			keys = append(keys, k)
		}
		selectedIP = keys[id]
	}

	sendButton := widget.NewButton("Send", func() {
		message := input.Text
		if message != "" && selectedIP != "" {
			sendMessage(selectedIP, message)
			input.SetText("")
		} else {
			dialog.ShowInformation("Error", "Please select a user and enter a message", myWindow)
		}
	})

	userListContainer := container.NewScroll(usersList)
	userListContainer.SetMinSize(fyne.NewSize(300, 200))

	if desk, ok := myApp.(desktop.App); ok {
		m := fyne.NewMenu("MyApp",
			fyne.NewMenuItem("Show", func() {
				myWindow.Show()
			}))
		desk.SetSystemTrayMenu(m)

		iconData, err := fyne.LoadResourceFromPath("icon.ico")
		if err != nil {
			fmt.Println("Error loading icon:", err)
			return
		}
		desk.SetSystemTrayIcon(iconData)
	}

	content := container.NewVBox(
		userListContainer,
		chatLogContainer,
		input,
		sendButton,
	)
	myWindow.SetContent(content)
	myWindow.SetCloseIntercept(func() {
		myWindow.Hide()
	})

	// Start UDP broadcasting and listening
	go startBroadcasting()
	go listenForBroadcasts()

	// Start TCP server for direct messaging
	go startTCPServer()

	myApp.Run()
}

// Broadcast presence to the network
func startBroadcasting() {
	addr, err := net.ResolveUDPAddr("udp", broadcast)
	if err != nil {
		fmt.Println("Error resolving UDP address:", err)
		return
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		fmt.Println("Error dialing UDP:", err)
		return
	}
	defer conn.Close()

	for {
		message := username + "@" + tcpPort
		_, err := conn.Write([]byte(message))
		if err != nil {
			fmt.Println("Error broadcasting:", err)
		}
		time.Sleep(5 * time.Second)
	}
}

// Listen for broadcasts from other users
func listenForBroadcasts() {
	addr, err := net.ResolveUDPAddr("udp", localPort)
	if err != nil {
		fmt.Println("Error resolving UDP address:", err)
		return
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		fmt.Println("Error listening on UDP:", err)
		return
	}
	defer conn.Close()

	buffer := make([]byte, 1024)
	for {
		n, remoteAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			fmt.Println("Error reading from UDP:", err)
			continue
		}

		data := strings.Split(string(buffer[:n]), "@")
		if len(data) != 2 {
			continue
		}

		name := data[0]
		tcpAddress := remoteAddr.IP.String() + data[1]

		mutex.Lock()
		users[tcpAddress] = User{
			Addr: tcpAddress,
			Name: name,
		}
		mutex.Unlock()

		usersList.Refresh()
	}
}

// Start TCP server for receiving direct messages
func startTCPServer() {
	listener, err := net.Listen("tcp", tcpPort)
	if err != nil {
		fmt.Println("Error starting TCP server:", err)
		return
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting connection:", err)
			continue
		}
		go handleTCPConnection(conn)
	}
}

// Handle incoming TCP connections
func handleTCPConnection(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	message, err := reader.ReadString('\n')
	if err != nil {
		fmt.Println("Error reading from connection:", err)
		return
	}
	displayMessage(message)
	// Show notification for incoming message
	myApp.SendNotification(&fyne.Notification{
		Title:   "New Message",
		Content: message,
	})
}

// Send message to selected user
func sendMessage(addr, message string) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		fmt.Println("Error connecting to peer:", err)
		return
	}
	defer conn.Close()

	writer := bufio.NewWriter(conn)
	writer.WriteString(message + "\n")
	writer.Flush()

	displayMessage("Me: " + message)
}

// Display a message in the chat log
func displayMessage(message string) {
	length := chatList.Length()
	if length >= 100 {
		// Remove the first element by setting a new slice without it
		existing, _ := chatList.Get()
		chatList.Set(existing[1:])
	}
	chatList.Append(message)
}

// Generate a random ID for the username
func generateRandomID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
