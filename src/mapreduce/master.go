package mapreduce

import "container/list"
import "fmt"
import "sync"
import "strconv"

type WorkerInfo struct {
	address string
	// You can add definitions here.
}

type SyncMap struct {
	sync.RWMutex
	m map[string]string
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
	go func() {
		for {
			select {
			case newWorkerAddress := <-mr.registerChannel:
				workInfo := WorkerInfo{newWorkerAddress}
				mr.Workers[newWorkerAddress] = &workInfo
				mr.UserWorkerList.PushFront(workInfo)
				fmt.Printf("worker %s register to master\n", newWorkerAddress)
			default:
			}
		}
	}()

	distributeJob(mr, Map)
	distributeJob(mr, Reduce)
	return mr.KillWorkers()
}

func distributeJob(mr *MapReduce, jobType JobType) {
	var nJob int
	var otherJob int
	resultMap := SyncMap{m: make(map[string]string)}
	done := make(chan bool)
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
			doJobNum(mr, i, otherJob, jobType, resultMap)
			i++
		}
	}

	go handlerFailure(mr, jobType, nJob, otherJob, resultMap, done)

	<-done
}

func doJobNum(mr *MapReduce, jobNum int, otherJobNum int, jobType JobType, resultMap SyncMap) {
	if mr.UserWorkerList.Len() < 1 {
		fmt.Println("no userful wo")
		return
	}
	resultMap.Lock()
	resultMap.m[strconv.Itoa(jobNum)] = "doing"
	resultMap.Unlock()
	element := mr.UserWorkerList.Front()
	workInfo := element.Value.(WorkerInfo)
	jobArg := new(DoJobArgs)
	jobArg.File = mr.file
	jobArg.Operation = jobType
	jobArg.JobNumber = jobNum
	jobArg.NumOtherPhase = otherJobNum
	jobReply := new(DoJobReply)
	go func() {
		result := call(workInfo.address, "Worker.DoJob", jobArg, jobReply)
		if result && jobReply.OK {
			fmt.Printf("%d %s success \n", jobNum, jobType)
			resultMap.Lock()
			resultMap.m[strconv.Itoa(jobNum)] = "success"
			resultMap.Unlock()
		} else {
			fmt.Printf("%d %s fail \n", jobNum, jobType)
			resultMap.Lock()
			resultMap.m[strconv.Itoa(jobNum)] = "fail"
			resultMap.Unlock()
		}
	}()
}
func handlerFailure(mr *MapReduce, jobType JobType, jobNum int, otherJobNum int, resultMap SyncMap, done chan bool) {
	for {
		j := 0
		for i := 0; i < jobNum; i++ {
			resultMap.RLock()
			result, ok := resultMap.m[strconv.Itoa(i)]
			resultMap.RUnlock()
			if !ok {
				continue
			}
			if result == "success" {
				j++
				continue
			} else if result == "fail" {
				fmt.Printf("retry %d %s job \n", i, jobType)
				doJobNum(mr, i, otherJobNum, jobType, resultMap)
			}
		}

		if j >= jobNum {
			break
		}
	}
	done <- true
}
