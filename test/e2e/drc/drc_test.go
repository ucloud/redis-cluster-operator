package drc_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/ucloud/redis-cluster-operator/test/e2e"
)

var _ = Describe("DistributedRedisCluster CRUD", func() {
	It("should create a DistributedRedisCluster", func() {
		name := e2e.RandString(8)
		password := e2e.RandString(8)
		drc = e2e.NewDistributedRedisCluster(name, f.Namespace(), e2e.Redis5_0_4, f.PasswordName(), 3, 1)
		Ω(f.CreateRedisClusterPassword(password)).Should(Succeed())
		Ω(f.CreateRedisCluster(drc)).Should(Succeed())
		Eventually(e2e.IsDistributedRedisClusterProperly(f, drc), "10m", "10s").ShouldNot(HaveOccurred())
	})

	Context("when the DistributedRedisCluster is created", func() {
		It("should change redis config for a DistributedRedisCluster", func() {
			e2e.ChangeDRCRedisConfig(drc)
			Ω(f.UpdateRedisCluster(drc)).Should(Succeed())
			Eventually(e2e.IsDistributedRedisClusterProperly(f, drc), "10m", "10s").ShouldNot(HaveOccurred())
		})
		It("should recover from accidentally deleting master pods", func() {
			e2e.DeleteMasterPodForDRC(drc, f.Client)
			Eventually(e2e.IsDistributedRedisClusterProperly(f, drc), "10m", "10s").ShouldNot(HaveOccurred())
		})
		It("should scale up a DistributedRedisCluster", func() {
			e2e.ScaleUPDRC(drc)
			Ω(f.UpdateRedisCluster(drc)).Should(Succeed())
			Eventually(e2e.IsDistributedRedisClusterProperly(f, drc), "10m", "10s").ShouldNot(HaveOccurred())
		})
		Context("when the scale up succeeded", func() {
			It("should scale down a DistributedRedisCluster", func() {
				e2e.ScaleUPDown(drc)
				Ω(f.UpdateRedisCluster(drc)).Should(Succeed())
				Eventually(e2e.IsDistributedRedisClusterProperly(f, drc), "10m", "10s").ShouldNot(HaveOccurred())
			})
		})
		It("should update the DistributedRedisCluster minor version", func() {
			e2e.RollingUpdateDRC(drc)
			Ω(f.UpdateRedisCluster(drc)).Should(Succeed())
			Eventually(e2e.IsDistributedRedisClusterProperly(f, drc), "10m", "10s").ShouldNot(HaveOccurred())
		})
	})
})
