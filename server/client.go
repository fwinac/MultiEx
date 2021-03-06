package server

import (
	"MultiEx/msg"
	"MultiEx/util"
	"io"
	"net"
	"sync"
	"time"
)

// Client represent a MultiEx client in server.
type Client struct {
	ID        string
	Conn      Conn
	Ports     []string
	InUsePort util.Count
	Listeners []net.Listener
	Proxies   chan Conn
	LastPing  *time.Time
}

func (client *Client) Close() {
	defer func() {
		if r := recover(); r != nil {
			client.Conn.Warn("when close a client, unexpected error: %v", r)
		}
	}()
	client.Conn.Info("client closing...")
	client.LastPing = nil
	client.Conn.Close()
	// close channel,dont accept new proxy
	close(client.Proxies)
	for c := range client.Proxies {
		c.Close()
	}
	for _, l := range client.Listeners {
		l.Close()
	}
	client.Conn.Info("close finished")
}

func (client *Client) AcceptCmd(reg *ClientRegistry) {
	go func() {
		for {
			ticker := time.Tick(time.Second * 30)
			select {
			case <-ticker:
				if client.LastPing == nil {
					client.Conn.Error("client already closed, stop ticker to check ping")
					return
				}
				if time.Now().Sub(*client.LastPing) > time.Minute {
					client.Conn.Warn("client not ping too long")
					client.Close()
					reg.Unregister(client.ID)
					return
				}
			}
		}
	}()
	for {
		m, e,_ := msg.ReadMsg(client.Conn)
		if e != nil {
			client.Conn.Warn("%s when read cmd from client", e)
			client.Close()
			break
		}
		switch m.(type) {
		case *msg.Ping:
			now := time.Now()
			client.LastPing = &now
			msg.WriteMsg(client.Conn, msg.Pong{})
		}
	}
}

func (client *Client) StartListener(wg *sync.WaitGroup) {
	wg.Add(len(client.Ports))
	for _, p := range client.Ports {
		go func(port string) {
			l, e := net.Listen("tcp", ":"+port)
			if e != nil {
				client.InUsePort.Inc()
				wg.Done()
				client.Conn.Warn("port %s is in use", port)
				msg.WriteMsg(client.Conn, msg.PortInUse{Port: port})
				return
			}
			wg.Done()
			client.Listeners = append(client.Listeners, l)
			for {
				c, e := l.Accept()
				if e != nil {
					client.Conn.Warn("listener at %s closed", port)
					break
				}
				client.Conn.Info("remote host:%s is coming", c.RemoteAddr().String())
				go handlePublic(port, c, client)
			}
		}(p)
	}
}

func handlePublic(port string, c net.Conn, client *Client) {
	defer func() {
		if r := recover(); r != nil {
			client.Conn.Error("fatal when handle public conn:%v", r)
		}
	}()

	var proxy Conn
	for i := 0; i < 5 && proxy == nil; i++ {
		client.Conn.Info("try to get proxy connection...")
		select {
		case proxy = <-client.Proxies:
			// A new proxy
			msg.WriteMsg(client.Conn, msg.NewProxy{})
			e := msg.WriteMsg(proxy, msg.ForwardInfo{Port: port})
			if e != nil {
				proxy = nil
			}
		default:
			client.Conn.Info("no proxy available, send NewProxy")
			msg.WriteMsg(client.Conn, msg.NewProxy{})
			select {
			case proxy = <-client.Proxies:
				msg.WriteMsg(client.Conn, msg.NewProxy{})
				e := msg.WriteMsg(proxy, msg.ForwardInfo{Port: port})
				if e != nil {
					proxy = nil
				}
			case <-time.After(time.Second * 15):
				client.Conn.Warn("no proxy after 15 secs")
			}
		}
	}

	// just exit
	if proxy == nil {
		return
	}

	proxy.AddPrefix("remote-" + c.RemoteAddr().String())
	proxy.Info("proxy selected, forward start")

	defer func() {
		proxy.Info("forward finished, public visitor:%s", c.RemoteAddr().String())
		proxy.Close()
		c.Close()
	}()
	// begin transfer data between them.
	go io.Copy(c, proxy)
	io.Copy(proxy, c)
	return
}

// ClientRegistry is a place storing clients.
type ClientRegistry map[string]*Client

// Register register client
func (registry *ClientRegistry) Register(id string, client *Client) (oClient *Client) {
	oClient, ok := (*registry)[id]
	if ok {
		return
	}
	(*registry)[id] = client
	return
}

// Register register client
func (registry *ClientRegistry) Unregister(id string) (oClient *Client) {
	oClient, ok := (*registry)[id]
	if ok {
		delete(*registry, id)
		return
	}
	return
}
