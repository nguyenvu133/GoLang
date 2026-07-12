package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
)

const defaultPort = ":9000"

type InventoryItem struct {
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
}

type PlayerState struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Username string  `json:"username,omitempty"`
	Password string  `json:"password,omitempty"`
	RoomID   string  `json:"room_id,omitempty"`
	MatchID  string  `json:"match_id,omitempty"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Z        float64 `json:"z"`
	RotY     float64 `json:"roty"`
	HP       float64 `json:"hp"`
}

type Packet struct {
	Type       string          `json:"type"`
	Player     PlayerState     `json:"player,omitempty"`
	Players    []PlayerState   `json:"players,omitempty"`
	Inventory  []InventoryItem `json:"inventory,omitempty"`
	AttackerID string          `json:"attacker_id,omitempty"`
	TargetID   string          `json:"target_id,omitempty"`
	Damage     float64         `json:"damage,omitempty"`
	Error      string          `json:"error,omitempty"`
}

type Client struct {
	Conn  net.Conn
	State PlayerState
}

var (
	clients      = make(map[net.Conn]*Client)
	mu           sync.RWMutex
	nextClientID int
	store        *PostgresStore
)

func loadEnv() (string, string) {
	databaseURL := os.Getenv("DATABASE_URL")
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}
	if databaseURL == "" {
		if _, err := os.Stat(".env"); err == nil {
			fileBytes, err := os.ReadFile(".env")
			if err == nil {
				for _, line := range strings.Split(string(fileBytes), "\n") {
					line = strings.TrimSpace(line)
					if line == "" || strings.HasPrefix(line, "#") {
						continue
					}
					parts := strings.SplitN(line, "=", 2)
					if len(parts) == 2 {
						key := strings.TrimSpace(parts[0])
						value := strings.TrimSpace(parts[1])
						if key == "DATABASE_URL" {
							databaseURL = value
						}
						if key == "PORT" {
							port = value
						}
					}
				}
			}
		}
	}
	if databaseURL == "" {
		databaseURL = "postgres://postgres:postgres@localhost:5432/gamegodot?sslmode=disable"
	}
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}
	return databaseURL, port
}

func main() {
	databaseURL, port := loadEnv()

	var err error
	store, err = NewPostgresStore(databaseURL)
	if err != nil {
		fmt.Println("Khong the ket noi Postgres:", err)
		return
	}

	listener, err := net.Listen("tcp", port)
	if err != nil {
		fmt.Println("Khong the mo port", port, ":", err)
		return
	}
	defer listener.Close()

	fmt.Printf("Server Go multiplayer da chay tren PORT=%s bind=%s\n", strings.TrimPrefix(port, ":"), port)

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Accept loi:", err)
			continue
		}

		mu.Lock()
		nextClientID++
		client := &Client{
			Conn: conn,
			State: PlayerState{
				ID:   fmt.Sprintf("player_%d", nextClientID),
				Name: "Player",
				Y:    1.2,
			},
		}
		clients[conn] = client
		mu.Unlock()

		fmt.Println("Client ket noi:", conn.RemoteAddr().String())
		go handleClient(client)
	}
}

func handleClient(client *Client) {
	defer func() {
		mu.Lock()
		delete(clients, client.Conn)
		mu.Unlock()
		broadcastLeave(client.State.ID)
		client.Conn.Close()
	}()

	reader := bufio.NewReader(client.Conn)

	if err := sendPacket(client.Conn, Packet{Type: "welcome", Player: client.State}); err != nil {
		fmt.Println("Gui welcome loi:", err)
		return
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Client ngat ket noi:", client.Conn.RemoteAddr().String(), err)
			return
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(strings.ToUpper(line), "GET ") || strings.HasPrefix(strings.ToUpper(line), "HEAD ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 && (parts[1] == "/health" || parts[1] == "/") {
				_ = sendHealthResponse(client.Conn)
				return
			}
		}

		fmt.Printf("Nhan tu %s: %s\n", client.Conn.RemoteAddr().String(), line)

		var packet Packet
		if err := json.Unmarshal([]byte(line), &packet); err != nil {
			fmt.Println("JSON loi:", err)
			continue
		}

		switch packet.Type {
		case "login":
			state, inventory, err := store.LoginOrCreateAccount(packet.Player.Username, packet.Player.Password)
			if err != nil {
				_ = sendPacket(client.Conn, Packet{Type: "error", Error: err.Error()})
				continue
			}
			client.State = state
			if packet.Player.RoomID != "" {
				client.State.RoomID = packet.Player.RoomID
			}
			if packet.Player.MatchID != "" {
				client.State.MatchID = packet.Player.MatchID
			}
			if err := store.UpsertPlayerState(client.State); err != nil {
				fmt.Println("Luu player state loi:", err)
			}
			if err := sendPacket(client.Conn, Packet{Type: "welcome", Player: client.State, Inventory: inventory}); err != nil {
				fmt.Println("Gui welcome sau login loi:", err)
				return
			}
			if err := sendSnapshot(client.Conn, client.State.RoomID, client.State.MatchID); err != nil {
				fmt.Println("Gui snapshot sau login loi:", err)
				return
			}
			broadcastJoin(client.State)
		case "join":
			if packet.Player.Name != "" {
				client.State.Name = packet.Player.Name
			}
			if packet.Player.RoomID != "" {
				client.State.RoomID = packet.Player.RoomID
			}
			if packet.Player.MatchID != "" {
				client.State.MatchID = packet.Player.MatchID
			}
			_ = store.UpsertPlayerState(client.State)
			broadcastJoin(client.State)
		case "state":
			client.State.X = packet.Player.X
			client.State.Y = packet.Player.Y
			client.State.Z = packet.Player.Z
			client.State.RotY = packet.Player.RotY
			client.State.HP = packet.Player.HP
			if packet.Player.Name != "" {
				client.State.Name = packet.Player.Name
			}
			if packet.Player.ID != "" {
				client.State.ID = packet.Player.ID
			}
			if packet.Player.RoomID != "" {
				client.State.RoomID = packet.Player.RoomID
			}
			if packet.Player.MatchID != "" {
				client.State.MatchID = packet.Player.MatchID
			}
			_ = store.UpsertPlayerState(client.State)
			broadcastState(client.State)
		case "attack":
			if packet.AttackerID == "" || packet.TargetID == "" {
				continue
			}
			_ = store.ApplyDamage(packet.TargetID, packet.Damage)
			broadcastDamage(packet.AttackerID, packet.TargetID, packet.Damage)
		}
	}
}

func sendHealthResponse(conn net.Conn) error {
	body := `{"status":"ok"}`
	response := "HTTP/1.1 200 OK\r\n" +
		"Content-Type: application/json\r\n" +
		"Content-Length: " + strconv.Itoa(len(body)) + "\r\n" +
		"Connection: close\r\n\r\n" +
		body
	_, err := conn.Write([]byte(response))
	return err
}

func broadcastJoin(state PlayerState) {
	mu.RLock()
	defer mu.RUnlock()

	for _, c := range clients {
		if c == nil || c.Conn == nil {
			continue
		}
		if err := sendPacket(c.Conn, Packet{Type: "join", Player: state}); err != nil {
			fmt.Println("Gui join loi:", err)
		}
	}
}

func broadcastState(state PlayerState) {
	mu.RLock()
	defer mu.RUnlock()

	for _, c := range clients {
		if c == nil || c.Conn == nil {
			continue
		}
		if err := sendPacket(c.Conn, Packet{Type: "state", Player: state}); err != nil {
			fmt.Println("Gui state loi:", err)
		}
	}
}

func broadcastLeave(playerID string) {
	mu.RLock()
	defer mu.RUnlock()

	for _, c := range clients {
		if c == nil || c.Conn == nil {
			continue
		}
		if err := sendPacket(c.Conn, Packet{Type: "leave", Player: PlayerState{ID: playerID}}); err != nil {
			fmt.Println("Gui leave loi:", err)
		}
	}
}

func broadcastDamage(attackerID string, targetID string, damage float64) {
	mu.RLock()
	defer mu.RUnlock()

	for _, c := range clients {
		if c == nil || c.Conn == nil {
			continue
		}
		if err := sendPacket(c.Conn, Packet{
			Type:       "damage",
			AttackerID: attackerID,
			TargetID:   targetID,
			Damage:     damage,
		}); err != nil {
			fmt.Println("Gui damage loi:", err)
		}
	}
}

func sendSnapshot(conn net.Conn, roomID string, matchID string) error {
	players, err := store.Snapshot(roomID, matchID)
	if err != nil {
		return err
	}
	return sendPacket(conn, Packet{Type: "snapshot", Players: players})
}

func sendPacket(conn net.Conn, packet Packet) error {
	data, err := json.Marshal(packet)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = conn.Write(data)
	return err
}
