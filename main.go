package main

import (
	"io"
	"log"
	"net/http"
	"os/exec"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
} // use default options

type FProxy struct {
	conn *websocket.Conn
	mt   int
	buff []byte

	writeChan chan []byte
	closeChan chan []bool

	inPip  io.WriteCloser
	outPip io.ReadCloser
	errPip io.ReadCloser
}

func (r *FProxy) RunIn() {
	defer close(r.closeChan)

	for {
		_, p, err := r.conn.ReadMessage()
		if err != nil {
			log.Printf("read msg err %v", err)
			return
		}

		log.Printf("read msg %v", string(p))

		r.inPip.Write(p)
		r.inPip.Write([]byte("\n"))
	}
}

func (r *FProxy) WaitOut() {
	for {
		buff, status := <-r.writeChan
		if status == false {
			return
		}
		r.conn.WriteMessage(1, buff)
	}
}

func (r *FProxy) RunOut() {
	buff := make([]byte, 6418)

	for {
		n, err := r.outPip.Read(buff)
		if err != nil {
			return
		}
		r.writeChan <- buff[0:n]
		//r.conn.WriteMessage(1, buff[0:n])
	}
}

func (r *FProxy) RunErr() {
	buff := make([]byte, 6418)

	for {
		n, err := r.errPip.Read(buff)
		if err != nil {
			return
		}
		r.writeChan <- buff[0:n]
		//r.conn.WriteMessage(1, buff[0:n])
	}
}

func echo(w http.ResponseWriter, r *http.Request) {
	log.Printf("new echo")
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade: %v", err)
		return
	}

	defer c.Close()

	mt, message, err := c.ReadMessage()
	log.Printf("read first msg %v", string(message))
	err = c.WriteMessage(mt, message)
	if err != nil {
		return
	}

	cmd := exec.Command("gly")
	log.Printf("gly begin")

	inPip, _ := cmd.StdinPipe()
	outPip, _ := cmd.StdoutPipe()
	errPip, _ := cmd.StderrPipe()

	proxy := &FProxy{
		conn:      c,
		inPip:     inPip,
		outPip:    outPip,
		errPip:    errPip,
		writeChan: make(chan []byte, 200),
		closeChan: make(chan []bool),
	}

	go proxy.RunIn()
	go proxy.RunOut()
	go proxy.RunErr()
	go proxy.WaitOut()

	go func() {
		<-proxy.closeChan
		cmd.Process.Kill()
	}()

	cmd.Run()
	log.Printf("gly over")
}

func main() {
	http.Handle("/", http.FileServer(http.Dir("web/")))
	http.HandleFunc("/echo", echo)
	http.ListenAndServe(":9090", nil)
}
