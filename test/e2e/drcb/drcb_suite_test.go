package drcb_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
	"github.com/ucloud/redis-cluster-operator/test/e2e"
)

var f *e2e.Framework
var drc *redisv1alpha1.DistributedRedisCluster
var rdrc *redisv1alpha1.DistributedRedisCluster
var drcb *redisv1alpha1.RedisClusterBackup

func TestDrcb(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Drcb Suite")
}

var _ = BeforeSuite(func() {
	f = e2e.NewFramework("drcb")
	if err := f.BeforeEach(); err != nil {
		f.Failf("Framework BeforeEach err: %s", err.Error())
	}
})

var _ = AfterSuite(func() {
	if err := f.DeleteRedisCluster(rdrc); err != nil {
		f.Logf("deleting DistributedRedisCluster err: %s", err.Error())
	}
	if err := f.AfterEach(); err != nil {
		f.Failf("Framework AfterSuite err: %s", err.Error())
	}
})
