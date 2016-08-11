package mapreduce

import "container/list"
import "fmt"

type WorkerInfo struct {
	address string
	// You can add definitions here.
}

// Clean up all workers by sending a Shutdown RPC to each one of them Collect
// the number of jobs each work has performed.
func (mr *MapReduce) KillWorkers() *list.List {
	l := list.New()
	for _, w := range mr.Workers {
		DPrintf("DoWork: shutdown %s\n", w.address)
		args := &ShutdownArgs{}
		var reply ShutdownReply
		ok := call(w.address, "Worker.Shutdown", args, &reply)
		if ok == false {
			fmt.Printf("DoWork: RPC %s shutdown error\n", w.address)
		} else {
			l.PushBack(reply.Njobs)
		}
	}
	return l
}

func (mr *MapReduce) RunMaster() *list.List {
	// Your code here

	go func() {
		for {
			select {
			case newWorkerAddress := <-mr.registerChannel:
				workInfo := WorkerInfo{newWorkerAddress}
				mr.Workers[newWorkerAddress] = &workInfo
				mr.UserWorkerList.PushFront(workInfo)
				fmt.Println("worker register to master")
			default:
			}
		}
	}()

	doMasterMap(mr, Map)
	doMasterMap(mr, Reduce)
	return mr.KillWorkers()
}

func doMasterMap(mr *MapReduce, jobType JobType) {
	var nJob int
	var otherJob int
	chanMap := make(map[int]chan bool)
	if jobType == Map {
		nJob = mr.nMap
		otherJob = mr.nReduce
	} else {
		nJob = mr.nReduce
		otherJob = mr.nMap
	}

	for i := 0; i < nJob; {
		if mr.UserWorkerList.Len() < 1 {
			continue
		} else {
			workInfo := mr.UserWorkerList.Front().Value.(WorkerInfo)
			jobArg := new(DoJobArgs)
			jobArg.File = mr.file
			jobArg.Operation = jobType
			jobArg.JobNumber = i
			jobArg.NumOtherPhase = otherJob
			jobReply := new(DoJobReply)
			done := make(chan bool)
			go func() {
				call(workInfo.address, "Worker.DoJob", jobArg, jobReply)
				mr.UserWorkerList.PushFront(workInfo)
				done <- true
			}()
			chanMap[i] = done
			i++
		}
	}

	for _, done := range chanMap {
		<-done
	}
}
