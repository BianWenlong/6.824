package pbservice

const (
	OK             = "OK"
	ErrNoKey       = "ErrNoKey"
	ErrWrongServer = "ErrWrongServer"
	ErrDoingOrDone = "ErrDoingOrDone"
)

type Err string

// Put or Append
type PutAppendArgs struct {
	Key      string
	Value    string
	Sequence int64
	Op       string
	// You'll have to add definitions here.

	// Field names must start with capital letters,
	// otherwise RPC will break.
}

type PutAppendReply struct {
	Err Err
}

type GetArgs struct {
	Key      string
	Sequence int64
	// You'll have to add definitions here.
}

type GetReply struct {
	Err   Err
	Value string
}

type CopyArgs struct {
	Data        map[string]string
	HandlResult map[int64]bool
}

type CopyReply struct {
	Err Err
}

// Your RPC definitions here.
