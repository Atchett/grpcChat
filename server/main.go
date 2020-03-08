package main

import (
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/Atchett/grpcChat/chat"
	"google.golang.org/grpc"
)

// Connection - connection struct
type Connection struct {
	conn chat.Chat_ChatServer
	send chan *chat.ChatMessage
	quit chan struct{}
}

// NewConnection - creates a new pointer to a connection
func NewConnection(conn chat.Chat_ChatServer) *Connection {
	c := &Connection{
		conn: conn,
		send: make(chan *chat.ChatMessage),
		quit: make(chan struct{}),
	}
	go c.start()
	return c
}

// Close - closes all channels causing the connection to close
func (c *Connection) Close() error {
	close(c.quit)
	close(c.send)
	return nil
}

// Send - sends message to a channel
func (c *Connection) Send(msg *chat.ChatMessage) {
	defer func() {
		// Ignore any errors about sending on a closed channel
		recover()
	}()
	c.send <- msg
}

func (c *Connection) start() {
	running := true
	for running {
		select {
		case msg := <-c.send:
			c.conn.Send(msg) // Ignoreing the error
		case <-c.quit:
			running = false
		}
	}
}

// GetMessages - gets the messages from the channel
func (c *Connection) GetMessages(broadcast chan<- *chat.ChatMessage) error {
	for {
		msg, err := c.conn.Recv()
		if err == io.EOF {
			c.Close()
			return nil
		} else if err != nil {
			c.Close()
			return err
		}
		go func(msg *chat.ChatMessage) {
			select {
			case broadcast <- msg:
			case <-c.quit:
			}
		}(msg)
	}
}

// ChatServer - struct for a chat server
type ChatServer struct {
	broadcast   chan *chat.ChatMessage
	quit        chan struct{}
	connections []*Connection
	connLock    sync.Mutex
}

// NewChatServer - create a new chat server
func NewChatServer() *ChatServer {
	srv := &ChatServer{
		broadcast: make(chan *chat.ChatMessage),
		quit:      make(chan struct{}),
	}
	go srv.start()
	return srv
}

// Close - closes the quit channel
func (c *ChatServer) Close() error {
	close(c.quit)
	return nil
}

func (c *ChatServer) start() {
	running := true
	for running {
		select {
		case msg := <-c.broadcast:
			c.connLock.Lock()
			for _, v := range c.connections {
				go v.Send(msg)
			}
			c.connLock.Unlock()
		case <-c.quit:
			running = false
		}
	}
}

// Chat - implements the prototype
func (c *ChatServer) Chat(stream chat.Chat_ChatServer) error {
	conn := NewConnection(stream)
	c.connLock.Lock()
	c.connections = append(c.connections, conn)
	c.connLock.Unlock()

	// once returned finished
	err := conn.GetMessages(c.broadcast)

	c.connLock.Lock()
	// look through list of connections
	for i, v := range c.connections {
		if v == conn {
			// remove the connection if found
			c.connections = append(c.connections[:i], c.connections[i+1:]...)
		}
	}
	c.connLock.Unlock()
	return err
}

func main() {
	lst, err := net.Listen("tcp", ":8000")
	if err != nil {
		panic(err)
	}

	s := grpc.NewServer()
	srv := NewChatServer()
	chat.RegisterChatServer(s, srv)

	fmt.Println("Now serving on port 8000")
	err = s.Serve(lst)
	if err != nil {
		panic(err)
	}
}
