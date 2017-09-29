package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"regexp"
	"strings"

	"github.com/gorilla/websocket"
)

var (
	index_html = `
<html>
    <head>
        <script src="https://cdn.bootcss.com/jquery/3.2.1/jquery.min.js"></script>
        <script src="https://cdnjs.cloudflare.com/ajax/libs/jquery.terminal/1.8.0/js/jquery.terminal.min.js"></script>
        <link href="https://cdnjs.cloudflare.com/ajax/libs/jquery.terminal/1.8.0/css/jquery.terminal.min.css" rel="stylesheet"/>

        <title>tortex</title>

        <script type="text/javascript">

         var ws = new WebSocket('ws://%s:%s/echo');

         ws.onopen = function(evt) {
             console.log('Connection open ...');
         };
         var echo_obj

         ws.onmessage = function(evt) {
             console.log('Received Message: ' + evt.data);
             if(echo_obj) {
                 echo_obj.echo(String(evt.data))
             }
         };

         ws.onclose = function(evt) {
             console.log('Connection closed.');
             alert("connection closed. please F5")
         };


         $(document).ready(function() {
             jQuery(function($, undefined){
                 $('#tortex').terminal(function(command) {
                     echo_obj = this
                     //this.echo(command)
                     ws.send(command)
                 }, {
                     greetings: '',
                     name: 'tortex',
                     height: 768,
                     width: 1024,
                     prompt: 'tortex> '
                 });
             });

         });
        </script>
    </head>
    <body>
        <div name="tortex" id="tortex"></div>
    </body>
</html>

`

	IP   = ""
	PORT = ""
	CMD  = ""
)

func GetIndexHtml(ip string, port string) []byte {
	s := fmt.Sprintf(index_html, ip, port)
	return []byte(s)
}

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

		p1 := strings.Replace(string(p), "\n", " ", -1)
		log.Printf("read msg %v", string(p1))

		r.inPip.Write([]byte(p1))
		r.inPip.Write([]byte("\n"))
	}
}

func (r *FProxy) WaitOut() {
	for {
		buff, status := <-r.writeChan
		if status == false {
			return
		}

		reg := regexp.MustCompile(`\[\d+m`)
		buff1 := reg.ReplaceAllString(string(buff), "")

		err := r.conn.WriteMessage(1, []byte(buff1))
		if err != nil {
			log.Printf("WriteMessageError %v", err)
		} else {
			//log.Printf("WriteMessage %v", string(buff))
		}
	}
}

func (r *FProxy) RunOut() {

	for {
		buff := make([]byte, 6418)
		n, err := r.outPip.Read(buff)
		if err != nil {
			return
		}

		if n < 0 {
			continue
		}
		//log.Print("read out:", string(buff[0:n]))
		r.writeChan <- buff[0:n]
	}
}

func (r *FProxy) RunErr() {
	for {
		buff := make([]byte, 6418)
		n, err := r.errPip.Read(buff)
		if err != nil {
			return
		}

		if n < 0 {
			continue
		}

		//log.Print("read err:", string(buff[0:n]))
		r.writeChan <- buff[0:n]
	}
}

func index(w http.ResponseWriter, r *http.Request) {
	w.Write(GetIndexHtml(IP, PORT))
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

	cmd := exec.Command(CMD)
	log.Printf("tortex begin")

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
	log.Printf("tortex over")
}

func main() {
	//http.Handle("/", http.FileServer(http.Dir("web/")))
	ip := (flag.String("ip", "127.0.0.1", "外网IP"))
	port := (flag.String("port", "9090", "外网PORT"))
	cmd := (flag.String("cmd", "ls", "托管的cmd"))

	flag.Parse()

	IP = *ip
	PORT = *port
	CMD = *cmd

	log.Print(IP, " ", PORT, " ", CMD)

	http.HandleFunc("/", index)
	http.HandleFunc("/echo", echo)
	http.ListenAndServe(":9090", nil)
}
