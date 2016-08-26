package pbservice

import "net"
import "fmt"
import "net/rpc"
import "log"
import "time"
import "viewservice"
import "sync"
import "sync/atomic"
import "os"
import "syscall"
import "math/rand"

type PBServer struct {
	mu           sync.Mutex
	l            net.Listener
	dead         int32 // for testing
	unreliable   int32 // for testing
	me           string
	vs           *viewservice.Clerk
	viewNum      uint
	primary      string
	backup       string
	data         map[string]string
	handleResult map[int64]bool
	pingFail     int
	// Your declarations here.
}

func (pb *PBServer) Get(args *GetArgs, reply *GetReply) error {
	//log.Printf("Server [%s] get key[%s] primary[%s]\n", pb.me, args.Key, pb.primary)
	if !pb.isPrimary() {
		reply.Err = ErrWrongServer
		reply.Value = ""
		return nil
	}
	if value, ok := pb.data[args.Key]; ok {
		reply.Value = value
		reply.Err = OK
	} else {
		reply.Err = ErrNoKey
		reply.Value = ""
	}
	return nil
}

func (pb *PBServer) isPrimary() bool {
	return pb.primary == pb.me
}

func (pb *PBServer) isDone(seq int64) bool {
	value, ok := pb.handleResult[seq]
	return ok && value
}

func (pb *PBServer) PutAppend(args *PutAppendArgs, reply *PutAppendReply) error {
	//log.Printf("Server[%s] put key[%s] value[%s]\n", pb.me, args.Key, args.Value)
	pb.mu.Lock()
	defer pb.mu.Unlock()

	// 判断自己是不是primary
	if !pb.isPrimary() {
		//log.Printf("Server [%s] is not Primary! Client should send request to [%s]", pb.me, pb.primary)
		reply.Err = ErrWrongServer
		return nil
	}

	pb.handlePutOrAppend(args, reply)
	//log.Printf("Done Server put key[%s] value[%s]\n", args.Key, args.Value)
	return nil
}

func (pb *PBServer) handlePutOrAppend(args *PutAppendArgs, reply *PutAppendReply) {
	// 判断请求是否已经处理过了
	if pb.isDone(args.Sequence) {
		reply.Err = ErrDoingOrDone
		return
	} else {
		pb.handleResult[args.Sequence] = true
	}

	//数据处理
	if value, ok := pb.data[args.Key]; ok && args.Op == "Append" {
		pb.data[args.Key] = value + args.Value
	} else {
		pb.data[args.Key] = args.Value
	}

	//请求发给Backup
	if pb.backup != "" && pb.isPrimary() {
		for {
			forwardArgs := new(PutAppendArgs)
			forwardArgs.Key = args.Key
			forwardArgs.Value = args.Value
			forwardArgs.Sequence = args.Sequence
			forwardArgs.Op = args.Op
			forwardReply := new(PutAppendReply)
			result := call(pb.backup, "PBServer.Forward", forwardArgs, forwardReply)
			if result {
				reply.Err = OK
				break
			} else {
				pb.tick()
			}
		}
	}
}

//
// ping the viewserver periodically.
// if view changed:
//   transition to new view.
//   manage transfer of state from primary to new backup.
//
func (pb *PBServer) tick() {
	nextView, err := pb.vs.Ping(pb.viewNum)
	if err == nil {
		//log.Printf("Server [%s] New View num is [%d], Primary is [%s] ", pb.me, nextView.Viewnum, nextView.Primary)
		if pb.me == nextView.Primary && pb.backup != nextView.Backup {
			args := new(CopyArgs)
			args.Data = pb.data
			args.HandlResult = pb.handleResult
			reply := new(CopyReply)
			call(nextView.Backup, "PBServer.Copy", args, reply)
		}
		pb.viewNum = nextView.Viewnum
		pb.primary = nextView.Primary
		pb.backup = nextView.Backup
		pb.pingFail = 0
	} else {
		pb.pingFail += 1
		if pb.pingFail > viewservice.DeadPings {
			pb.primary = ""
			pb.backup = ""
			pb.viewNum = 0
		}
	}
}

func (pb *PBServer) Copy(args *CopyArgs, reply *CopyReply) error {
	//log.Printf("Backup [%s] receive Date", pb.me)
	pb.data = args.Data
	pb.handleResult = args.HandlResult
	reply.Err = OK
	return nil
}

func (pb *PBServer) Forward(args *PutAppendArgs, reply *PutAppendReply) error {
	//log.Printf("Backup [%s] receive forward data Key[%s] value [%s] Op [%s]", pb.me, args.Key, args.Value, args.Op)
	pb.mu.Lock()
	defer pb.mu.Unlock()

	pb.handlePutOrAppend(args, reply)
	return nil
}

// tell the server to shut itself down.
// please do not change these two functions.
func (pb *PBServer) kill() {
	atomic.StoreInt32(&pb.dead, 1)
	pb.l.Close()
}

// call this to find out if the server is dead.
func (pb *PBServer) isdead() bool {
	return atomic.LoadInt32(&pb.dead) != 0
}

// please do not change these two functions.
func (pb *PBServer) setunreliable(what bool) {
	if what {
		atomic.StoreInt32(&pb.unreliable, 1)
	} else {
		atomic.StoreInt32(&pb.unreliable, 0)
	}
}

func (pb *PBServer) isunreliable() bool {
	return atomic.LoadInt32(&pb.unreliable) != 0
}

func StartServer(vshost string, me string) *PBServer {
	pb := new(PBServer)
	pb.me = me
	pb.vs = viewservice.MakeClerk(me, vshost)
	pb.viewNum = 0
	pb.primary = ""
	pb.backup = ""
	pb.data = make(map[string]string)
	pb.handleResult = make(map[int64]bool)
	pb.pingFail = 0
	pb.tick()
	// Your pb.* initializations here.

	rpcs := rpc.NewServer()
	rpcs.Register(pb)

	os.Remove(pb.me)
	l, e := net.Listen("unix", pb.me)
	if e != nil {
		log.Fatal("listen error: ", e)
	}
	pb.l = l

	// please do not change any of the following code,
	// or do anything to subvert it.

	go func() {
		for pb.isdead() == false {
			conn, err := pb.l.Accept()
			if err == nil && pb.isdead() == false {
				if pb.isunreliable() && (rand.Int63()%1000) < 100 {
					// discard the request.
					conn.Close()
				} else if pb.isunreliable() && (rand.Int63()%1000) < 200 {
					// process the request but force discard of reply.
					c1 := conn.(*net.UnixConn)
					f, _ := c1.File()
					err := syscall.Shutdown(int(f.Fd()), syscall.SHUT_WR)
					if err != nil {
						fmt.Printf("shutdown: %v\n", err)
					}
					go rpcs.ServeConn(conn)
				} else {
					go rpcs.ServeConn(conn)
				}
			} else if err == nil {
				conn.Close()
			}
			if err != nil && pb.isdead() == false {
				fmt.Printf("PBServer(%v) accept: %v\n", me, err.Error())
				pb.kill()
			}
		}
	}()

	go func() {
		for pb.isdead() == false {
			pb.tick()
			time.Sleep(viewservice.PingInterval)
		}
	}()

	return pb
}
