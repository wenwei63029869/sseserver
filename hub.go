package sseserver

import (
	"fmt"
	"strings"
	"time"

	"github.com/azer/debug"
)

// A Hub keeps track of all the active client connections, and handles
// broadcasting messages out to those connections that match the appropriate
// namespace.
//
// The Hub is effectively the "heart" of a Server, but is kept private to hide
// implementation detais.
type Hub struct {
	broadcast   chan SSEMessage      // Inbound messages to propagate out.
	connections map[*connection]bool // Registered connections.
	register    chan *connection     // Register requests from the connections.
	unregister  chan *connection     // Unregister requests from connections.
	shutdown    chan bool            // Internal chan to handle shutdown notification
	sentMsgs    uint64               // Msgs broadcast since startup
	startupTime time.Time            // Time Hub was created
}

func NewHub() *Hub {
	return &Hub{
		broadcast:   make(chan SSEMessage),
		connections: make(map[*connection]bool),
		register:    make(chan *connection),
		unregister:  make(chan *connection),
		shutdown:    make(chan bool),
		startupTime: time.Now(),
	}
}

// Shutdown method for cancellation of Hub run loop.
//
// right now we are only using this in tests, in the future we may want to be
// able to more gracefully shut down a Server in production as well, but...
//
// TODO: need to do some thinking before exposing this interface more broadly.
func (h *Hub) Shutdown() {
	h.shutdown <- true
}

// Start begins the main run loop for a Hub in a background go func.
func (h *Hub) Start() {
	go h.run()
}

func (h *Hub) run() {
	for {
		select {
		case <-h.shutdown:
			debug.Debug(fmt.Sprintf("Hub shutdown requested, cancelling %d connections...", len(h.connections)))
			for c := range h.connections {
				h._shutdownConn(c)
			}
			debug.Debug("...All connections cancelled, shutting down now.")
			return
		case c := <-h.register:
			debug.Debug("new connection being registered for " + c.namespace)
			h.connections[c] = true
		case c := <-h.unregister:
			debug.Debug("connection told us to unregister for " + c.namespace)
			h._unregisterConn(c)
		case msg := <-h.broadcast:
			h.sentMsgs++
			h._broadcastMessage(msg)
		}
	}
}

// internal method, removes that client from the Hub and tells it to shutdown
// _unregister is safe to call multiple times with the same connection
func (h *Hub) _unregisterConn(c *connection) {
	delete(h.connections, c)
}

// internal method, removes that client from the Hub and tells it to shutdown
// must only be called once for a given connection to avoid panic!
func (h *Hub) _shutdownConn(c *connection) {
	// for maximum safety, ALWAYS unregister a connection from the Hub prior to
	// shutting it down, as we want no possibility of a send on closed channel
	// panic.
	h._unregisterConn(c)
	// close the connection's send channel, which will cause it to exit its
	// event loop and return to the HTTP handler.
	close(c.send)
}

// internal method, broadcast a message to all matching clients if this fails
// due to any client having a full send buffer,
func (h *Hub) _broadcastMessage(msg SSEMessage) {
	formattedMsg := msg.sseFormat()
	for c := range h.connections {
		if strings.HasPrefix(msg.Namespace, c.namespace) {
			select {
			case c.send <- formattedMsg:
			default:
				debug.Debug("cant pass to a connection send chan, buffer is full -- kill it with fire")
				h._shutdownConn(c)
				/*
					we are already closing the send channel, in *theory* shouldn't the
					connection clean up? I guess possible it doesnt if its deadlocked or
					something... is it?

					closing the send channel will result in our handleFunc exiting, which Go
					treats as meaning we are done with the connection... but what if it's wedged?

					TODO: investigate using panic in a http.Handler, to absolutely force

					we want to make sure to always close the HTTP connection though,
					so server can never fill up max num of open sockets.
				*/
			}
		}
	}
}
