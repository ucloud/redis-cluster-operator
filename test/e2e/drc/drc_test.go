package drc_test

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/ucloud/redis-cluster-operator/test/e2e"
)

var (
	goredis *e2e.GoRedis
	dbsize  int64
	err     error
)

var _ = Describe("DistributedRedisCluster CRUD", func() {
	It("should create a DistributedRedisCluster", func() {
		name := e2e.RandString(8)
		password := e2e.RandString(8)
		drc = e2e.NewDistributedRedisCluster(name, f.Namespace(), e2e.Redis5_0_4, f.PasswordName(), 3, 1)
		Ω(f.CreateRedisClusterPassword(f.PasswordName(), password)).Should(Succeed())
		Ω(f.CreateRedisCluster(drc)).Should(Succeed())
		Eventually(e2e.IsDistributedRedisClusterProperly(f, drc), "10m", "10s").ShouldNot(HaveOccurred())
		goredis = e2e.NewGoRedisClient(name, f.Namespace(), password)
		Expect(goredis.StuffingData(10, 300000)).NotTo(HaveOccurred())
		dbsize, err = goredis.DBSize()
		Expect(err).NotTo(HaveOccurred())
		f.Logf("%s DBSIZE: %d", name, dbsize)
	})

	Context("when the DistributedRedisCluster is created", func() {
		It("should change redis config for a DistributedRedisCluster", func() {
			e2e.ChangeDRCRedisConfig(drc)
			Ω(f.UpdateRedisCluster(drc)).Should(Succeed())
			Eventually(e2e.IsDistributedRedisClusterProperly(f, drc), "10m", "10s").ShouldNot(HaveOccurred())
			Expect(e2e.IsDBSizeConsistent(dbsize, goredis)).NotTo(HaveOccurred())
		})
		It("should recover from accidentally deleting master pods", func() {
			e2e.DeleteMasterPodForDRC(drc, f.Client)
			Eventually(e2e.IsDRCPodBeDeleted(f, drc), "5m", "10s").ShouldNot(HaveOccurred())
			Eventually(e2e.IsDistributedRedisClusterProperly(f, drc), "10m", "10s").ShouldNot(HaveOccurred())
			goredis = e2e.NewGoRedisClient(drc.Name, f.Namespace(), goredis.Password())
			Expect(e2e.IsDBSizeConsistent(dbsize, goredis)).NotTo(HaveOccurred())
		})
		It("should scale up a DistributedRedisCluster", func() {
			e2e.ScaleUPDRC(drc)
			Ω(f.UpdateRedisCluster(drc)).Should(Succeed())
			Eventually(e2e.IsDistributedRedisClusterProperly(f, drc), "10m", "10s").ShouldNot(HaveOccurred())
			goredis = e2e.NewGoRedisClient(drc.Name, f.Namespace(), goredis.Password())
			Expect(e2e.IsDBSizeConsistent(dbsize, goredis)).NotTo(HaveOccurred())
		})
		Context("when the scale up succeeded", func() {
			It("should scale down a DistributedRedisCluster", func() {
				e2e.ScaleUPDown(drc)
				Ω(f.UpdateRedisCluster(drc)).Should(Succeed())
				Eventually(e2e.IsDistributedRedisClusterProperly(f, drc), "10m", "10s").ShouldNot(HaveOccurred())
				goredis = e2e.NewGoRedisClient(drc.Name, f.Namespace(), goredis.Password())
				Expect(e2e.IsDBSizeConsistent(dbsize, goredis)).NotTo(HaveOccurred())
			})
		})
		It("should reset the DistributedRedisCluster password", func() {
			newPassword := e2e.RandString(8)
			Ω(f.CreateRedisClusterPassword(f.NewPasswordName(), newPassword)).Should(Succeed())
			e2e.ResetPassword(drc, f.NewPasswordName())
			Ω(f.UpdateRedisCluster(drc)).Should(Succeed())
			time.Sleep(5 * time.Second)
			Eventually(e2e.IsDistributedRedisClusterProperly(f, drc), "10m", "10s").ShouldNot(HaveOccurred())
			goredis = e2e.NewGoRedisClient(drc.Name, f.Namespace(), newPassword)
			Expect(e2e.IsDBSizeConsistent(dbsize, goredis)).NotTo(HaveOccurred())
		})
		It("should update the DistributedRedisCluster minor version", func() {
			e2e.RollingUpdateDRC(drc)
			Ω(f.UpdateRedisCluster(drc)).Should(Succeed())
			Eventually(e2e.IsDistributedRedisClusterProperly(f, drc), "10m", "10s").ShouldNot(HaveOccurred())
			goredis = e2e.NewGoRedisClient(drc.Name, f.Namespace(), goredis.Password())
			Expect(e2e.IsDBSizeConsistent(dbsize, goredis)).NotTo(HaveOccurred())
		})
	})
})
