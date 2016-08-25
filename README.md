# 6.824课程
@(我的第一个笔记本)[go, 分布式计算]
**[6.824](http://nil.csail.mit.edu/6.824/2015/index.html)**是MIT的一门分布式课程，它没有教学视频，只有文档，我们通过文档描述、阅读它提供的论文，来完成课程中给出的Lab。课程中共有五个Lab，我们需要完善Lab提供的代码，实现它的功能。课程代码是用golang编写的，因此在学习分布式系统理论的同时，可以提高golang的编程水平。

---------------------
[TOC]

###Lecture 1：课程介绍
**分布式系统**是一个多主机通过网络连接来协同实现某种功能的一类系统。我们选择分布式系统的原因：

- **资源利用** : 把分离的主机利用网络联系在一起
- **安全** : 通过物理隔离来实现安全
- **容错** : 利用不同节点复制来实现容错
- **高性能** : 通过并行来提升CPU、内存、硬盘、网络的性能

同时，分布式系统也面临者更多的困难和挑战：
- 复杂、很难调试
- 

### Lab 2  Primary/Backup Key/Value Service
#### Part B  Key/Value Service

**Primary/Backup Key/Value Service**的源代码存放在**pbservice**目录下，其中**client.go**是客户端的部分代码，**server.go**是服务端的部分代码，客户端通过生成**Clert**对象，调用它的方法，来实现RPC的调用。

键值服务的Get应该返回正确的结果，有以下两种情况：
- 返回上一次Put或者Append的数据
- 如果不存在返回空

保证同一时刻只有一个Primary Server,如果不能保证，则会出现这种情况：假设S1在前一个View中是Primary，ViewService更新了View，S2成为了Primary。S1仍然认为自己是Primary，接收客户端的Put。这样两个Server同时接收插入KV，没有去复制其他Server的数据，造成数据丢失。

非Primary Server在接收到客户端的请求时，应该返回Error

服务端在接收到客户端的请求后，必须同步完成后才能返回。
> Your server must filter out the duplicate RPCs that these client re-tries will generate to ensure at-most-once semantics for operations. You can assume that each clerk has only one outstanding Put or Get. Think carefully about what the commit point is for a Put.

服务端不必每次插入和查询时都去调用View Service，而是应该周期性的去Ping View Service。同样的客户端也不应该每次都去调用View Service，可以缓存当前的View，直到当前Server不可用的时候，才去调用View Service获取最新的View。

>A server should not talk to the viewservice for every Put/Get it receives, since that would put the viewservice on the critical path for performance and fault-tolerance. Instead servers should Ping the viewservice periodically (in pbservice/server.go's tick()) to learn about new views. Similarly, the client Clerk should not talk to the viewservice for every RPC it sends; instead, the Clerk should cache the current primary, and only talk to the viewservice when the current primary seems to be dead.

View Service 在更新Server时，只能把前一个View的BackUp晋升为Primary ，这样才能保证同一时刻只有一个Primary Server的规则。如果前一个View i 的Primary接受到一个客户端的请求，应该引导客户端调用 View i 的BackUp。如果BackUp还没有接收到View i+1，那么它会拒绝客户端的请求，这是没有问题的。如果BackUp接收到了View i+1，它就会执行客户端的请求。

>Part of your one-primary-at-a-time strategy should rely on the viewservice only promoting the backup from view i to be primary in view i+1. If the old primary from view i tries to handle a client request, it will forward it to its backup. If that backup hasn't heard about view i+1, then it's not acting as primary yet, so no harm done. If the backup has heard about view i+1 and is acting as primary, it knows enough to reject the old primary's forwarded client requests.


BackUp应该用当前完整的KV值来初始化，然后用户在Primary上的每一个Put/Append操作，BackUp都应该执行一遍，这样才能保证BackUp上的数据是正确的。
 >You'll need to ensure that the backup sees every update to the key/value database, by a combination of the primary initializing it with the complete key/value database and forwarding subsequent client operations. Your primary should forward just the arguments to each Append() to the backup; do not forward the resulting value, which might be large.
 
 **怎么做:**
- 首先修改**pbservice/server.go**的tick()方法 ，周期性的去Ping ViewService，来获取最新的View，确认自己是Primary还是BackUp。
- 实现**pbservice/server.go**中的Get, Put, 和 Append方法。利用Map来存储数据，实现**client.go**中的服务调用。
- 增加方法来实现BackUp上的数据也同步更新。
- 如果一个Server从Primary变成BackUp，它应该把所有的数据发给新的Primary
- 修改**client.go**来保证客户端在没有获得返回时能够不断的重试，同时服务端应该能够识别出客户端的重试请求，并正确处理。可以在RPC参数中增加字段来实现。
- 客户端应该能够处理当前View Primary拒绝接受请求的情况，这时候客户端应该请求ViewService获取最新的View。客户端可以Sleep来重试。

 >⚠注意：
 >- 应该新增加一个RPC方法来让BackUp更新数据，因为BackUp不能直接接受客户端的请求。
 >- 应该新增加一个RPC方法来让BackUp接受当前的全部KV数据，来初始化。
