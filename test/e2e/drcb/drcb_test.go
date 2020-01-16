package drcb_test

import (
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/ucloud/redis-cluster-operator/test/e2e"
)

var _ = Describe("Restore DistributedRedisCluster From RedisClusterBackup", func() {
	It("should create a DistributedRedisCluster", func() {
		name := e2e.RandString(8)
		password := e2e.RandString(8)
		drc = e2e.NewDistributedRedisCluster(name, f.Namespace(), e2e.Redis5_0_4, f.PasswordName(), 3, 1)
		Ω(f.CreateRedisClusterPassword(password)).Should(Succeed())
		Ω(f.CreateRedisCluster(drc)).Should(Succeed())
		Eventually(e2e.IsDistributedRedisClusterProperly(f, drc), "10m", "10s").ShouldNot(HaveOccurred())
	})

	Context("when the DistributedRedisCluster is created", func() {
		It("should create a RedisClusterBackup", func() {
			name := e2e.RandString(8)
			s3ID := os.Getenv(e2e.S3ID)
			s3Key := os.Getenv(e2e.S3KEY)
			endpoint := os.Getenv(e2e.S3ENDPOINT)
			bucket := os.Getenv(e2e.S3BUCKET)
			drcb = e2e.NewRedisClusterBackup(name, f.Namespace(), e2e.BackupImage, drc.Name, f.S3SecretName(), endpoint, bucket)
			Ω(f.CreateS3Secret(s3ID, s3Key)).Should(Succeed())
			Ω(f.CreateRedisClusterBackup(drcb)).Should(Succeed())
			Eventually(e2e.IsRedisClusterBackupProperly(f, drcb), "10m", "10s").ShouldNot(HaveOccurred())
		})
		Context("when the RedisClusterBackup is created", func() {
			It("should restore from backup", func() {
				Ω(f.DeleteRedisCluster(drc)).Should(Succeed())
				e2e.RestoreDRC(drc, drcb)
				Ω(f.CreateRedisCluster(drc)).Should(Succeed())
				Eventually(e2e.IsDistributedRedisClusterProperly(f, drc), "10m", "10s").ShouldNot(HaveOccurred())
			})
			Context("when restore is succeeded", func() {
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
	})
})
