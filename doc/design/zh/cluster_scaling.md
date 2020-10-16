# Redis Cluster 横向扩容和缩容

## 扩容

当 Operator Watch 到一个 Redis Cluster 需要扩容时，Operator 会:

1. 新建 Statefulset。
2. 等待新的 Pod 状态变为 Ready 且每个节点相互识别后后从新的 Statefulset 选出 Master 节点，其余为 Slave。
3. Operator 调整 Slot，将其他 Statefulset 的 Master 节点的 Slots 调度到新的 Master，尽可能使其平均分布到所有的 Redis 节点上。

以扩容一个节点为例。
numSlots(迁移节点数): 表示扩容时分配到新节点的 slot 数量
numSlots=16384/集群节点。
集群现有的每个 Master 节点待迁移slot数计算公式为：
待迁移slot数量 * (该源节点负责的slot数量 / slot总数)

当前 Master Slots 分布:
```
Master[0] -> Slots 0 - 5460      slots=5461
Master[1] -> Slots 5461 - 10922  slots=5462
Master[2] -> Slots 10923 - 16383 slots=5461
```
加入节点 Master[3]，numSlots=16384/4=4096
那么分配到集群现有每个 Master 节点的待迁移 migratingSlots 数为：
```
Master[0] migratingSlots = 4096 * (5461 / 16384) = 1365.25=1365
Master[1] migratingSlots = 4096 * (5462 / 16384) = 1365.5=1366
Master[2] migratingSlots = 4096 * (5461 / 16384) = 1365.25=1365
```
最终:
```
Master[0] -> Slots 1365-5460                      slots=4096
Master[1] -> Slots 6827-10922                     slots=4096
Master[2] -> Slots 12288-16383                    slots=4096
Master[3] -> Slots 0-1364 5461-6826 10923-12287   slots=4096
```
## 缩容

当 Operator Watch 到一个 Redis Cluster 需要缩容时，Operator 会:

1. 将待删除 Statefulset 的 Master 节点所有的 Slots 迁移到其他 Master。
2. 从后缀名最大的 Statefulset 的开始，将其所有的节点踢出集群。
3. 删除 Statefulset 及相关资源。