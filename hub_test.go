package sseserver

import (
	"testing"
	"time"
)

func mockHub(initialConnections int) (h *hub) {
	h = newHub()
	h.Start()
	for i := 0; i < initialConnections; i++ {
		h.register <- mockConn("/test")
	}
	return h
}

func mockConn(namespace string) *connection {
	return &connection{
		send:      make(chan []byte, 256),
		created:   time.Now(),
		namespace: namespace,
	}
}

func mockSinkedHub(initialConnections int) (h *hub) {
	h = newHub()
	h.Start()
	for i := 0; i < initialConnections; i++ {
		h.register <- mockSinkedConn("/test")
	}
	return h
}

// mock a connection that sinks data sent to it
func mockSinkedConn(namespace string) *connection {
	c := &connection{
		send:      make(chan []byte, 256),
		created:   time.Now(),
		namespace: namespace,
	}
	go func() {
		for range c.send {
			// no-op, but will break loop if chan is closed
			// (versus using <-c.send in infinite loop)
		}
	}()
	return c
}

type deliveryCase struct {
	conn     *connection
	expected int
}

func TestBroadcastSingleplex(t *testing.T) {
	h := mockHub(0)
	c1 := mockConn("/foo")
	c2 := mockConn("/bar")
	h.register <- c1
	h.register <- c2

	//broadcast to foo channel
	h.broadcast <- SSEMessage{"", []byte("yo"), "/foo"}
	h.Shutdown() // ensures delivery is finished

	//check for proper delivery
	d := []deliveryCase{
		deliveryCase{c1, 1},
		deliveryCase{c2, 0},
	}
	for _, c := range d {
		if actual := len(c.conn.send); actual != c.expected {
			t.Fatalf("Expected conn to have %d message in queue, actual: %d", c.expected, actual)
		}
	}

}

func TestBroadcastMultiplex(t *testing.T) {
	h := mockHub(0)
	c1 := mockConn("/foo")
	c2 := mockConn("/foo")
	c3 := mockConn("/burrito")
	h.register <- c1
	h.register <- c2
	h.register <- c3

	//broadcast to channels
	h.broadcast <- SSEMessage{"", []byte("yo"), "/foo"}
	h.broadcast <- SSEMessage{"", []byte("yo"), "/foo"}
	h.broadcast <- SSEMessage{"", []byte("yo"), "/bar"}
	h.Shutdown() // ensures delivery is finished

	//check for proper delivery
	d := []deliveryCase{
		deliveryCase{c1, 2},
		deliveryCase{c2, 2},
		deliveryCase{c3, 0},
	}
	for _, c := range d {
		if actual := len(c.conn.send); actual != c.expected {
			t.Fatalf("Expected conn to have %d message in queue, actual: %d", c.expected, actual)
		}
	}
}

func TestBroadcastWildcards(t *testing.T) {
	h := mockHub(0)

	cDogs := mockConn("/pets/dogs")
	cCats := mockConn("/pets/cats")
	cWild := mockConn("/pets")
	cOther := mockConn("/kids")

	h.register <- cDogs
	h.register <- cCats
	h.register <- cWild
	h.register <- cOther

	//broadcast to channels
	h.broadcast <- SSEMessage{"", []byte("woof"), "/pets/dogs"}
	h.broadcast <- SSEMessage{"", []byte("meow"), "/pets/cats"}
	h.broadcast <- SSEMessage{"", []byte("wahh"), "/kids"}
	h.Shutdown() // ensures delivery is finished

	//check for proper delivery
	d := []deliveryCase{
		deliveryCase{cDogs, 1},
		deliveryCase{cCats, 1},
		deliveryCase{cWild, 2},
		deliveryCase{cOther, 1},
	}
	for _, c := range d {
		if actual := len(c.conn.send); actual != c.expected {
			t.Fatalf("Expected conn to have %d message in queue, actual: %d", c.expected, actual)
		}
	}
}

func benchmarkBroadcast(conns int, b *testing.B) {
	b.StopTimer()
	h := mockSinkedHub(conns)
	b.StartTimer()
	for n := 0; n < b.N; n++ {
		h.broadcast <- SSEMessage{"", []byte("foo bar woo"), "/test"}
		h.broadcast <- SSEMessage{"event-foo", []byte("foo bar woo"), "/test"}
	}
	b.StopTimer()
	h.Shutdown()
}

func BenchmarkBroadcast1(b *testing.B)    { benchmarkBroadcast(1, b) }
func BenchmarkBroadcast10(b *testing.B)   { benchmarkBroadcast(10, b) }
func BenchmarkBroadcast100(b *testing.B)  { benchmarkBroadcast(100, b) }
func BenchmarkBroadcast500(b *testing.B)  { benchmarkBroadcast(500, b) }
func BenchmarkBroadcast1000(b *testing.B) { benchmarkBroadcast(1000, b) }
