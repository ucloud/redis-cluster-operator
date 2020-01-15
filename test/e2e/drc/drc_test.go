package drc_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/ucloud/redis-cluster-operator/test/e2e"
)

var _ = Describe("RedisCluster CRUD", func() {
	It("should create a DistributedRedisCluster", func() {
		name := e2e.RandString(8)
		password := e2e.RandString(8)
		drc = e2e.NewDistributedRedisCluster(name, f.Namespace(), e2e.Redis5_0_4, f.PasswordName(), 3, 1)
		Ω(f.CreateRedisClusterPassword(password)).Should(Succeed())
		Ω(f.CreateRedisCluster(drc)).Should(Succeed())
		Eventually(e2e.IsDistributedRedisClusterProperly(f, drc), "10m", "10s").ShouldNot(HaveOccurred())
	})

	Context("when the DistributedRedisCluster is created", func() {
		It("should scale up a DistributedRedisCluster", func() {
			drc.Spec.MasterSize = 4
			Ω(f.UpdateRedisCluster(drc)).Should(Succeed())
			Eventually(e2e.IsDistributedRedisClusterProperly(f, drc), "10m", "10s").ShouldNot(HaveOccurred())
		})
		Context("when the scale up succeeded", func() {
			It("should scale down a DistributedRedisCluster", func() {
				drc.Spec.MasterSize = 3
				Ω(f.UpdateRedisCluster(drc)).Should(Succeed())
				Eventually(e2e.IsDistributedRedisClusterProperly(f, drc), "10m", "10s").ShouldNot(HaveOccurred())
			})
		})
		It("should update the DistributedRedisCluster minor version", func() {
			drc.Spec.Image = e2e.Redis5_0_6
			Ω(f.UpdateRedisCluster(drc)).Should(Succeed())
			Eventually(e2e.IsDistributedRedisClusterProperly(f, drc), "10m", "10s").ShouldNot(HaveOccurred())
		})
	})
})
