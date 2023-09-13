package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

var pool = &Pool{}
var messages = []Message{}

type Message struct {
	Type int    `json:"type"`
	Body string `json:"message"`
	Name string `json:"name"`
}

type Pool struct {
	Register   chan *Client
	Unregister chan *Client
	Clients    map[*Client]bool
	Broadcast  chan Message
}

type Client struct {
	ID   string
	Conn *websocket.Conn
	Pool *Pool
	Name string
}

// Define the template registry struct
type TemplateRegistry struct {
	templates map[string]*template.Template
}

// Implement e.Renderer interface
func (t *TemplateRegistry) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	tmpl, ok := t.templates[name]
	if !ok {
		err := errors.New("Template not found -> " + name)
		return err
	}
	return tmpl.ExecuteTemplate(w, "base.html", data)
}

func main() {

	e := echo.New()
	templates := make(map[string]*template.Template)
	templates["home.html"] = template.Must(template.ParseFiles("public/home.html", "public/base.html"))
	templates["chat.html"] = template.Must(template.ParseFiles("public/chat.html", "public/base.html"))
	e.Renderer = &TemplateRegistry{
		templates: templates,
	}

	pool = NewPool()
	go pool.Start()

	e.GET("/", func(c echo.Context) error {
		e.Logger.Infof("masuk")
		//return c.Render(http.StatusOK, "./public/index.html", nil)
		return c.Render(http.StatusOK, "home.html", nil)
	})
	e.GET("/chat", func(c echo.Context) error {

		//return c.Render(http.StatusOK, "./public/chat2.html", nil)
		return c.Render(http.StatusOK, "chat.html", nil)
	})
	//http.HandleFunc("/chat/ws", wsHandler)
	e.GET("/chat/ws/:name", wsHandler)

	log.Println("server running at port :5555")
	e.Logger.Fatal(e.Start(":5555"))
}

func wsHandler2(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("error when try to upgrade connection :", err.Error())
		return
	}

	defer conn.Close()

	client := Client{
		Conn: conn,
		Pool: pool,
	}

	pool.Register <- &client
	client.Read()
}

func wsHandler(c echo.Context) error {
	conn, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		log.Println("error when try to upgrade connection :", err.Error())
		return nil
	}

	defer conn.Close()

	name := c.Param("name")
	fmt.Printf("new user : %v", name)
	client := Client{
		Conn: conn,
		Pool: pool,
		Name: name,
	}

	pool.Register <- &client
	client.Read()
	return nil
}

func Reader(conn *websocket.Conn) {
	for {
		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			log.Println("error when try to read message :", err.Error())
			return
		}
		log.Println("message type :", msgType)
		log.Println("received message :", string(msg))

		err = conn.WriteMessage(msgType, []byte("receive : "+string(msg)))
		if err != nil {
			log.Println("error when try to write message :", err.Error())
			return
		}
	}
}

func Writer(conn *websocket.Conn) {
	for {
		log.Println("sending ...")
		msgType, msg, err := conn.NextReader()
		if err != nil {
			log.Println("error when try to Next Reader :", err.Error())
			return
		}

		w, err := conn.NextWriter(msgType)
		if err != nil {
			log.Println("error when try to Next Writer :", err.Error())
			return
		}

		_, err = io.Copy(w, msg)
		if err != nil {
			log.Println("error when try to Copy Message :", err.Error())
			return
		}

		defer w.Close()
	}
}

func (c *Client) Read() {
	defer func() {
		c.Pool.Unregister <- c
		c.Conn.Close()
	}()

	for {
		msgType, msg, err := c.Conn.ReadMessage()
		if err != nil {
			log.Println("error when try to read message :", err.Error())
			return
		}

		message := Message{}

		err = json.Unmarshal(msg, &message)
		if err != nil {
			log.Println("error when try to parsing message :", err.Error())
			return
		}
		message.Type = msgType
		messages = append(messages, message)
		c.Pool.Broadcast <- message
		log.Println("message type :", msgType)
		log.Println("received message :", string(msg))
	}

}

func NewPool() *Pool {
	return &Pool{
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
		Clients:    make(map[*Client]bool),
		Broadcast:  make(chan Message),
	}
}

func (p *Pool) Start() {
	for {
		select {
		case client := <-p.Register:
			fmt.Println(p.Clients)
			fmt.Println("ini")
			fmt.Println(client.Name)
			msg := Message{
				Type: 1,
				Body: fmt.Sprintf("%s Joined ...", client.Name),
			}
			// messages = append(messages, msg)
			log.Println("new user joined")
			p.Clients[client] = true
			fmt.Println("Size of Connection Pool: ", len(p.Clients))

			for c, _ := range p.Clients {
				c.Conn.WriteJSON(msg)
			}

			for _, m := range messages {
				client.Conn.WriteJSON(m)
			}

		case client := <-p.Unregister:
			delete(p.Clients, client)
			msg := Message{
				Type: 1,
				Body: "User disconnected ...",
			}
			// messages = append(messages, msg)
			log.Println("user", client, "logout")
			fmt.Println("Size of Connection Pool: ", len(p.Clients))
			for c, _ := range p.Clients {
				c.Conn.WriteJSON(msg)
			}

		case message := <-p.Broadcast:
			fmt.Println("Sending message to all clients in Pool")
			for c, _ := range p.Clients {
				c.Conn.WriteJSON(message)
			}
		}
	}
}
