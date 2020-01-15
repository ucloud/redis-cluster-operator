package drc_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	redisv1alpha1 "github.com/ucloud/redis-cluster-operator/pkg/apis/redis/v1alpha1"
	"github.com/ucloud/redis-cluster-operator/test/e2e"
)

var f *e2e.Framework
var drc *redisv1alpha1.DistributedRedisCluster

func TestDrc(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Drc Suite")
}

var _ = BeforeSuite(func() {
	f = e2e.NewFramework("test")
	if err := f.BeforeEach(); err != nil {
		f.Failf("Framework BeforeEach err: %s", err.Error())
	}
})

var _ = AfterSuite(func() {
	if err := f.DeleteRedisCluster(drc); err != nil {
		f.Logf("deleting DistributedRedisCluster err: %s", err.Error())
	}
	if err := f.AfterEach(); err != nil {
		f.Failf("Framework AfterSuite err: %s", err.Error())
	}
})
