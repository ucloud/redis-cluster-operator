# redis-operator

## Overview

Redis Cluster Operator manages [Redis Cluster](https://redis.io/topics/cluster-spec) atop Kubernetes.

The operator itself is built with the [Operator framework](https://github.com/operator-framework/operator-sdk).

![Redis Cluster atop Kubernetes](/static/redis-cluster.png)

## Prerequisites

* go version v1.13+.
* Access to a Kubernetes v1.13.3 cluster.

## Features

- __Customize the number of master nodes and the number of replica nodes per master__

- __Password__

- __Safely Scaling the Redis Cluster__

- __Backup and Restore__

- __Persistent Volume__

- __Custom Configuration__

- __Prometheus Discovery__


## Quick Start

### Deploy redis cluster operator

Register the DistributedRedisCluster and RedisClusterBackup custom resource definition (CRD).
```
$ kubectl create -f deploy/crds/redis.kun_distributedredisclusters_crd.yaml
$ kubectl create -f deploy/crds/redis.kun_redisclusterbackups_crd.yaml
```

A namespace-scoped operator watches and manages resources in a single namespace, whereas a cluster-scoped operator watches and manages resources cluster-wide.
You can chose run your operator as namespace-scoped or cluster-scoped.
```
// cluster-scoped
$ kubectl create -f deploy/service_account.yaml
$ kubectl create -f deploy/cluster/cluster_role.yaml
$ kubectl create -f deploy/cluster/cluster_role_binding.yaml
$ kubectl create -f deploy/cluster/operator.yaml

// namespace-scoped
$ kubectl create -f deploy/service_account.yaml
$ kubectl create -f deploy/namespace/role.yaml
$ kubectl create -f deploy/namespace/role_binding.yaml
$ kubectl create -f deploy/namespace/operator.yaml
```

Verify that the redis-cluster-operator is up and running:
```
$ kubectl get deployment
NAME                     READY   UP-TO-DATE   AVAILABLE   AGE
redis-cluster-operator   1/1     1            1           1d
```

#### Deploy a sample Redis Cluster

```
$ kubectl apply -f deploy/example/redis.kun_v1alpha1_distributedrediscluster_cr.yaml
```

Verify that the cluster instances and its components are running.
```
$ kubectl get distributedrediscluster
NAME                              MASTERSIZE   STATUS    AGE
example-distributedrediscluster   3            Scaling   11s

$ kubectl get all -l redis.kun/name=example-distributedrediscluster
NAME                                        READY   STATUS    RESTARTS   AGE
pod/drc-example-distributedrediscluster-0   1/1     Running   0          4m5s
pod/drc-example-distributedrediscluster-1   1/1     Running   0          3m31s
pod/drc-example-distributedrediscluster-2   1/1     Running   0          2m54s
pod/drc-example-distributedrediscluster-3   1/1     Running   0          2m20s
pod/drc-example-distributedrediscluster-4   1/1     Running   0          103s
pod/drc-example-distributedrediscluster-5   1/1     Running   0          62s

NAME                                      TYPE        CLUSTER-IP   EXTERNAL-IP   PORT(S)              AGE
service/example-distributedrediscluster   ClusterIP   None         <none>        6379/TCP,16379/TCP   4m5s

NAME                                                   READY   AGE
statefulset.apps/drc-example-distributedrediscluster   6/6     4m5s

$ kubectl get distributedrediscluster
NAME                              MASTERSIZE   STATUS    AGE
example-distributedrediscluster   3            Healthy   4m
```

#### Scaling the Redis Cluster

Increase the masterSize to trigger the scaling.

```
apiVersion: redis.kun/v1alpha1
kind: DistributedRedisCluster
metadata:
  name: example-distributedrediscluster
spec:
  # Increase the masterSize to trigger the scaling.
  masterSize: 4
  ClusterReplicas: 1
  image: redis:5.0.4-alpine
```

#### Backup and Restore

Backup
```
$ kubectl create -f deploy/example/backup-restore/redisclusterbackup_cr.yaml
```

Restore from backup
```
$ kubectl create -f deploy/example/backup-restore/restore.yaml
```

#### Prometheus Discovery

```
$ kubectl create -f deploy/example/prometheus-exporter.yaml
```

#### Create Redis Cluster with password

```
$ kubectl create -f deploy/example/custom-password.yaml
```

#### Persistent Volume

```
$ kubectl create -f deploy/example/persistent.yaml
```

#### Custom Configuration

```
$ kubectl create -f deploy/example/custom-config.yaml
```

#### Custom Headless Service

```
$ kubectl create -f deploy/example/custom-service.yaml
```

#### Custom Resource

```
$ kubectl create -f deploy/example/custom-resources.yaml
```