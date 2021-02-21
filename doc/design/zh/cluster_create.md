# 创建集群

**需要注意的是，只有配置了持久化存储(PVC)的 CR 实例可以故障自愈**

如用户希望创建 3 分片的集群，每个分片一主一从：

```
apiVersion: redis.kun/v1alpha1
kind: DistributedRedisCluster
metadata:
  annotations:
    # if your operator run as cluster-scoped, add this annotations
    redis.kun/scope: cluster-scoped
  name: example-distributedrediscluster
spec:
  masterSize: 3         # 三分片
  clusterReplicas: 1    # 每个主节点一个从节点
  image: redis:5.0.4-alpine
  storage:
    type: persistent-claim
    size: 1Gi
    class: {StorageClassName}
    deleteClaim: true   # 删除 Redis Cluster 时，自动清理 pvc
```

Operator Watch 到新的 Redis Cluster CR 实例被创建时，Operator 会执行以下操作:

1. 为每个分片创建一个 Statefulset，每个 Statefulset Name 后缀以 0,1,2... 递增，设置 Statefulset Replicas 为副本数+1(Master)，每个 Statefulset 代表着一个分片及其所有副本，所以将创建 3 个 Statefulset，每个 Statefulset 的 Replicas 为 3，每个 Pod 代表一个 Redis 实例。
2. 等待所有 Pod 状态变为 Ready 且每个节点相互识别后，Operator 会在每个 Statefulset 的 Pod 中挑选一个作为 Master 节点，其余节点为该 Master 的 Slave，并尽可能保证所有 Master 节点不在同一个 k8s node。
3. 为 Master 分配 Slots，将 Slave 加入集群，从而组建集群。
4. 为每一个 Statefulset 创建一个 Headless Service，为整个集群创建一个 Service 指向所有的 pod。

## 亲和性和反亲和性

为保证 Redis CLuster 的高可用性，CRD 中设计了 `affinity` 及 `requiredAntiAffinity` (bool) 字段来做 Redis 节点间的打散：

* 当 affinity 和 requiredAntiAffinity 都未设置时，Operator 默认设置 Statefulset 管理的一组 pod 及 所有 pod 尽量反亲和；
* 当用户只设置 requiredAntiAffinity 字段的时，Operator 会设置 Statefulset 管理的一组 pod 强制反亲和，所有 pod 尽量反亲和；
* 当用户设置了 affinity 时，Statefulset 直接继承 affinity，Operator 不做额外设置。