package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	gmap "server/pkg/google_map"

	"github.com/google/uuid"
	supa "github.com/nedpals/supabase-go"
	"golang.org/x/time/rate"
	"googlemaps.github.io/maps"
	"nhooyr.io/websocket"
)

var ROLES = [3]string{"admin", "client", "taxi"}

type Server struct {
	subscriberMessageBuffer int

	publishLimiter *rate.Limiter

	logf func(f string, v ...interface{})

	serveMux    http.ServeMux
	supabaseCli *supa.Client
	mapsCli     *maps.Client

	connectionsMu sync.Mutex
	connections   map[*websocket.Conn]uuid.UUID
	entryCha      chan string

	taxiSubs   map[uuid.UUID]*Subscriber
	clientSubs map[uuid.UUID]*Subscriber
	adminSubs  map[uuid.UUID]*Subscriber

	taxiPositions map[uuid.UUID]gmap.Location
}

type Subscriber struct {
	id        uuid.UUID
	msgs      chan string
	closeSlow func()
	protocol  string
	position  gmap.Location
	conn      *websocket.Conn
}

func (s *Server) initSupabaseCli() {
	supabaseUrl := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_KEY")
	s.supabaseCli = supa.CreateClient(supabaseUrl, supabaseKey)
}

func (s *Server) initMapCli() {
	googleKey := os.Getenv("GOOGLE_MAPS_API_KEY")
	client, err := maps.NewClient(maps.WithAPIKey(googleKey))
	if err != nil {
		log.Fatal(err)
	}
	s.mapsCli = client
}

func NewServer() *Server {
	s := &Server{
		subscriberMessageBuffer: 16,
		logf:                    log.Printf,
		connections:             make(map[*websocket.Conn]uuid.UUID),
		publishLimiter:          rate.NewLimiter(rate.Every(time.Millisecond*100), 8),
		entryCha:                make(chan string),

		taxiSubs:   make(map[uuid.UUID]*Subscriber),
		clientSubs: make(map[uuid.UUID]*Subscriber),
		adminSubs:  make(map[uuid.UUID]*Subscriber),

		taxiPositions: make(map[uuid.UUID]gmap.Location),
	}

	s.initSupabaseCli()
	s.initMapCli()
	s.serveMux.Handle("/", http.FileServer(http.Dir("./assets/simulator")))
	// s.serveMux.Handle("/", http.FileServer(http.Dir("./assets/chat")))
	s.serveMux.HandleFunc("/subscribe", s.subscribeHandler)
	s.serveMux.HandleFunc("/profile", s.getProfileHandler)
	s.serveMux.HandleFunc("/taxis", s.getProfilesHandler)
	s.serveMux.HandleFunc("/route", s.getRouteHandler)

	go s.broadcastTaxis()
	// go s.printSubsReads()

	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.serveMux.ServeHTTP(w, r)
}

func (server *Server) getProfileHandler(w http.ResponseWriter, r *http.Request) {
	querys := r.URL.Query()
	idQ := querys.Get("id")

	id, err := uuid.Parse(idQ)
	if err != nil {
		server.logf("invalid UUID: %v", err)
		return
	}

	var results map[string]interface{}
	err = server.supabaseCli.DB.From("profiles").Select("*").Single().Eq("id", id.String()).Execute(&results)
	if err != nil {
		panic(err)
	}
	server.logf("profile", results)

	const profileTemplate = `
		{
			"id": "%v",
			"slug": "%v",
			"role": "%v",
			"username": "%v",
			"full_name": "%v",
			"avatar_url": "%v",
			"cover_img_url": "%v",
			"phone": "%v"	
		}
	`
	w.Write([]byte(fmt.Sprintf(profileTemplate, results["id"], results["slug"], results["role"], results["username"], results["full_name"], results["avatar_url"], results["cover_img_url"], results["phone"])))
}

func (server *Server) getRouteHandler(w http.ResponseWriter, r *http.Request) {
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	if from == "" || to == "" {
		http.Error(w, "Missing 'from' or 'to' parameter", http.StatusBadRequest)
		return
	}

	route := gmap.GetRoute(server.mapsCli, from, to)

	// Encode the response data as JSON
	responseJSON, err := json.Marshal(route)
	if err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
	// Set the Content-Type header to application/json
	w.Header().Set("Content-Type", "application/json")

	// Write the response back to the client
	w.Write(responseJSON)
	//json.NewEncoder(w).Encode(route)
}

func (server *Server) getProfilesHandler(w http.ResponseWriter, r *http.Request) {
	querys := r.URL.Query()
	idsQ := querys.Get("ids")
	ids := strings.Split(idsQ, ",")
	fmt.Printf("ids: %+v\n", ids)

	var results []map[string]interface{}
	err := server.supabaseCli.DB.From("profiles").Select("*").In("id", ids).Execute(&results)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Profiles: %+v\n", results)

	const profileTemplate = `{
		"id": "%v",
		"slug": "%v",
		"role": "%v",
		"username": "%v",
		"full_name": "%v",
		"avatar_url": "%v",
		"cover_img_url": "%v",
		"phone": "%v"	
    }`

	profiles := make([]string, len(results))
	for i, result := range results {
		profiles[i] = fmt.Sprintf(profileTemplate, result["id"], result["slug"], result["role"], result["username"], result["full_name"], result["avatar_url"], result["cover_img_url"], result["phone"])
	}

	w.Write([]byte(fmt.Sprintf("[%s]", strings.Join(profiles, ","))))
}

func (server *Server) subscribeHandler(w http.ResponseWriter, r *http.Request) {
	querys := r.URL.Query()
	latQ, lonQ, idQ, headerQ := querys.Get("lat"), querys.Get("lon"), querys.Get("id"), querys.Get("head")

	lat, err := strconv.ParseFloat(latQ, 64)
	if err != nil {
		server.logf("invalid latitude: %v", err)
		return
	}

	lon, err := strconv.ParseFloat(lonQ, 64)
	if err != nil {
		server.logf("invalid longitude: %v", err)
		return
	}

	id, err := uuid.Parse(idQ)
	if err != nil {
		server.logf("invalid UUID: %v", err, id)
		return
	}

	header, err := strconv.ParseInt(strings.TrimSpace(headerQ), 10, 16)
	if err != nil {
		server.logf("invalid header: %v", err)
		return
	}

	coord := gmap.Location{
		Lat:  lat,
		Lon:  lon,
		Head: int16(header),
	}

	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols: []string{"map-admin", "map-client", "map-taxi"},
	})
	if err != nil {
		server.logf("%v", err)
		return
	}

	defer func() {
		ws.Close(websocket.StatusInternalError, "the sky is falling")
		server.deleteSubById(id)
	}()

	protocol := ws.Subprotocol()
	fmt.Printf("adding sub with protocols: %v and id: %v\n ", protocol, id.String())

	server.connectionsMu.Lock()
	server.connections[ws] = id
	sub := &Subscriber{
		msgs:      make(chan string, server.subscriberMessageBuffer),
		closeSlow: func() { ws.Close(websocket.StatusPolicyViolation, "slow subscriber") },
		protocol:  protocol,
		id:        id,
		conn:      ws,
		position:  coord,
	}
	if protocol == "map-admin" {
		if server.adminSubs[id] != nil {
			server.logf("this admin is already subscribed")
		}
		server.adminSubs[id] = sub
	} else if protocol == "map-taxi" {
		if server.taxiSubs[id] != nil {
			server.logf("this taxi is already subscribed")
		}
		server.taxiSubs[id] = sub
	} else {
		if server.clientSubs[id] != nil {
			server.logf("this client is already subscribed")
		}
		server.clientSubs[id] = sub
	}
	server.connectionsMu.Unlock()

	l := rate.NewLimiter(rate.Every(time.Millisecond*100), 10)
	for {
		select {
		case <-r.Context().Done():
			return
		default:
			err = subReader(r.Context(), server, ws, l, id)
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
				return
			}
			if err != nil {
				server.logf("failed to echo with %v: %v", r.RemoteAddr, err)
				return
			}
		}
	}
}

func (server *Server) deleteSubById(id uuid.UUID) {
	server.connectionsMu.Lock()
	defer server.connectionsMu.Unlock()

	for ws, sub := range server.connections {
		if id == sub {
			protocol := ws.Subprotocol()
			if protocol == "map-taxi" {
				delete(server.taxiSubs, id)
				delete(server.taxiPositions, id)
			}
			if protocol == "map-client" {
				delete(server.clientSubs, id)
			}
			if protocol == "map-admin" {
				delete(server.adminSubs, id)
			}
			delete(server.connections, ws)
			return
		}
	}
}

func subReader(ctx context.Context, server *Server, ws *websocket.Conn, l *rate.Limiter, id uuid.UUID) error {
	err := l.Wait(ctx)
	if err != nil {
		return err
	}

	_, r, err := ws.Reader(ctx)
	if err != nil {
		return err
	}

	msg, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("failed to read message: %w", err)
	}
	msgString := string(msg)

	protocols := ws.Subprotocol()
	if strings.HasPrefix(msgString, "pos") {
		newPosition := strings.Split(msgString, "#")[1]
		fmt.Printf("Position recieved: %s \n", newPosition)
		server.connectionsMu.Lock()
		loc, err := gmap.ParseLocation(newPosition)
		if err != nil {
			return fmt.Errorf("failed to parse location: %v", err)
		}
		if protocols == "map-taxi" {
			server.taxiPositions[id] = loc
		}
		server.connectionsMu.Unlock()
	}

	// server.entryCha <- msgString

	return err
}

/* func (server *Server) printSubsReads() {
	for {
		msg := <-server.entryCha
		fmt.Println("Recieved string: ", msg)
	}
} */

func (server *Server) broadcastTaxis() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		server.connectionsMu.Lock()

		if len(server.taxiPositions) != 0 {
			fmt.Printf("server.taxiPositions: %+v\n", server.taxiPositions)
			fmt.Printf("server.taxiSubs: %+v\n", server.taxiSubs)

			var taxiPositionSlice []string
			for id, position := range server.taxiPositions {
				posAndId := position.String() + "&" + id.String()
				taxiPositionSlice = append(taxiPositionSlice, posAndId)
			}
			taxiPositionString := strings.Join(taxiPositionSlice, "$")
			fmt.Printf("sending message to clients: %+v\n", server.clientSubs)
			for _, sub := range server.clientSubs {
				err := sub.conn.Write(context.Background(), websocket.MessageText, []byte("taxis-"+taxiPositionString))
				if err != nil {
					server.logf("failed to send taxi positions to client connection: %v", err)
					delete(server.clientSubs, sub.id)
				}
			}
		}

		server.connectionsMu.Unlock()
	}
}
