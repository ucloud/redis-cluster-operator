# End to end tests for DistributedRedisCluster and RedisClusterBackup

## Run test in Kubernetes

`kubectl create -f deploy/e2e.yml`

## Build and push e2e test images
` DOCKER_REGISTRY=your_registry make push-e2e`
