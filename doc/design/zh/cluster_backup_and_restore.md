# 备份和恢复

目前只支持备份到 ceph S3对象存储及本地 pvc 中。

备份开始时使用 redis-cli 同步 Master 的 RDB到本地后再使用 [Rclone](https://rclone.org/) 将
RDB 文件传输到对象存储或者 pvc 中，恢复时先使用 Rclone 从之前备份的位置同步备份到本地后，再启动 Redis
服务。备份恢复的工具类镜像中预置了 redis-cli 和 Rclone，参见 [Dockerfile](hack/docker/redis-tools/Dockerfile)。

## 备份

Operator 会 Watch 集群所有的 RedisClusterBackup 实例变化，当用户提交一个备份的 CR 之后，Operator 会：

1. 创建一个 Kubernetes batch job，根据 Redis 集群分布数，在 job 中注入相同数量的 container，每个 container 向一个 Master 发起备份请求，设置开始时间及备份状态。
2. 同步完成 RDB 文件后，将 Redis 集群每个分片的 RDB 文件和 cluster-config-file(记录节点slots信息) 上传到对象存储，同时将 CR 的状态置为 Succeeded，设置完成时间。redis集群备份的快照和节点元数据信息，上传到对象存储后，有统一的路径，当前的规则是：redis/{Namespace}/{RedisClusterName}/{StartTime}/{BackupName}-x
比如一个备份一个在 default 命名空间的名为 redis-cluster-test 的 Redis 集群（集群含有三个 master 节点），备份名为 backup , 备份开始时间为 20191101083020，最后会有如下对象存储路径：

```
redis/default/redis-cluster-test/20191101083020/backup-0
redis/default/redis-cluster-test/20191101083020/backup-1
redis/default/redis-cluster-test/20191101083020/backup-2
```

每个master节点备份的快照和节点元数据信息会存储在上述路径，用户可以到相应的 bucket 中查看。

## 从备份恢复

从备份恢复和创建步骤不同，分为两阶段，第一阶段同步数据，从快照启动 Master 节点；第二阶段启动 Slave 节点。

1. 设置`DistributedRedisCluster.Status.Restore.Phase=Running`，根据备份信息，创建与备份集群切片数相同的 Statefulset，
设置 Replicas 为 1，只启动 master 节点，注入 init container，init container 的作用是拉取对象存储上的快照数据。
2. 等待第1步同步数据完成，master 启动完成后，设置`DistributedRedisCluster.Status.Restore.Phase=Restart`，移除
init container 后等待节点重启。
3. 第2步完成之后，增加每个分片的副本数调大 Statefulset 的 Replicas，拉起 Slave 节点，设置`DistributedRedisCluster.Status.Restore.Phase=Succeeded`，
等待所有 Pod 节点状态变为 Runing 之后，设置每个 Statefulset 的 Slave 节点 replicate Master 节点，加入集群。
