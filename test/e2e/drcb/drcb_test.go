package drcb_test

import (
	"os"
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

var _ = Describe("Restore DistributedRedisCluster From RedisClusterBackup", func() {
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
			Expect(e2e.IsDBSizeConsistent(dbsize, goredis)).NotTo(HaveOccurred())
		})
		Context("when the RedisClusterBackup is created", func() {
			It("should restore from backup", func() {
				Ω(f.DeleteRedisCluster(drc)).Should(Succeed())
				rdrc = e2e.RestoreDRC(drc, drcb)
				Ω(f.CreateRedisCluster(rdrc)).Should(Succeed())
				Eventually(e2e.IsDistributedRedisClusterProperly(f, rdrc), "10m", "10s").ShouldNot(HaveOccurred())
				goredis = e2e.NewGoRedisClient(rdrc.Name, f.Namespace(), goredis.Password())
				Expect(e2e.IsDBSizeConsistent(dbsize, goredis)).NotTo(HaveOccurred())
			})
			Context("when restore is succeeded", func() {
				It("should change redis config for a DistributedRedisCluster", func() {
					e2e.ChangeDRCRedisConfig(rdrc)
					Ω(f.UpdateRedisCluster(rdrc)).Should(Succeed())
					Eventually(e2e.IsDistributedRedisClusterProperly(f, rdrc), "10m", "10s").ShouldNot(HaveOccurred())
					Expect(e2e.IsDBSizeConsistent(dbsize, goredis)).NotTo(HaveOccurred())
				})
				It("should recover from accidentally deleting master pods", func() {
					e2e.DeleteMasterPodForDRC(rdrc, f.Client)
					Eventually(e2e.IsDRCPodBeDeleted(f, rdrc), "5m", "10s").ShouldNot(HaveOccurred())
					Eventually(e2e.IsDistributedRedisClusterProperly(f, rdrc), "10m", "10s").ShouldNot(HaveOccurred())
					goredis = e2e.NewGoRedisClient(rdrc.Name, f.Namespace(), goredis.Password())
					Expect(e2e.IsDBSizeConsistent(dbsize, goredis)).NotTo(HaveOccurred())
				})
				It("should scale up a DistributedRedisCluster", func() {
					e2e.ScaleUPDRC(rdrc)
					Ω(f.UpdateRedisCluster(rdrc)).Should(Succeed())
					Eventually(e2e.IsDistributedRedisClusterProperly(f, rdrc), "10m", "10s").ShouldNot(HaveOccurred())
					goredis = e2e.NewGoRedisClient(rdrc.Name, f.Namespace(), goredis.Password())
					Expect(e2e.IsDBSizeConsistent(dbsize, goredis)).NotTo(HaveOccurred())
				})
				Context("when the scale up succeeded", func() {
					It("should scale down a DistributedRedisCluster", func() {
						e2e.ScaleUPDown(rdrc)
						Ω(f.UpdateRedisCluster(rdrc)).Should(Succeed())
						Eventually(e2e.IsDistributedRedisClusterProperly(f, rdrc), "10m", "10s").ShouldNot(HaveOccurred())
						goredis = e2e.NewGoRedisClient(rdrc.Name, f.Namespace(), goredis.Password())
						Expect(e2e.IsDBSizeConsistent(dbsize, goredis)).NotTo(HaveOccurred())
					})
				})
				It("should reset the DistributedRedisCluster password", func() {
					newPassword := e2e.RandString(8)
					Ω(f.CreateRedisClusterPassword(f.NewPasswordName(), newPassword)).Should(Succeed())
					e2e.ResetPassword(rdrc, f.NewPasswordName())
					Ω(f.UpdateRedisCluster(rdrc)).Should(Succeed())
					time.Sleep(5 * time.Second)
					Eventually(e2e.IsDistributedRedisClusterProperly(f, rdrc), "10m", "10s").ShouldNot(HaveOccurred())
					goredis = e2e.NewGoRedisClient(rdrc.Name, f.Namespace(), newPassword)
					Expect(e2e.IsDBSizeConsistent(dbsize, goredis)).NotTo(HaveOccurred())
				})
				It("should update the DistributedRedisCluster minor version", func() {
					e2e.RollingUpdateDRC(rdrc)
					Ω(f.UpdateRedisCluster(rdrc)).Should(Succeed())
					Eventually(e2e.IsDistributedRedisClusterProperly(f, rdrc), "10m", "10s").ShouldNot(HaveOccurred())
					goredis = e2e.NewGoRedisClient(rdrc.Name, f.Namespace(), goredis.Password())
					Expect(e2e.IsDBSizeConsistent(dbsize, goredis)).NotTo(HaveOccurred())
				})
			})
		})
	})
})
