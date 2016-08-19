package viewservice

import "net"
import "net/rpc"
import "log"
import "time"
import "sync"
import "fmt"
import "os"
import "sync/atomic"

type ViewServer struct {
	mu             sync.Mutex
	l              net.Listener
	dead           int32 // for testing
	rpccount       int32 // for testing
	me             string
	currentView    View
	currentViewAck map[uint]bool
	spTime         map[string]time.Time
	extraAddress   map[string]bool
	// Your declarations here.
}

//
// server Ping RPC handler.
//
func (vs *ViewServer) Ping(args *PingArgs, reply *PingReply) error {
	me := args.Me
	viewNum := args.Viewnum
	vs.spTime[me] = time.Now()
	if vs.currentView.Primary == "" {
		handlerNoPing(vs, me, viewNum)
	} else if vs.currentView.Primary == me {
		handlerPrimeryPing(vs, me, viewNum)
	} else if vs.currentView.Backup == me {

	} else {
		handlerNoPing(vs, me, viewNum)
	}
	reply.View = vs.currentView
	return nil
}

func handlerNoPing(vs *ViewServer, me string, viewNum uint) {
	if vs.currentView.Primary == "" {
		v := View{1, me, ""}
		vs.currentView = v
	} else {
		vs.extraAddress[me] = true
	}
}

func handlerPrimeryPing(vs *ViewServer, me string, viewNum uint) {
	if viewNum == 0 {
		if ack, ok := vs.currentViewAck[vs.currentView.Viewnum]; ok && ack {
			vs.currentView.Primary = vs.currentView.Backup
			vs.currentView.Backup = ""
			vs.currentView.Viewnum += 1
			vs.extraAddress[me] = true
			backup := ""
			if len(vs.extraAddress) > 0 {
				for k, v := range vs.extraAddress {
					if v {
						vs.currentView.Backup, backup = k, k
						break
					}
				}
				if backup != "" {
					delete(vs.extraAddress, backup)
				}
			}
		}
		return
	}
	vs.currentViewAck[viewNum] = true
	if vs.currentView.Backup == "" {
		backup := ""
		if len(vs.extraAddress) > 0 {
			for k, v := range vs.extraAddress {
				if v {
					vs.currentView.Backup, backup = k, k
					vs.currentView.Viewnum += 1
					break
				}
			}
			if backup != "" {
				delete(vs.extraAddress, backup)
			}
		}
	}

}

//
// server Get() RPC handler.
//
func (vs *ViewServer) Get(args *GetArgs, reply *GetReply) error {

	reply.View = vs.currentView

	return nil
}

//
// tick() is called once per PingInterval; it should notice
// if servers have died or recovered, and change the view
// accordingly.
//
func (vs *ViewServer) tick() {
	for k, v := range vs.spTime {
		interval := time.Now().Sub(v)
		if interval > PingInterval*DeadPings {
			handlerDeadServer(vs, k)
		}
	}
	// Your code here.
}

func handlerDeadServer(vs *ViewServer, me string) {
	if me == vs.currentView.Primary {
		handlerPrimaryDead(vs, me)
	} else if me == vs.currentView.Backup {
		handlerBackupDead(vs, me)
	} else {
		handlerOtherDead(vs, me)
	}
}
func handlerPrimaryDead(vs *ViewServer, me string) {
	if ack, ok := vs.currentViewAck[vs.currentView.Viewnum]; ok && ack {
		vs.currentView.Primary = vs.currentView.Backup
		vs.currentView.Backup = ""
		vs.currentView.Viewnum += 1
		backup := ""
		if len(vs.extraAddress) > 0 {
			for k, v := range vs.extraAddress {
				if v {
					vs.currentView.Backup, backup = k, k
					break
				}
			}
			if backup != "" {
				delete(vs.extraAddress, backup)
			}
		}
	}
}

func handlerBackupDead(vs *ViewServer, me string) {
	if ack, ok := vs.currentViewAck[vs.currentView.Viewnum]; ok && ack {
		vs.currentView.Backup = ""
		vs.currentView.Viewnum += 1
		backup := ""
		if len(vs.extraAddress) > 0 {
			for k, v := range vs.extraAddress {
				if v {
					vs.currentView.Backup, backup = k, k
					break
				}
			}
			if backup != "" {
				delete(vs.extraAddress, backup)
			}
		}
	}
}

func handlerOtherDead(vs *ViewServer, me string) {
	vs.extraAddress[me] = false
}

//
// tell the server to shut itself down.
// for testing.
// please don't change these two functions.
//
func (vs *ViewServer) Kill() {
	atomic.StoreInt32(&vs.dead, 1)
	vs.l.Close()
}

//
// has this server been asked to shut down?
//
func (vs *ViewServer) isdead() bool {
	return atomic.LoadInt32(&vs.dead) != 0
}

// please don't change this function.
func (vs *ViewServer) GetRPCCount() int32 {
	return atomic.LoadInt32(&vs.rpccount)
}

func StartServer(me string) *ViewServer {
	vs := new(ViewServer)
	vs.me = me
	vs.currentView = View{0, "", ""}
	vs.currentViewAck = make(map[uint]bool)
	vs.extraAddress = make(map[string]bool)
	vs.spTime = make(map[string]time.Time)
	// Your vs.* initializations here.

	// tell net/rpc about our RPC server and handlers.
	rpcs := rpc.NewServer()
	rpcs.Register(vs)

	// prepare to receive connections from clients.
	// change "unix" to "tcp" to use over a network.
	os.Remove(vs.me) // only needed for "unix"
	l, e := net.Listen("unix", vs.me)
	if e != nil {
		log.Fatal("listen error: ", e)
	}
	vs.l = l

	// please don't change any of the following code,
	// or do anything to subvert it.

	// create a thread to accept RPC connections from clients.
	go func() {
		for vs.isdead() == false {
			conn, err := vs.l.Accept()
			if err == nil && vs.isdead() == false {
				atomic.AddInt32(&vs.rpccount, 1)
				go rpcs.ServeConn(conn)
			} else if err == nil {
				conn.Close()
			}
			if err != nil && vs.isdead() == false {
				fmt.Printf("ViewServer(%v) accept: %v\n", me, err.Error())
				vs.Kill()
			}
		}
	}()

	// create a thread to call tick() periodically.
	go func() {
		for vs.isdead() == false {
			vs.tick()
			time.Sleep(PingInterval)
		}
	}()

	return vs
}
